package toolexecutor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/sandbox"
	"github.com/rs/zerolog/log"
	"github.com/xeipuuv/gojsonschema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// ToolPolicy defines which tools an agent can use
type ToolPolicy struct {
	Allow []string `json:"allow"` // List of allowed tools (* for all)
	Deny  []string `json:"deny"`  // List of denied tools (overrides allow)
}

// IsToolAllowed checks if a tool is allowed by the policy
func (tp *ToolPolicy) IsToolAllowed(toolName string) bool {
	if tp == nil {
		// No policy means allow all
		return true
	}

	// Check deny list first (overrides allow list)
	for _, denied := range tp.Deny {
		if denied == toolName || denied == "*" {
			return false
		}
	}

	// Check allow list
	for _, allowed := range tp.Allow {
		if allowed == toolName || allowed == "*" {
			return true
		}
	}

	// If no explicit allow, deny by default
	return false
}

// ToolParameter defines a parameter for a tool
type ToolParameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// ToolDefinition defines a tool's metadata and handler
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  []ToolParameter `json:"parameters"`
	Category    ToolCategory    `json:"category,omitempty"`
	Handler     ToolHandler     `json:"-"`
	PluginID    string          `json:"plugin_id,omitempty"`   // Plugin ID if this is a plugin tool
	Permissions []string        `json:"permissions,omitempty"` // Plugin permissions for approval decisions
}

// ToolHandler is the function signature for tool execution
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// ExecutionContext provides runtime information for tool execution
type ExecutionContext struct {
	SessionKey    string
	WorkingDir    string
	Timeout       time.Duration
	SandboxPolicy map[string]interface{}
	AgentID       string // Agent ID for policy enforcement
	ToolPolicy    *ToolPolicy
}

// RetryConfig controls retry behavior for transient tool execution failures.
type RetryConfig struct {
	Enabled        bool
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success   bool                   `json:"success"`
	Output    interface{}            `json:"output,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Truncated bool                   `json:"truncated,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolExecutor manages and executes tools
type ToolExecutor struct {
	tools           map[string]*ToolDefinition
	schemas         map[string]*gojsonschema.Schema
	sandboxManager  *SandboxManager
	approvalManager *ApprovalManager
	pluginRuntime   PluginToolExecutor
	policyEngine    *PolicyEngine
	policyEvaluator *PolicyEvaluator
	toolRegistry    *ToolRegistry
	retryConfig     RetryConfig
	mu              sync.RWMutex
}

// New creates a new ToolExecutor
func New() *ToolExecutor {
	observability.EnsureRegistered()

	te := &ToolExecutor{
		tools:           make(map[string]*ToolDefinition),
		schemas:         make(map[string]*gojsonschema.Schema),
		sandboxManager:  nil,
		approvalManager: nil, // Approval is optional
		pluginRuntime:   nil, // Plugin runtime is optional
		policyEngine:    NewPolicyEngine(),
		toolRegistry:    NewToolRegistry(),
		retryConfig: RetryConfig{
			Enabled:        true,
			MaxAttempts:    3,
			InitialBackoff: 250 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
		},
	}
	te.policyEvaluator = NewPolicyEvaluator(te.policyEngine)

	log.Info().Msg("Tool executor initialized")

	return te
}

// PluginToolProvider is an interface for plugin tool execution
type PluginToolProvider interface {
	ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error)
}

// PluginToolExecutor is an interface for executing plugin tools
type PluginToolExecutor interface {
	GetPlugin(pluginID string) (interface{}, error)
}

// PluginInfo provides metadata about a plugin for approval decisions
type PluginInfo interface {
	GetID() string
	GetPermissions() []string
}

