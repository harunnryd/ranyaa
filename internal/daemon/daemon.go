package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/browser"
	"github.com/harun/ranya/pkg/channels"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/cron"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/harun/ranya/pkg/hooks"
	"github.com/harun/ranya/pkg/memory"
	"github.com/harun/ranya/pkg/moderation"
	"github.com/harun/ranya/pkg/node"
	"github.com/harun/ranya/pkg/orchestrator"
	"github.com/harun/ranya/pkg/planner"
	"github.com/harun/ranya/pkg/plugin"
	"github.com/harun/ranya/pkg/routing"
	"github.com/harun/ranya/pkg/sandbox"
	"github.com/harun/ranya/pkg/session"
	"github.com/harun/ranya/pkg/subagent"
	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/harun/ranya/pkg/webhook"
	"github.com/harun/ranya/pkg/workspace"
	"github.com/rs/zerolog"
)

// Daemon represents the Ranya daemon service
type Daemon struct {
	config *config.Config
	logger *logger.Logger

	// Core modules
	queue          *commandqueue.CommandQueue
	sessionMgr     *session.SessionManager
	memoryMgr      *memory.Manager
	toolExecutor   *toolexecutor.ToolExecutor
	agentRunner    *agent.Runner
	workspaceMgr   *workspace.WorkspaceManager
	browserContext *browser.BrowserServerContext
	pluginRuntime  *plugin.PluginRuntime
	orchestrator   *orchestrator.Orchestrator
	planner        *planner.Planner
	subagentCoord  *subagent.Coordinator
	hookManager    *hooks.Manager
	contentFilter  *moderation.ContentFilter

	// Services
	gatewayServer   *gateway.Server
	webhookServer   *webhook.Server
	cronService     *cron.Service
	nodeManager     *node.NodeManager
	routingService  *routing.RoutingService
	channelRegistry *channels.Registry

	// Telegram
	telegramBot *telegram.Bot
	telegramCmd *telegram.Commands

	// Session management
	archiver *session.Archiver
	cleanup  *session.Cleanup

	// Internal
	eventLoop     *EventLoop
	router        *Router
	lifecycle     *LifecycleManager
	runtimeEvents *runtimeEventHub

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startTime time.Time
	running   bool
	mu        sync.RWMutex

	tracingEnabled bool
}

var newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
	return agent.NewRunner(cfg)
}

// New creates a new daemon instance
func New(cfg *config.Config, log *logger.Logger) (*Daemon, error) {
	ctx, cancel := context.WithCancel(context.Background())

	observability.EnsureRegistered()
	if err := tracing.InitOpenTelemetry("ranya-daemon"); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize tracing: %w", err)
	}

	d := &Daemon{
		config:         cfg,
		logger:         log,
		ctx:            ctx,
		cancel:         cancel,
		tracingEnabled: true,
		runtimeEvents:  newRuntimeEventHub(),
	}

	// Initialize core modules in dependency order
	if err := d.initializeCoreModules(); err != nil {
		cancel()
		if d.tracingEnabled {
			_ = tracing.ShutdownOpenTelemetry(context.Background())
			d.tracingEnabled = false
		}
		return nil, fmt.Errorf("failed to initialize core modules: %w", err)
	}

	// Initialize services
	if err := d.initializeServices(); err != nil {
		cancel()
		if d.tracingEnabled {
			_ = tracing.ShutdownOpenTelemetry(context.Background())
			d.tracingEnabled = false
		}
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	// Create internal components
	d.eventLoop = NewEventLoop(d)
	d.router = NewRouter(d)
	d.lifecycle = NewLifecycleManager(d)

	return d, nil
}

// initializeCoreModules initializes all core modules
func (d *Daemon) initializeCoreModules() error {
	d.queue = commandqueue.New()
	d.logger.Info().Msg("Command queue initialized")

	hookManager, err := newHookManager(d.config.Hooks, d.logger.GetZerolog())
	if err != nil {
		return fmt.Errorf("failed to create hook manager: %w", err)
	}
	d.hookManager = hookManager
	d.bindQueueHooks()
	d.logger.Info().Msg("Hook manager initialized")

	contentFilter, err := moderation.New(d.config.Moderation)
	if err != nil {
		return fmt.Errorf("failed to create content filter: %w", err)
	}
	d.contentFilter = contentFilter
	d.logger.Info().Bool("enabled", d.config.Moderation.Enabled).Msg("Content moderation initialized")

	sessionMgr, err := session.New(d.config.DataDir + "/sessions")
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}
	d.sessionMgr = sessionMgr
	d.logger.Info().Msg("Session manager initialized")

	memoryMgr, err := memory.NewManager(memory.Config{
		WorkspacePath: d.config.WorkspacePath,
		DBPath:        d.config.DataDir + "/memory.db",
		Logger:        d.logger.GetZerolog(),
	})
	if err != nil {
		return fmt.Errorf("failed to create memory manager: %w", err)
	}
	d.memoryMgr = memoryMgr
	d.logger.Info().Msg("Memory manager initialized")

	d.toolExecutor = toolexecutor.New()
	d.toolExecutor.SetRetryConfig(toolexecutor.RetryConfig{
		Enabled:        d.config.Tools.Retry.Enabled,
		MaxAttempts:    d.config.Tools.Retry.MaxAttempts,
		InitialBackoff: time.Duration(d.config.Tools.Retry.InitialBackoffMs) * time.Millisecond,
		MaxBackoff:     time.Duration(d.config.Tools.Retry.MaxBackoffMs) * time.Millisecond,
	})
	sandboxCfg := sandbox.DefaultConfig()
	sandboxCfg.Mode = sandbox.ModeTools
	sandboxCfg.Scope = sandbox.ScopeSession
	if len(d.config.Agents) > 0 {
		agentSandbox := d.config.Agents[0].Sandbox
		if runtime := strings.TrimSpace(agentSandbox.Runtime); runtime != "" {
			sandboxCfg.Runtime = sandbox.Runtime(runtime)
		}
		if image := strings.TrimSpace(agentSandbox.DockerImage); image != "" {
			sandboxCfg.Docker.Image = image
		}
	}
	if d.config.WorkspacePath != "" {
		sandboxCfg.FilesystemAccess.AllowedPaths = append(sandboxCfg.FilesystemAccess.AllowedPaths, d.config.WorkspacePath)
	}
	d.toolExecutor.SetSandboxManager(toolexecutor.NewSandboxManager(sandboxCfg))
	d.logger.Info().Msg("Tool executor initialized")

	if err := d.memoryMgr.RegisterTools(d.toolExecutor); err != nil {
		return fmt.Errorf("failed to register memory tools: %w", err)
	}
	d.logger.Info().Msg("Memory tools registered")

	if d.config.WorkspacePath != "" {
		workspaceMgr, err := workspace.NewWorkspaceManager(workspace.WorkspaceConfig{
			WorkspacePath: d.config.WorkspacePath,
		})
		if err != nil {
			return fmt.Errorf("failed to create workspace manager: %w", err)
		}
		d.workspaceMgr = workspaceMgr
		d.logger.Info().Str("path", d.config.WorkspacePath).Msg("Workspace manager initialized")
	}

	agentRunner, err := newAgentRunner(agent.Config{
		SessionManager: d.sessionMgr,
		ToolExecutor:   d.toolExecutor,
		CommandQueue:   d.queue,
		MemoryManager:  d.memoryMgr,
		Logger:         d.logger.GetZerolog(),
		AuthProfiles:   convertAuthProfiles(d.config.AI.Profiles),
	})
	if err != nil {
		return fmt.Errorf("failed to create agent runner: %w", err)
	}
	d.agentRunner = agentRunner
	d.logger.Info().Msg("Agent runner initialized")

	d.archiver = session.NewArchiver(d.sessionMgr, 30*time.Minute)
	d.logger.Info().Msg("Session archiver initialized")

	d.cleanup = session.NewCleanup(d.sessionMgr, 7*24*time.Hour)
	d.logger.Info().Msg("Session cleanup initialized")

	browserConfigPath := d.config.DataDir + "/browser.json"
	browserBaseDir := d.config.DataDir + "/browser"
	browserContext, err := browser.NewBrowserServerContext(browserConfigPath, browserBaseDir)
	if err != nil {
		return fmt.Errorf("failed to create browser context: %w", err)
	}
	if err := browserContext.Initialize(d.ctx); err != nil {
		return fmt.Errorf("failed to initialize browser context: %w", err)
	}
	d.browserContext = browserContext
	d.logger.Info().Msg("Browser context initialized")

	// Register browser tools with tool executor
	if err := browser.RegisterBrowserTools(d.toolExecutor, d.browserContext); err != nil {
		return fmt.Errorf("failed to register browser tools: %w", err)
	}
	d.logger.Info().Msg("Browser tools registered")

	pluginRuntime := plugin.NewPluginRuntime(d.logger.GetZerolog(), plugin.PluginRuntimeConfig{
		BuiltinDir:    d.config.DataDir + "/plugins/builtin",
		WorkspaceDir:  d.config.WorkspacePath + "/.ranya/plugins",
		ExtraDirs:     []string{},
		PluginConfigs: make(map[string]map[string]any),
	})
	loadResult, err := pluginRuntime.Initialize()
	if err != nil {
		d.logger.Warn().Err(err).Msg("Plugin runtime initialization had errors")
	}
	if loadResult != nil {
		d.logger.Info().
			Int("loaded", len(loadResult.Loaded)).
			Int("failed", len(loadResult.Failed)).
			Msg("Plugin runtime initialized")
	}
	d.pluginRuntime = pluginRuntime

	// Register plugin tools with tool executor
	if err := d.registerPluginTools(); err != nil {
		d.logger.Warn().Err(err).Msg("Failed to register plugin tools")
	}

	orchStore, err := orchestrator.NewFileStore(d.config.DataDir + "/orchestrator")
	if err != nil {
		return fmt.Errorf("failed to create orchestrator store: %w", err)
	}

	d.orchestrator = orchestrator.New(
		orchestrator.WithStore(orchStore),
		orchestrator.WithLogger(&orchLoggerAdapter{logger: d.logger.GetZerolog()}),
		orchestrator.WithMaxConcurrent(50),
	)

	if err := d.orchestrator.Recover(); err != nil {
		d.logger.Warn().Err(err).Msg("Failed to recover orchestrator state")
	}

	if err := d.registerOrchestratorAgents(); err != nil {
		return fmt.Errorf("failed to register orchestrator agents: %w", err)
	}
	d.logger.Info().Msg("Orchestrator initialized")

	d.planner = planner.NewPlanner()
	d.logger.Info().Msg("Planner initialized")

	subagentCoord, err := subagent.NewCoordinator(subagent.Config{
		RegistryPath: d.config.DataDir + "/subagents.json",
		AutoSave:     true,
		Logger:       d.logger.GetZerolog(),
	})
	if err != nil {
		return fmt.Errorf("failed to create subagent coordinator: %w", err)
	}
	if err := subagentCoord.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize subagent coordinator: %w", err)
	}
	d.subagentCoord = subagentCoord
	d.logger.Info().Msg("Subagent coordinator initialized")

	return nil
}