// RegisterPluginTool registers a single plugin tool with conflict resolution
func (te *ToolExecutor) RegisterPluginTool(pluginID string, toolDef ToolDefinition, pluginRuntime PluginToolExecutor) error {
	// Check for name conflicts
	originalName := toolDef.Name
	toolName := originalName

	te.mu.Lock()
	defer te.mu.Unlock()

	// If tool name already exists, prefix with plugin name
	if _, exists := te.tools[toolName]; exists {
		toolName = pluginID + "_" + originalName
		log.Warn().
			Str("original_name", originalName).
			Str("prefixed_name", toolName).
			Str("plugin", pluginID).
			Msg("Tool name conflict resolved by prefixing with plugin name")
	}

	// Get plugin permissions for approval workflow
	var pluginPermissions []string
	plugin, err := pluginRuntime.GetPlugin(pluginID)
	if err == nil {
		if pluginInfo, ok := plugin.(PluginInfo); ok {
			pluginPermissions = pluginInfo.GetPermissions()
		}
	}

	// Create a handler that routes to the plugin through PluginRuntime
	handler := func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		log.Debug().
			Str("plugin", pluginID).
			Str("tool", originalName).
			Msg("Routing tool execution to plugin")

		// Get plugin provider from runtime
		plugin, err := pluginRuntime.GetPlugin(pluginID)
		if err != nil {
			return nil, fmt.Errorf("failed to get plugin %s: %w", pluginID, err)
		}

		// Type assert to PluginToolProvider interface
		provider, ok := plugin.(PluginToolProvider)
		if !ok {
			return nil, fmt.Errorf("plugin %s does not implement PluginToolProvider interface", pluginID)
		}

		// Execute tool through the plugin provider with timeout handling
		result, err := provider.ExecuteTool(ctx, originalName, params)
		if err != nil {
			// Check if error is due to context cancellation or timeout
			if ctx.Err() != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("plugin tool execution timeout: %w", err)
				}
				return nil, fmt.Errorf("plugin tool execution cancelled: %w", err)
			}
			return nil, fmt.Errorf("plugin %s tool %s execution failed: %w", pluginID, originalName, err)
		}

		log.Debug().
			Str("plugin", pluginID).
			Str("tool", originalName).
			Msg("Plugin tool execution completed")

		return result, nil
	}

	// Create tool definition with the handler and plugin metadata
	category := toolDef.Category
	if category == "" {
		category = inferToolCategory(toolName, toolDef.Description)
	}
	wrappedDef := ToolDefinition{
		Name:        toolName,
		Description: fmt.Sprintf("[Plugin: %s] %s", pluginID, toolDef.Description),
		Parameters:  toolDef.Parameters,
		Category:    category,
		Handler:     handler,
		PluginID:    pluginID,
		Permissions: pluginPermissions,
	}

	// Validate and register
	if err := te.validateToolDefinition(wrappedDef); err != nil {
		return fmt.Errorf("invalid plugin tool definition: %w", err)
	}

	schema, err := te.generateJSONSchema(wrappedDef)
	if err != nil {
		return fmt.Errorf("failed to generate schema for plugin tool: %w", err)
	}

	te.tools[toolName] = &wrappedDef
	te.schemas[toolName] = schema
	_ = te.toolRegistry.Register(toolName, wrappedDef.Description, wrappedDef.Category)

	log.Info().
		Str("tool", toolName).
		Str("plugin", pluginID).
		Strs("permissions", pluginPermissions).
		Msg("Plugin tool registered")

	return nil
}

// SetSandboxManager sets the sandbox manager for the tool executor
func (te *ToolExecutor) SetSandboxManager(manager *SandboxManager) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.sandboxManager = manager
}

// SetApprovalManager sets the approval manager for the tool executor
func (te *ToolExecutor) SetApprovalManager(manager *ApprovalManager) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.approvalManager = manager
	log.Info().Msg("Approval manager configured for tool executor")
}

// SetRetryConfig updates retry behavior for transient tool errors.
func (te *ToolExecutor) SetRetryConfig(cfg RetryConfig) {
	te.mu.Lock()
	defer te.mu.Unlock()

	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 250 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 2 * time.Second
	}
	te.retryConfig = cfg
}