// initializeServices initializes all services
func (d *Daemon) initializeServices() error {
	gatewayServer, err := gateway.NewServer(gateway.Config{
		Port:            d.config.Gateway.Port,
		SharedSecret:    d.config.Gateway.SharedSecret,
		TickInterval:    time.Duration(d.config.Gateway.TickInterval) * time.Millisecond,
		CommandQueue:    d.queue,
		AgentRunner:     d.agentRunner,
		AgentDispatcher: d.dispatchGatewayRequest,
		SessionManager:  d.sessionMgr,
		MemoryManager:   d.memoryMgr,
		RuntimeEventHub: d.runtimeEvents,
		Logger:          d.logger.GetZerolog(),
	})
	if err != nil {
		return fmt.Errorf("failed to create gateway server: %w", err)
	}
	d.gatewayServer = gatewayServer
	d.logger.Info().Int("port", d.config.Gateway.Port).Msg("Gateway server initialized")
	d.agentRunner.SetEventEmitter(agent.EventEmitterFunc(func(ctx context.Context, evt agent.RuntimeEvent) {
		if d.gatewayServer == nil {
			if d.runtimeEvents != nil {
				d.runtimeEvents.Publish(tracing.GetSessionKey(ctx), evt)
			}
			return
		}

		payload := make(map[string]interface{}, len(evt.Metadata)+3)
		for k, v := range evt.Metadata {
			payload[k] = v
		}
		if evt.ToolName != "" {
			payload["tool"] = evt.ToolName
		}
		if evt.ToolCall != "" {
			payload["tool_call_id"] = evt.ToolCall
		}
		if evt.Content != "" {
			payload["content"] = evt.Content
		}

		sessionKey := tracing.GetSessionKey(ctx)

		d.gatewayServer.BroadcastTyped(gateway.EventMessage{
			Event:     evt.Event,
			Stream:    gateway.StreamType(evt.Stream),
			Phase:     evt.Phase,
			Data:      payload,
			TraceID:   tracing.GetTraceID(ctx),
			RunID:     tracing.GetRunID(ctx),
			Session:   sessionKey,
			AgentID:   tracing.GetAgentID(ctx),
			Timestamp: time.Now().UnixMilli(),
		})

		if d.runtimeEvents != nil {
			d.runtimeEvents.Publish(sessionKey, evt)
		}
	}))

	// Configure Tool Approval Workflow (P0-1)
	approvalForwarder := gateway.NewApprovalForwarder(d.gatewayServer)
	// Note: AllowlistManager is nil for now (stateless persistence)
	approvalHandler := toolexecutor.NewChatApprovalHandler(approvalForwarder, nil)
	approvalManager := toolexecutor.NewApprovalManager(approvalHandler)

	// Set approval manager on tool executor
	d.toolExecutor.SetApprovalManager(approvalManager)

	// Register approval RPC methods on gateway
	d.gatewayServer.RegisterApprovalMethods(approvalManager)
	d.logger.Info().Msg("User-in-the-loop tool approval workflow configured")

	nodeManager := node.NewNodeManager(node.NodeManagerConfig{
		NodeConfig: node.NodeConfig{
			HeartbeatInterval:   30 * time.Second,
			HeartbeatTimeout:    90 * time.Second,
			MaxMissedHeartbeats: 3,
			InvocationTimeout:   30 * time.Second,
			DegradedThreshold:   5,
			StoragePath:         d.config.DataDir + "/nodes",
			AutoSaveInterval:    5 * time.Minute,
		},
		CommandQueue: d.queue,
	})
	d.nodeManager = nodeManager
	d.logger.Info().Msg("Node manager initialized")

	routingCfg := routing.DefaultRoutingServiceConfig()
	if d.config.DataDir != "" {
		routingCfg.Storage.FilePath = filepath.Join(d.config.DataDir, "routes.json")
	}
	routingService := routing.NewRoutingService(routingCfg)
	d.routingService = routingService
	d.logger.Info().Msg("Routing service initialized")

	if err := d.initializeChannelRegistry(); err != nil {
		return fmt.Errorf("failed to initialize channel registry: %w", err)
	}

	if err := node.RegisterGatewayMethods(d.gatewayServer, d.nodeManager, d.queue); err != nil {
		return fmt.Errorf("failed to register node gateway methods: %w", err)
	}
	if err := routing.RegisterGatewayMethods(d.gatewayServer, d.routingService, d.queue); err != nil {
		return fmt.Errorf("failed to register routing gateway methods: %w", err)
	}
	d.logger.Info().Msg("Gateway methods registered")

	if d.config.Webhook.Enabled {
		webhookServer, err := webhook.NewServer(
			webhook.ServerOptions{
				Port: d.config.Webhook.Port,
				Host: d.config.Webhook.Host,
			},
			d.queue,
			d.agentRunner,
			d.gatewayServer,
			d.logger.GetZerolog(),
		)
		if err != nil {
			return fmt.Errorf("failed to create webhook server: %w", err)
		}
		d.webhookServer = webhookServer
		d.logger.Info().Int("port", d.config.Webhook.Port).Msg("Webhook server initialized")
	}

	cronService, err := cron.NewService(cron.ServiceOptions{
		StorePath: d.config.DataDir + "/cron.json",
		EnqueueSystemEvent: func(text string, agentID string) {
			d.logger.Info().Str("text", text).Str("agentID", agentID).Msg("Cron system event")
		},
		RunIsolatedAgentJob: func(job *cron.Job, message string) error {
			sessionKey := fmt.Sprintf("cron:%s", job.ID)
			ctx := tracing.NewRequestContext(d.ctx)
			ctx = tracing.WithSessionKey(ctx, sessionKey)
			ctx = tracing.WithRunID(ctx, tracing.NewRunID())
			logger := tracing.LoggerFromContext(ctx, d.logger.GetZerolog()).With().
				Str("session_key", sessionKey).
				Str("job_id", job.ID).
				Logger()

			agentCfg := agent.DefaultConfig()
			if len(d.config.Agents) > 0 {
				agentCfg.Model = d.config.Agents[0].Model
				agentCfg.SystemPrompt = d.config.Agents[0].SystemPrompt
			}
			agentCfg.UseMemory = true

			logger.Info().Str("message", message).Msg("Executing cron agent job")
			_, runErr := d.channelRegistry.Dispatch(ctx, channels.InboundMessage{
				Channel:    "cron",
				SessionKey: sessionKey,
				Content:    message,
				AgentID:    "default",
				RunConfig:  agentCfg,
				CWD:        d.config.WorkspacePath,
				Metadata: map[string]interface{}{
					"job_id": job.ID,
				},
			})
			if runErr != nil {
				logger.Error().Err(runErr).Msg("Cron agent job failed")
				return runErr
			}

			logger.Info().Msg("Cron agent job completed")
			return nil
		},
		RequestHeartbeatNow: func() {
			d.logger.Debug().Msg("Cron heartbeat requested")
		},
		OnEvent: func(evt cron.Event) {
			d.logger.Debug().Str("jobID", evt.JobID).Msg("Cron event")
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create cron service: %w", err)
	}
	d.cronService = cronService
	d.logger.Info().Msg("Cron service initialized")

	if d.config.Channels.Telegram.Enabled {
		bot, err := telegram.New(&d.config.Telegram, d.logger)
		if err != nil {
			return fmt.Errorf("failed to create telegram bot: %w", err)
		}
		d.telegramBot = bot

		commands := telegram.NewCommands(bot)
		d.telegramCmd = commands
		d.telegramBot.SetCommandHandler(commands)

		if err := d.registerChannel(newTelegramIngressChannel(
			bot,
			commands,
			d.config.Telegram,
			d.subscribeRuntimeEvents,
			d.clearRuntimeSession,
			d.logger.GetZerolog(),
		)); err != nil {
			return fmt.Errorf("failed to register telegram channel: %w", err)
		}
		if err := d.configureTelegramApprovalWorkflow(); err != nil {
			return fmt.Errorf("failed to configure telegram approvals: %w", err)
		}
		d.logger.Info().Msg("Telegram bot initialized")
	}

	return nil
}

// convertAuthProfiles converts config auth profiles to agent auth profiles
func convertAuthProfiles(profiles []config.AIProfile) []agent.AuthProfile {
	result := make([]agent.AuthProfile, len(profiles))
	for i, p := range profiles {
		result[i] = agent.AuthProfile{
			ID:       p.ID,
			Provider: p.Provider,
			APIKey:   p.APIKey,
			Priority: p.Priority,
		}
	}
	return result
}

// registerPluginTools registers all plugin tools with the tool executor
func (d *Daemon) registerPluginTools() error {
	if d.pluginRuntime == nil {
		return nil
	}

	// Create adapter for plugin runtime
	adapter := plugin.NewToolExecutorAdapter(d.pluginRuntime)

	// Get all plugin tools
	pluginTools := adapter.GetAllPluginTools()

	if len(pluginTools) == 0 {
		d.logger.Debug().Msg("No plugin tools to register")
		return nil
	}

	// Register each plugin tool
	registered := 0
	failed := 0

	for _, pluginTool := range pluginTools {
		// Convert plugin ToolDefinition to toolexecutor ToolDefinition
		// We need to convert the Parameters from map[string]any to []ToolParameter
		params := convertPluginToolParameters(pluginTool.Tool.Parameters)

		toolDef := toolexecutor.ToolDefinition{
			Name:        pluginTool.Tool.Name,
			Description: pluginTool.Tool.Description,
			Parameters:  params,
		}

		// Register with conflict resolution
		if err := d.toolExecutor.RegisterPluginTool(pluginTool.PluginID, toolDef, adapter); err != nil {
			d.logger.Warn().
				Err(err).
				Str("plugin", pluginTool.PluginID).
				Str("tool", pluginTool.Tool.Name).
				Msg("Failed to register plugin tool")
			failed++
			continue
		}

		registered++
	}

	d.logger.Info().
		Int("registered", registered).
		Int("failed", failed).
		Int("total", len(pluginTools)).
		Msg("Plugin tools registered with tool executor")

	return nil
}

// convertPluginToolParameters converts plugin tool parameters to toolexecutor parameters
func convertPluginToolParameters(params map[string]any) []toolexecutor.ToolParameter {
	// Plugin tools use JSON Schema format in params
	// We need to extract properties and required fields
	var result []toolexecutor.ToolParameter

	properties, ok := params["properties"].(map[string]any)
	if !ok {
		return result
	}

	required := make(map[string]bool)
	if reqList, ok := params["required"].([]any); ok {
		for _, r := range reqList {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}

	for name, propData := range properties {
		prop, ok := propData.(map[string]any)
		if !ok {
			continue
		}

		param := toolexecutor.ToolParameter{
			Name:     name,
			Required: required[name],
		}

		if typeVal, ok := prop["type"].(string); ok {
			param.Type = typeVal
		}

		if desc, ok := prop["description"].(string); ok {
			param.Description = desc
		}

		if def, ok := prop["default"]; ok {
			param.Default = def
		}

		result = append(result, param)
	}

	return result
}

// Start starts the daemon service
func (d *Daemon) Start() error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon is already running")
	}
	d.running = true
	d.startTime = time.Now()
	d.mu.Unlock()

	traceID := tracing.NewTraceID()
	logger := d.logger.GetZerolog().With().Str("trace_id", traceID).Logger()
	logger.Info().Msg("Starting Ranya daemon")

	// Start lifecycle manager
	if err := d.lifecycle.Start(); err != nil {
		return fmt.Errorf("failed to start lifecycle manager: %w", err)
	}

	// Start workspace manager if configured
	if d.workspaceMgr != nil {
		if err := d.workspaceMgr.Init(); err != nil {
			logger.Warn().Err(err).Msg("Failed to initialize workspace manager")
		} else {
			logger.Info().Msg("Workspace manager started")
		}
	}

	// Start gateway server
	if err := d.gatewayServer.Start(); err != nil {
		return fmt.Errorf("failed to start gateway server: %w", err)
	}
	logger.Info().Msg("Gateway server started")

	// Start webhook server if enabled
	if d.webhookServer != nil {
		if err := d.webhookServer.Start(); err != nil {
			logger.Warn().Err(err).Msg("Failed to start webhook server")
		} else {
			logger.Info().Msg("Webhook server started")
		}
	}

	// Start cron service
	logger.Info().Msg("Cron service started")

	// Start routing service
	if d.routingService != nil {
		if err := d.routingService.Start(); err != nil {
			return fmt.Errorf("failed to start routing service: %w", err)
		}
		logger.Info().Msg("Routing service started")
	}

	if d.channelRegistry != nil {
		if err := d.channelRegistry.StartAll(d.ctx); err != nil {
			return fmt.Errorf("failed to start ingress channels: %w", err)
		}
		logger.Info().Strs("channels", d.channelRegistry.Names()).Msg("Ingress channels started")
	}

	// Start Telegram bot if enabled
	if d.telegramBot != nil {
		if err := d.telegramBot.Start(); err != nil {
			return fmt.Errorf("failed to start telegram bot: %w", err)
		}
		logger.Info().Msg("Telegram bot started")
	}

	// Start session archiver
	if err := d.archiver.Start(); err != nil {
		logger.Warn().Err(err).Msg("Failed to start session archiver")
	} else {
		logger.Info().Msg("Session archiver started")
	}

	// Start session cleanup
	if err := d.cleanup.Start(); err != nil {
		logger.Warn().Err(err).Msg("Failed to start session cleanup")
	} else {
		logger.Info().Msg("Session cleanup started")
	}

	// Start event loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.eventLoop.Run(d.ctx)
	}()

	d.triggerStartupHooks()

	logger.Info().Msg("Daemon started successfully - all core modules active")

	return nil
}

// Stop stops the daemon service gracefully
func (d *Daemon) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon is not running")
	}
	d.running = false
	d.mu.Unlock()

	traceID := tracing.NewTraceID()
	logger := d.logger.GetZerolog().With().Str("trace_id", traceID).Logger()
	logger.Info().Msg("Stopping Ranya daemon")

	if d.channelRegistry != nil {
		if err := d.channelRegistry.StopAll(context.Background()); err != nil {
			logger.Error().Err(err).Msg("Failed to stop ingress channels")
		}
	}

	// Stop Telegram bot
	if d.telegramBot != nil {
		if err := d.telegramBot.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop telegram bot")
		}
	}

	// Stop webhook server
	if d.webhookServer != nil {
		if err := d.webhookServer.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop webhook server")
		}
	}

	// Stop gateway server
	if d.gatewayServer != nil {
		if err := d.gatewayServer.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop gateway server")
		}
	}

	// Stop cron service
	logger.Info().Msg("Cron service stopped")

	// Stop routing service
	if d.routingService != nil {
		if err := d.routingService.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop routing service")
		}
	}

	// Stop workspace manager
	if d.workspaceMgr != nil {
		if err := d.workspaceMgr.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close workspace manager")
		}
	}

	// Stop session archiver
	if d.archiver != nil && d.archiver.IsRunning() {
		if err := d.archiver.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop session archiver")
		}
	}

	// Stop session cleanup
	if d.cleanup != nil && d.cleanup.IsRunning() {
		if err := d.cleanup.Stop(); err != nil {
			logger.Error().Err(err).Msg("Failed to stop session cleanup")
		}
	}

	// Stop browser context
	if d.browserContext != nil {
		if err := d.browserContext.Shutdown(d.ctx); err != nil {
			logger.Error().Err(err).Msg("Failed to shutdown browser context")
		}
	}

	// Stop plugin runtime
	if d.pluginRuntime != nil {
		if err := d.pluginRuntime.Shutdown(); err != nil {
			logger.Error().Err(err).Msg("Failed to shutdown plugin runtime")
		}
	}

	// Stop orchestrator
	if d.orchestrator != nil {
		if err := d.orchestrator.Shutdown(d.ctx); err != nil {
			logger.Error().Err(err).Msg("Failed to shutdown orchestrator")
		}
	}

	// Stop subagent coordinator
	if d.subagentCoord != nil {
		if err := d.subagentCoord.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close subagent coordinator")
		}
	}

	// Cancel context
	d.cancel()

	// Wait for goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info().Msg("All goroutines stopped")
	case <-time.After(5 * time.Second):
		logger.Warn().Msg("Timeout waiting for goroutines to stop")
	}

	// Stop lifecycle manager
	if err := d.lifecycle.Stop(); err != nil {
		logger.Error().Err(err).Msg("Failed to stop lifecycle manager")
	}

	// Close memory manager
	if d.memoryMgr != nil {
		if err := d.memoryMgr.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close memory manager")
		}
	}

	// Close session manager
	if err := d.sessionMgr.Close(); err != nil {
		logger.Error().Err(err).Msg("Failed to close session manager")
	}

	if d.tracingEnabled {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := tracing.ShutdownOpenTelemetry(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Failed to shutdown tracing")
		}
		cancel()
		d.tracingEnabled = false
	}

	logger.Info().Msg("Daemon stopped successfully")

	return nil
}