// RegisterTool registers a new tool
func (te *ToolExecutor) RegisterTool(def ToolDefinition) error {
	if def.Category == "" {
		def.Category = inferToolCategory(def.Name, def.Description)
	}

	// Validate tool definition
	if err := te.validateToolDefinition(def); err != nil {
		return fmt.Errorf("invalid tool definition: %w", err)
	}

	// Generate JSON Schema
	schema, err := te.generateJSONSchema(def)
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	te.mu.Lock()
	defer te.mu.Unlock()

	te.tools[def.Name] = &def
	te.schemas[def.Name] = schema
	_ = te.toolRegistry.Register(def.Name, def.Description, def.Category)

	log.Info().Str("tool", def.Name).Msg("Tool registered")

	return nil
}

// UnregisterTool removes a tool
func (te *ToolExecutor) UnregisterTool(name string) {
	te.mu.Lock()
	defer te.mu.Unlock()

	delete(te.tools, name)
	delete(te.schemas, name)
	if te.toolRegistry != nil {
		delete(te.toolRegistry.tools, name)
		delete(te.toolRegistry.categories, name)
	}

	log.Info().Str("tool", name).Msg("Tool unregistered")
}

// GetTool returns a tool definition by name
func (te *ToolExecutor) GetTool(name string) *ToolDefinition {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return te.tools[name]
}

// ListTools returns all registered tool names
func (te *ToolExecutor) ListTools() []string {
	te.mu.RLock()
	defer te.mu.RUnlock()

	tools := make([]string, 0, len(te.tools))
	for name := range te.tools {
		tools = append(tools, name)
	}

	return tools
}

// GetToolCount returns the number of registered tools
func (te *ToolExecutor) GetToolCount() int {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return len(te.tools)
}

// Execute executes a tool with the given parameters
func (te *ToolExecutor) Execute(ctx context.Context, toolName string, params map[string]interface{}, execCtx *ExecutionContext) ToolResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracing.GetTraceID(ctx) == "" {
		ctx = tracing.NewRequestContext(ctx)
	}
	if tracing.GetRunID(ctx) == "" {
		ctx = tracing.WithRunID(ctx, tracing.NewRunID())
	}
	if execCtx != nil && execCtx.SessionKey != "" && tracing.GetSessionKey(ctx) == "" {
		ctx = tracing.WithSessionKey(ctx, execCtx.SessionKey)
	}

	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.toolexecutor",
		"toolexecutor.execute",
		attribute.String("tool", toolName),
	)
	defer span.End()

	logger := tracing.LoggerFromContext(ctx, log.Logger)
	startTime := time.Now()
	tool, schema, category, sandboxManager := te.lookup(toolName)

	if tool == nil {
		logger.Error().Str("tool", toolName).Msg("Tool not found")
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.SetStatus(codes.Error, "tool not found")
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", toolName),
		}
	}

	agentID, policy := policyContext(execCtx)
	eval := te.policyEvaluator.Evaluate(toolName, policy, agentID)
	decision := "allow"
	if !eval.Allowed {
		decision = "deny"
	}
	logger.Info().
		Str("event", "tool:policy_decision").
		Str("tool", toolName).
		Str("category", string(category)).
		Str("agent_id", agentID).
		Str("decision", decision).
		Str("reason", eval.Reason).
		Msg("tool:policy_decision")

	if !eval.Allowed {
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.SetStatus(codes.Error, "tool blocked by policy")
		logger.Warn().
			Str("event", "tool:denied").
			Str("tool", toolName).
			Str("category", string(category)).
			Str("agent_id", agentID).
			Str("reason", eval.Reason).
			Msg("tool:denied")

		metadata := cloneMetadata(eval.Metadata)
		metadata["reason"] = eval.Reason
		metadata["decision"] = decision
		metadata["category"] = string(category)

		return ToolResult{
			Success:  false,
			Error:    fmt.Sprintf("tool '%s' is not allowed by agent policy: %s", toolName, eval.Reason),
			Metadata: metadata,
		}
	}

	// Validate parameters
	if err := te.validateParameters(schema, params); err != nil {
		logger.Error().Str("tool", toolName).Err(err).Msg("Parameter validation failed")
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.RecordError(err)
		span.SetStatus(codes.Error, "parameter validation failed")
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("parameter validation failed: %v", err),
		}
	}

	if err := validateWorkspacePaths(execCtx, params); err != nil {
		logger.Error().Str("tool", toolName).Err(err).Msg("Workspace path validation failed")
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.RecordError(err)
		span.SetStatus(codes.Error, "workspace path validation failed")
		return ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	// Apply timeout
	timeout := 30 * time.Second
	if execCtx != nil && execCtx.Timeout > 0 {
		timeout = execCtx.Timeout
	}

	// Enforce plugin approval workflow for sensitive plugin tools.
	if tool.PluginID != "" && te.requiresApproval(tool) {
		te.mu.RLock()
		approvalManager := te.approvalManager
		te.mu.RUnlock()

		if approvalManager != nil {
			approvalReq := ApprovalRequest{
				Command: toolName,
				Cwd:     "",
				Context: map[string]string{
					"plugin": tool.PluginID,
					"tool":   toolName,
				},
			}
			if execCtx != nil {
				approvalReq.Cwd = execCtx.WorkingDir
				approvalReq.AgentID = execCtx.AgentID
			}

			approved, approvalErr := approvalManager.RequestApproval(ctx, approvalReq)
			if approvalErr != nil {
				duration := time.Since(startTime)
				observability.RecordToolExecution(toolName, duration, false)
				span.RecordError(approvalErr)
				span.SetStatus(codes.Error, "plugin tool approval failed")
				return ToolResult{
					Success: false,
					Error:   fmt.Sprintf("plugin tool approval failed: %v", approvalErr),
					Metadata: map[string]interface{}{
						"duration":        duration.Milliseconds(),
						"plugin":          tool.PluginID,
						"approval_failed": true,
					},
				}
			}

			if !approved {
				duration := time.Since(startTime)
				observability.RecordToolExecution(toolName, duration, false)
				span.SetStatus(codes.Error, "plugin tool execution denied by user")
				return ToolResult{
					Success: false,
					Error:   "plugin tool execution denied by user",
					Metadata: map[string]interface{}{
						"duration":        duration.Milliseconds(),
						"plugin":          tool.PluginID,
						"approval_denied": true,
					},
				}
			}
		}
	}

	if isRiskyCategory(category) {
		if sandboxManager == nil {
			duration := time.Since(startTime)
			observability.RecordToolExecution(toolName, duration, false)
			err := fmt.Errorf("sandbox manager is required for risky tool category '%s'", category)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return ToolResult{
				Success: false,
				Error:   err.Error(),
				Metadata: map[string]interface{}{
					"category": string(category),
					"decision": "deny",
					"reason":   "sandbox manager not configured",
				},
			}
		}

		if err := te.executeSandboxGate(ctx, sandboxManager, toolName, category, execCtx, timeout); err != nil {
			duration := time.Since(startTime)
			observability.RecordToolExecution(toolName, duration, false)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return ToolResult{
				Success: false,
				Error:   err.Error(),
				Metadata: map[string]interface{}{
					"category": string(category),
					"decision": "deny",
					"reason":   err.Error(),
				},
			}
		}
	}

	logger.Info().
		Str("event", "tool:execute").
		Str("tool", toolName).
		Str("category", string(category)).
		Msg("tool:execute")

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute tool
	resultChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		result, err := tool.Handler(timeoutCtx, params)
		if err != nil {
			errChan <- err
		} else {
			resultChan <- result
		}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, true)

		// Truncate output if too large
		output, truncated := te.truncateOutput(result)

		logger.Debug().
			Str("tool", toolName).
			Dur("duration", duration).
			Bool("truncated", truncated).
			Msg("Tool execution completed")

		return ToolResult{
			Success:   true,
			Output:    output,
			Truncated: truncated,
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
				"plugin":   tool.PluginID,
				"category": string(category),
				"decision": decision,
				"reason":   eval.Reason,
			},
		}

	case err := <-errChan:
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		logger.Error().
			Str("tool", toolName).
			Dur("duration", duration).
			Err(err).
			Msg("Tool execution failed")

		return ToolResult{
			Success: false,
			Error:   err.Error(),
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
				"plugin":   tool.PluginID,
				"category": string(category),
				"decision": decision,
				"reason":   err.Error(),
			},
		}

	case <-timeoutCtx.Done():
		duration := time.Since(startTime)
		observability.RecordToolExecution(toolName, duration, false)
		span.RecordError(timeoutCtx.Err())
		span.SetStatus(codes.Error, "tool execution timeout")

		logger.Error().
			Str("tool", toolName).
			Dur("duration", duration).
			Msg("Tool execution timeout")

		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool execution timeout after %v", timeout),
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
				"plugin":   tool.PluginID,
				"category": string(category),
				"decision": decision,
				"reason":   "timeout",
			},
		}
	}
}