// Status returns the daemon status
func (d *Daemon) Status() Status {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := Status{
		Running: d.running,
	}

	if d.running {
		status.Uptime = time.Since(d.startTime)
		status.StartTime = d.startTime
	}

	return status
}

// Wait waits for the daemon to stop
func (d *Daemon) Wait() {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	d.logger.Info().Str("signal", sig.String()).Msg("Received signal")

	// Stop daemon
	if err := d.Stop(); err != nil {
		d.logger.Error().Err(err).Msg("Failed to stop daemon")
	}
}

// GetConfig returns the daemon configuration
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}

// GetLogger returns the daemon logger
func (d *Daemon) GetLogger() *logger.Logger {
	return d.logger
}

// GetQueue returns the command queue
func (d *Daemon) GetQueue() *commandqueue.CommandQueue {
	return d.queue
}

// GetSessionManager returns the session manager
func (d *Daemon) GetSessionManager() *session.SessionManager {
	return d.sessionMgr
}

// GetRouter returns the message router
func (d *Daemon) GetRouter() *Router {
	return d.router
}

// Status represents daemon status
type Status struct {
	Running   bool
	Uptime    time.Duration
	StartTime time.Time
}

// GetTelegramBot returns the Telegram bot
func (d *Daemon) GetTelegramBot() *telegram.Bot {
	return d.telegramBot
}

// GetArchiver returns the session archiver
func (d *Daemon) GetArchiver() *session.Archiver {
	return d.archiver
}

// GetCleanup returns the session cleanup
func (d *Daemon) GetCleanup() *session.Cleanup {
	return d.cleanup
}

// GetMemoryManager returns the memory manager
func (d *Daemon) GetMemoryManager() *memory.Manager {
	return d.memoryMgr
}

// GetToolExecutor returns the tool executor
func (d *Daemon) GetToolExecutor() *toolexecutor.ToolExecutor {
	return d.toolExecutor
}

// GetAgentRunner returns the agent runner
func (d *Daemon) GetAgentRunner() *agent.Runner {
	return d.agentRunner
}

// GetWorkspaceManager returns the workspace manager
func (d *Daemon) GetWorkspaceManager() *workspace.WorkspaceManager {
	return d.workspaceMgr
}

// GetGatewayServer returns the gateway server
func (d *Daemon) GetGatewayServer() *gateway.Server {
	return d.gatewayServer
}

// GetWebhookServer returns the webhook server
func (d *Daemon) GetWebhookServer() *webhook.Server {
	return d.webhookServer
}

// GetCronService returns the cron service
func (d *Daemon) GetCronService() *cron.Service {
	return d.cronService
}

// GetNodeManager returns the node manager
func (d *Daemon) GetNodeManager() *node.NodeManager {
	return d.nodeManager
}