// ExecuteWithRetry runs tool execution with retry/backoff for transient failures.
func (te *ToolExecutor) ExecuteWithRetry(ctx context.Context, toolName string, params map[string]interface{}, execCtx *ExecutionContext) ToolResult {
	if ctx == nil {
		ctx = context.Background()
	}

	te.mu.RLock()
	retryCfg := te.retryConfig
	te.mu.RUnlock()

	if !retryCfg.Enabled || retryCfg.MaxAttempts <= 1 {
		return te.Execute(ctx, toolName, params, execCtx)
	}

	var lastResult ToolResult
	for attempt := 1; attempt <= retryCfg.MaxAttempts; attempt++ {
		result := te.Execute(ctx, toolName, params, execCtx)
		if result.Success {
			if attempt > 1 {
				if result.Metadata == nil {
					result.Metadata = make(map[string]interface{})
				}
				result.Metadata["retry_attempts"] = attempt - 1
			}
			return result
		}

		lastResult = result
		if !isRetryableToolError(result.Error) || attempt == retryCfg.MaxAttempts {
			if attempt > 1 {
				if lastResult.Metadata == nil {
					lastResult.Metadata = make(map[string]interface{})
				}
				lastResult.Metadata["retry_attempts"] = attempt - 1
			}
			return lastResult
		}

		backoff := retryCfg.InitialBackoff * time.Duration(1<<(attempt-1))
		if backoff > retryCfg.MaxBackoff {
			backoff = retryCfg.MaxBackoff
		}

		select {
		case <-ctx.Done():
			cancelled := ToolResult{
				Success: false,
				Error:   ctx.Err().Error(),
				Metadata: map[string]interface{}{
					"retry_attempts": attempt - 1,
				},
			}
			return cancelled
		case <-time.After(backoff):
		}
	}

	return lastResult
}

func (te *ToolExecutor) lookup(toolName string) (*ToolDefinition, *gojsonschema.Schema, ToolCategory, *SandboxManager) {
	te.mu.RLock()
	defer te.mu.RUnlock()

	tool := te.tools[toolName]
	schema := te.schemas[toolName]
	category := CategoryGeneral
	if tool != nil {
		if tool.Category != "" {
			category = tool.Category
		} else if te.toolRegistry != nil {
			if registered, err := te.toolRegistry.GetCategory(toolName); err == nil {
				category = registered
			} else {
				category = inferToolCategory(toolName, tool.Description)
			}
		} else {
			category = inferToolCategory(toolName, tool.Description)
		}
	}

	return tool, schema, category, te.sandboxManager
}

func policyContext(execCtx *ExecutionContext) (string, *ToolPolicy) {
	if execCtx == nil {
		return "", nil
	}
	return execCtx.AgentID, execCtx.ToolPolicy
}

func cloneMetadata(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}

	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func isRiskyCategory(category ToolCategory) bool {
	switch category {
	case CategoryShell, CategoryWrite, CategoryWeb:
		return true
	default:
		return false
	}
}