// GetRoutingService returns the routing service
func (d *Daemon) GetRoutingService() *routing.RoutingService {
	return d.routingService
}

// GetChannelRegistry returns the channel registry.
func (d *Daemon) GetChannelRegistry() *channels.Registry {
	return d.channelRegistry
}

// GetBrowserContext returns the browser context
func (d *Daemon) GetBrowserContext() *browser.BrowserServerContext {
	return d.browserContext
}

// GetPluginRuntime returns the plugin runtime
func (d *Daemon) GetPluginRuntime() *plugin.PluginRuntime {
	return d.pluginRuntime
}

// GetOrchestrator returns the orchestrator
func (d *Daemon) GetOrchestrator() *orchestrator.Orchestrator {
	return d.orchestrator
}

// GetPlanner returns the planner.
func (d *Daemon) GetPlanner() *planner.Planner {
	return d.planner
}

func (d *Daemon) subscribeRuntimeEvents(sessionKey string) (<-chan agent.RuntimeEvent, func()) {
	if d.runtimeEvents == nil {
		ch := make(chan agent.RuntimeEvent)
		close(ch)
		return ch, func() {}
	}
	return d.runtimeEvents.Subscribe(sessionKey, 128)
}

func (d *Daemon) clearRuntimeSession(sessionKey string) error {
	if d.agentRunner != nil {
		_ = d.agentRunner.Abort(sessionKey)
	}
	if d.sessionMgr == nil {
		return nil
	}
	return d.sessionMgr.ClearSessionWithContext(context.Background(), sessionKey)
}

func (d *Daemon) GetSubagentCoordinator() *subagent.Coordinator {
	return d.subagentCoord
}

// orchLoggerAdapter adapts zerolog.Logger to orchestrator.Logger
type orchLoggerAdapter struct {
	logger zerolog.Logger
}

func (l *orchLoggerAdapter) Info(msg string, fields ...interface{}) {
	l.log(l.logger.Info(), msg, fields...)
}

func (l *orchLoggerAdapter) Debug(msg string, fields ...interface{}) {
	l.log(l.logger.Debug(), msg, fields...)
}

func (l *orchLoggerAdapter) Error(msg string, err error, fields ...interface{}) {
	l.log(l.logger.Error().Err(err), msg, fields...)
}

func (l *orchLoggerAdapter) log(ev *zerolog.Event, msg string, fields ...interface{}) {
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			if key, ok := fields[i].(string); ok {
				ev.Interface(key, fields[i+1])
			}
		}
	}
	ev.Msg(msg)
}