func (te *ToolExecutor) executeSandboxGate(
	ctx context.Context,
	sandboxManager *SandboxManager,
	toolName string,
	category ToolCategory,
	execCtx *ExecutionContext,
	timeout time.Duration,
) error {
	logger := tracing.LoggerFromContext(ctx, log.Logger)
	sandboxKey := sandboxKeyFromContext(execCtx)
	if sandboxKey == "" {
		sandboxKey = "global"
	}

	logger.Info().
		Str("event", "sandbox:execute").
		Str("tool", toolName).
		Str("category", string(category)).
		Str("sandbox_key", sandboxKey).
		Msg("sandbox:execute")

	req := sandbox.ExecuteRequest{
		Command: "true",
		Timeout: timeout,
	}
	if execCtx != nil {
		req.WorkingDir = execCtx.WorkingDir
	}

	if _, err := sandboxManager.ExecuteInSandbox(ctx, sandboxKey, req); err != nil {
		return fmt.Errorf("sandbox execution failed for tool '%s': %w", toolName, err)
	}

	return nil
}

func sandboxKeyFromContext(execCtx *ExecutionContext) string {
	if execCtx == nil {
		return ""
	}

	scope := "session"
	if execCtx.SandboxPolicy != nil {
		if rawScope, ok := execCtx.SandboxPolicy["scope"].(string); ok && rawScope != "" {
			scope = rawScope
		}
	}

	if scope == "agent" && execCtx.AgentID != "" {
		return execCtx.AgentID
	}
	if execCtx.SessionKey != "" {
		return execCtx.SessionKey
	}
	return execCtx.AgentID
}

func inferToolCategory(name string, description string) ToolCategory {
	normalizedName := strings.ToLower(name)
	normalizedDescription := strings.ToLower(description)

	if strings.HasPrefix(normalizedName, "browser_") || strings.Contains(normalizedDescription, "browser") {
		return CategoryWeb
	}
	if normalizedName == "memory_write" || normalizedName == "memory_delete" {
		return CategoryWrite
	}
	if normalizedName == "memory_search" || normalizedName == "memory_list" {
		return CategoryRead
	}
	return CategoryGeneral
}

func validateWorkspacePaths(execCtx *ExecutionContext, params map[string]interface{}) error {
	if execCtx == nil || strings.TrimSpace(execCtx.WorkingDir) == "" {
		return nil
	}

	workspaceAbs, err := filepath.Abs(execCtx.WorkingDir)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace path: %w", err)
	}
	workspaceAbs = filepath.Clean(workspaceAbs)

	return validateWorkspaceValues(params, workspaceAbs)
}

func validateWorkspaceValues(value interface{}, workspaceAbs string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			if isPathLikeParamKey(key) {
				if err := validatePathValueWithinWorkspace(nested, workspaceAbs, key); err != nil {
					return err
				}
				continue
			}
			if err := validateWorkspaceValues(nested, workspaceAbs); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if err := validateWorkspaceValues(nested, workspaceAbs); err != nil {
				return err
			}
		}
	}

	return nil
}

func validatePathValueWithinWorkspace(value interface{}, workspaceAbs string, key string) error {
	switch typed := value.(type) {
	case string:
		return validatePathWithinWorkspace(typed, workspaceAbs, key)
	case []interface{}:
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				continue
			}
			if err := validatePathWithinWorkspace(str, workspaceAbs, key); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for nestedKey, nestedValue := range typed {
			if err := validatePathValueWithinWorkspace(nestedValue, workspaceAbs, nestedKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePathWithinWorkspace(pathValue string, workspaceAbs string, key string) error {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return nil
	}
	if strings.Contains(pathValue, "://") {
		return nil
	}

	var candidate string
	if filepath.IsAbs(pathValue) {
		candidate = filepath.Clean(pathValue)
	} else {
		candidate = filepath.Clean(filepath.Join(workspaceAbs, pathValue))
	}

	rel, err := filepath.Rel(workspaceAbs, candidate)
	if err != nil {
		return fmt.Errorf("failed to validate workspace path %q: %w", pathValue, err)
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q for parameter %q is outside workspace %q", pathValue, key, workspaceAbs)
	}
	return nil
}

func isPathLikeParamKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "path", "paths", "file", "files", "filepath", "file_path", "target", "source", "destination", "output", "input", "cwd", "dir", "directory", "root":
		return true
	default:
		return false
	}
}

func isRetryableToolError(errText string) bool {
	normalized := strings.ToLower(strings.TrimSpace(errText))
	if normalized == "" {
		return false
	}

	retryMarkers := []string{
		"timeout",
		"timed out",
		"temporary",
		"temporarily unavailable",
		"connection reset",
		"connection refused",
		"network is unreachable",
		"eof",
		"broken pipe",
		"i/o timeout",
		"deadline exceeded",
		"too many requests",
		"rate limit",
		"429",
		"500",
		"502",
		"503",
		"504",
	}

	for _, marker := range retryMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}

	return false
}

// validateToolDefinition validates a tool definition
func (te *ToolExecutor) validateToolDefinition(def ToolDefinition) error {
	if def.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if def.Description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}
	if def.Handler == nil {
		return fmt.Errorf("tool handler cannot be nil")
	}

	// Validate parameters
	for _, param := range def.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter name cannot be empty")
		}
		if param.Type == "" {
			return fmt.Errorf("parameter type cannot be empty for %s", param.Name)
		}
		if param.Description == "" {
			return fmt.Errorf("parameter description cannot be empty for %s", param.Name)
		}

		// Validate type
		validTypes := map[string]bool{
			"string": true, "number": true, "boolean": true,
			"object": true, "array": true, "integer": true,
		}
		if !validTypes[param.Type] {
			return fmt.Errorf("invalid parameter type %s for %s", param.Type, param.Name)
		}
	}

	return nil
}

// generateJSONSchema generates a JSON Schema from tool parameters
func (te *ToolExecutor) generateJSONSchema(def ToolDefinition) (*gojsonschema.Schema, error) {
	schemaMap := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           make(map[string]interface{}),
	}

	properties := schemaMap["properties"].(map[string]interface{})
	required := []string{}

	for _, param := range def.Parameters {
		paramSchema := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}

		if param.Default != nil {
			paramSchema["default"] = param.Default
		}

		properties[param.Name] = paramSchema

		if param.Required {
			required = append(required, param.Name)
		}
	}

	if len(required) > 0 {
		schemaMap["required"] = required
	}

	schemaLoader := gojsonschema.NewGoLoader(schemaMap)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// validateParameters validates parameters against a JSON Schema
func (te *ToolExecutor) validateParameters(schema *gojsonschema.Schema, params map[string]interface{}) error {
	if schema == nil {
		return nil
	}

	paramsLoader := gojsonschema.NewGoLoader(params)
	result, err := schema.Validate(paramsLoader)
	if err != nil {
		return err
	}

	if !result.Valid() {
		errors := []string{}
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		return fmt.Errorf("validation errors: %v", errors)
	}

	return nil
}

// truncateOutput truncates output if it exceeds the size limit
func (te *ToolExecutor) truncateOutput(output interface{}) (interface{}, bool) {
	const maxSize = 10 * 1024 // 10KB

	// Convert to string for size check
	str := fmt.Sprintf("%v", output)

	if len(str) <= maxSize {
		return output, false
	}

	truncated := str[:maxSize] + "\n... [output truncated]"
	log.Warn().
		Int("original", len(str)).
		Int("truncated", maxSize).
		Msg("Output truncated")

	return truncated, true
}

// requiresApproval determines if a plugin tool requires approval based on its permissions
// Sensitive permissions that require approval:
// - filesystem:write - Can modify files
// - process:spawn - Can execute processes
// - database:write - Can modify database
// - network:http - Can make external HTTP requests
// - network:websocket - Can establish WebSocket connections
func (te *ToolExecutor) requiresApproval(tool *ToolDefinition) bool {
	if tool.PluginID == "" {
		// Not a plugin tool, no approval needed
		return false
	}

	sensitivePermissions := map[string]bool{
		"filesystem:write":  true,
		"process:spawn":     true,
		"database:write":    true,
		"network:http":      true,
		"network:websocket": true,
	}

	for _, perm := range tool.Permissions {
		if sensitivePermissions[perm] {
			return true
		}
	}

	return false
}
