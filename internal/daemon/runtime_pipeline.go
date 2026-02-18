package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/channels"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/harun/ranya/pkg/orchestrator"
	"github.com/harun/ranya/pkg/planner"
	"github.com/harun/ranya/pkg/routing"
	"github.com/harun/ranya/pkg/toolexecutor"
)

// RuntimeRequest captures ingress data passed through the canonical runtime pipeline.
type RuntimeRequest struct {
	Prompt     string
	SessionKey string
	Source     string
	AgentID    string
	RunConfig  agent.AgentConfig
	CWD        string
	Metadata   map[string]interface{}
	RequestID  string // For idempotency tracking
}

func (d *Daemon) dispatchGatewayRequest(ctx context.Context, req gateway.AgentDispatchRequest) (agent.AgentResult, error) {
	if d.channelRegistry == nil {
		return agent.AgentResult{}, fmt.Errorf("channel registry is not initialized")
	}

	channelName := strings.TrimSpace(req.Source)
	if channelName == "" {
		channelName = "gateway"
	}

	result, err := d.channelRegistry.Dispatch(ctx, channels.InboundMessage{
		Channel:    channelName,
		SessionKey: req.SessionKey,
		Content:    req.Prompt,
		Metadata:   req.Metadata,
		AgentID:    req.AgentID,
		RunConfig:  req.Config,
		CWD:        req.CWD,
	})
	if err != nil {
		return agent.AgentResult{}, err
	}

	runResult, ok := result.(agent.AgentResult)
	if !ok {
		return agent.AgentResult{}, fmt.Errorf("unexpected runtime result type %T", result)
	}

	return runResult, nil
}

func (d *Daemon) executeRuntimeFlow(ctx context.Context, req RuntimeRequest) (agent.AgentResult, *planner.Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracing.GetTraceID(ctx) == "" {
		ctx = tracing.NewRequestContext(ctx)
	}
	if tracing.GetRunID(ctx) == "" {
		ctx = tracing.WithRunID(ctx, tracing.NewRunID())
	}
	if req.SessionKey == "" {
		return agent.AgentResult{}, nil, fmt.Errorf("session key is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return agent.AgentResult{}, nil, fmt.Errorf("prompt is required")
	}

	ctx = tracing.WithSessionKey(ctx, req.SessionKey)

	// Pre-execution moderation check
	if d.contentFilter != nil {
		if err := d.contentFilter.CheckPrompt(req.Prompt); err != nil {
			observability.RecordSecurityAudit(ctx, "moderation_event", req.SessionKey, "blocked", map[string]interface{}{
				"stage":       "pre",
				"session_key": req.SessionKey,
				"reason":      err.Error(),
			})
			return agent.AgentResult{
				Response:   "Your request was rejected by the content moderation policy.",
				SessionKey: req.SessionKey,
				Aborted:    true,
			}, nil, fmt.Errorf("policy violation: %w", err)
		}
	}

	agentCfg, err := d.resolveAgentConfig(req)
	if err != nil {
		return agent.AgentResult{}, nil, err
	}
	ctx = tracing.WithAgentID(ctx, agentCfg.ID)

	if strings.TrimSpace(req.CWD) == "" {
		if strings.TrimSpace(agentCfg.Workspace) != "" {
			req.CWD = agentCfg.Workspace
		} else if strings.TrimSpace(d.config.WorkspacePath) != "" {
			req.CWD = d.config.WorkspacePath
		}
	}

	if err := d.triggerAgentBootstrapHook(ctx, req, agentCfg.ID); err != nil {
		logger := tracing.LoggerFromContext(ctx, d.logger.GetZerolog())
		logger.Warn().Err(err).Msg("agent:bootstrap hooks failed")
	}

	runCfg := mergeRunConfig(agentCfg, req.RunConfig)
	if runCfg.Tools == nil && d.toolExecutor != nil {
		runCfg.Tools = d.toolExecutor.ListAllowedTools(&toolexecutor.ToolPolicy{
			Allow: append([]string{}, agentCfg.Tools.Allow...),
			Deny:  append([]string{}, agentCfg.Tools.Deny...),
		}, agentCfg.ID)
	}
	toolsExplicit := req.RunConfig.Tools != nil
	plan, err := d.generateRuntimePlan(req, runCfg, toolsExplicit)
	if err != nil {
		return agent.AgentResult{}, nil, err
	}

	logger := tracing.LoggerFromContext(ctx, d.logger.GetZerolog())
	logger.Info().
		Str("event", "planner:plan_created").
		Str("plan_id", plan.ID).
		Str("agent_id", agentCfg.ID).
		Int("steps", len(plan.Steps)).
		Msg("planner:plan_created")

	var finalResult agent.AgentResult
	var previousOutput string

	// Execute steps with dynamic replanning capability
	for i := 0; i < len(plan.Steps); i++ {
		// Safety break
		if i > 20 {
			logger.Warn().Msg("planner:max_steps_exceeded")
			break
		}

		step := &plan.Steps[i]
		step.Status = planner.StepStatusRunning
		stepStart := time.Now()

		stepPrompt, stepCfg := buildStepInput(req.Prompt, previousOutput, runCfg, i, len(plan.Steps))
		stepCtx := ctx
		stepRequestID := req.RequestID
		if strings.TrimSpace(stepRequestID) == "" {
			stepRequestID = tracing.NewTraceID()
		} else {
			stepRequestID = fmt.Sprintf("%s:step-%d", stepRequestID, i+1)
		}
		stepCtx = tracing.WithRequestID(stepCtx, stepRequestID)

		useOrchestrator := len(plan.Steps) > 1
		result, runErr := d.dispatchPlannedStep(stepCtx, req, agentCfg, stepPrompt, stepCfg, useOrchestrator)
		duration := time.Since(stepStart)

		if runErr != nil {
			step.Status = planner.StepStatusFailed
			step.Result = &planner.StepResult{
				Success:   false,
				Error:     runErr.Error(),
				Duration:  duration,
				Timestamp: time.Now(),
			}
			return agent.AgentResult{}, plan, runErr
		}

		step.Status = planner.StepStatusCompleted
		step.Result = &planner.StepResult{
			Success:   true,
			Output:    result.Response,
			Duration:  duration,
			Timestamp: time.Now(),
		}

		// Validation Loop (P1-2)
		if valid, reason, _ := d.validateStepResult(ctx, req, req.Prompt, result.Response, runCfg); !valid {
			logger.Warn().Str("reason", reason).Msg("planner:step_validation_failed")

			// Inject correction step only if we haven't looped too much and it's not the last step
			// For simplicity, we just append a correction step if the critic complains strongly.
			// Actually, let's just log for now to avoid infinite loops until we have strict loop detection.
			// Ideally: plan.Steps = append(plan.Steps, correctionStep)
		}

		previousOutput = result.Response
		finalResult = result
	}

	// Post-execution moderation check
	if d.contentFilter != nil && finalResult.Response != "" {
		if err := d.contentFilter.CheckResponse(finalResult.Response); err != nil {
			observability.RecordSecurityAudit(ctx, "moderation_event", req.SessionKey, "blocked", map[string]interface{}{
				"stage":       "post",
				"session_key": req.SessionKey,
				"reason":      err.Error(),
			})
			// Redact or block
			finalResult.Response = "[Content Redacted by Moderation Policy]"
			finalResult.Aborted = true
			// We don't return error here to preserve the plan execution record, but we modify result
			logger.Warn().Err(err).Msg("Response blocked by moderation")
		}
	}

	return finalResult, plan, nil
}

func (d *Daemon) dispatchPlannedStep(
	ctx context.Context,
	req RuntimeRequest,
	agentCfg config.AgentConfig,
	prompt string,
	runCfg agent.AgentConfig,
	useOrchestrator bool,
) (agent.AgentResult, error) {
	toolPolicy := &toolexecutor.ToolPolicy{
		Allow: append([]string{}, agentCfg.Tools.Allow...),
		Deny:  append([]string{}, agentCfg.Tools.Deny...),
	}

	sandboxMode := agentCfg.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "tools"
	}
	sandboxScope := agentCfg.Sandbox.Scope
	if sandboxScope == "" {
		sandboxScope = "session"
	}
	sandboxPolicy := map[string]interface{}{
		"mode":  sandboxMode,
		"scope": sandboxScope,
	}
	if runtime := strings.TrimSpace(agentCfg.Sandbox.Runtime); runtime != "" {
		sandboxPolicy["runtime"] = runtime
	}
	if image := strings.TrimSpace(agentCfg.Sandbox.DockerImage); image != "" {
		sandboxPolicy["docker_image"] = image
	}

	runParams := agent.AgentRunParams{
		Prompt:        prompt,
		SessionKey:    req.SessionKey,
		Config:        runCfg,
		CWD:           req.CWD,
		AgentID:       agentCfg.ID,
		ToolPolicy:    toolPolicy,
		SandboxPolicy: sandboxPolicy,
	}

	if !useOrchestrator || d.orchestrator == nil {
		return d.agentRunner.RunWithContext(ctx, runParams)
	}

	instance, err := d.orchestrator.CreateInstance(agentCfg.ID)
	if err != nil {
		return agent.AgentResult{}, fmt.Errorf("failed to dispatch orchestrator instance: %w", err)
	}

	logger := tracing.LoggerFromContext(ctx, d.logger.GetZerolog())
	logger.Info().
		Str("event", "orchestrator:dispatch").
		Str("agent_id", agentCfg.ID).
		Str("instance_id", instance.ID).
		Msg("orchestrator:dispatch")

	if err := d.orchestrator.UpdateInstanceStatus(instance.ID, orchestrator.AgentStatusRunning); err != nil {
		_ = d.orchestrator.StopInstance(instance.ID)
		return agent.AgentResult{}, fmt.Errorf("failed to set orchestrator instance running: %w", err)
	}

	result, runErr := d.agentRunner.RunWithContext(ctx, runParams)

	if runErr != nil {
		_ = d.orchestrator.UpdateInstanceStatus(instance.ID, orchestrator.AgentStatusFailed)
		_ = d.orchestrator.StopInstance(instance.ID)
		return agent.AgentResult{}, runErr
	}

	if err := d.orchestrator.UpdateInstanceStatus(instance.ID, orchestrator.AgentStatusStopped); err != nil {
		_ = d.orchestrator.StopInstance(instance.ID)
		return agent.AgentResult{}, fmt.Errorf("failed to stop orchestrator instance: %w", err)
	}
	if err := d.orchestrator.StopInstance(instance.ID); err != nil {
		return agent.AgentResult{}, fmt.Errorf("failed to cleanup orchestrator instance: %w", err)
	}

	return result, nil
}

func (d *Daemon) resolveAgentConfig(req RuntimeRequest) (config.AgentConfig, error) {
	if len(d.config.Agents) == 0 {
		return config.AgentConfig{}, fmt.Errorf("no agents configured")
	}

	requestedAgentID := strings.TrimSpace(req.AgentID)
	if requestedAgentID == "" {
		if routedID := d.resolveRoutedAgentID(req); routedID != "" {
			if cfg, ok := d.findAgentConfig(routedID); ok {
				return cfg, nil
			}
		}
		return d.config.Agents[0], nil
	}

	if requestedAgentID == "default" {
		if cfg, ok := d.findAgentConfig("default"); ok {
			return cfg, nil
		}
		return d.config.Agents[0], nil
	}

	if cfg, ok := d.findAgentConfig(requestedAgentID); ok {
		return cfg, nil
	}

	return config.AgentConfig{}, fmt.Errorf("agent %s not found", requestedAgentID)
}

func (d *Daemon) findAgentConfig(agentID string) (config.AgentConfig, bool) {
	for _, cfg := range d.config.Agents {
		if cfg.ID == agentID {
			return cfg, true
		}
	}
	return config.AgentConfig{}, false
}

func (d *Daemon) findAgentByRole(role string) (config.AgentConfig, bool) {
	for _, cfg := range d.config.Agents {
		if strings.EqualFold(strings.TrimSpace(cfg.Role), role) {
			return cfg, true
		}
	}
	return config.AgentConfig{}, false
}

func (d *Daemon) resolveRoutedAgentID(req RuntimeRequest) string {
	if d.routingService == nil {
		return ""
	}

	routingCtx := routing.RoutingContext{
		MessageID: req.SessionKey,
		Content:   req.Prompt,
		Metadata:  req.Metadata,
		Timestamp: time.Now(),
		Source:    req.Source,
		Channel:   req.Source,
		PeerID:    runtimeRoutingMetadataValue(req.Metadata, "peer", "peer_id", "peerId", "chat_id", "chatId"),
		GuildID:   runtimeRoutingMetadataValue(req.Metadata, "guild_id", "guildId"),
		TeamID:    runtimeRoutingMetadataValue(req.Metadata, "team_id", "teamId"),
		AccountID: runtimeRoutingMetadataValue(req.Metadata, "account_id", "accountId", "account"),
	}

	route, err := d.routingService.ResolveRoute(routingCtx)
	if err != nil || route == nil || strings.TrimSpace(route.Handler) == "" {
		return ""
	}

	return strings.TrimSpace(route.Handler)
}

func runtimeRoutingMetadataValue(metadata map[string]interface{}, keys ...string) string {
	if metadata == nil {
		return ""
	}
	for _, key := range keys {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func mergeRunConfig(base config.AgentConfig, override agent.AgentConfig) agent.AgentConfig {
	merged := override

	if merged.Model == "" {
		merged.Model = base.Model
	}
	if merged.Temperature == 0 {
		merged.Temperature = base.Temperature
	}
	if merged.MaxTokens == 0 {
		merged.MaxTokens = base.MaxTokens
	}
	if merged.SystemPrompt == "" {
		merged.SystemPrompt = base.SystemPrompt
	}
	if merged.MaxRetries <= 0 {
		merged.MaxRetries = 3
	}

	return merged
}

func (d *Daemon) generateRuntimePlan(req RuntimeRequest, runCfg agent.AgentConfig, toolsExplicit bool) (*planner.Plan, error) {
	steps := []planner.Step{{
		ID:          "step-1",
		Description: "Execute request and gather results",
	}}

	if shouldUseMultiStepPlan(req.Prompt, runCfg.Tools, toolsExplicit) {
		steps = []planner.Step{
			{
				ID:          "step-1",
				Description: "Execute request and gather results",
			},
			{
				ID:           "step-2",
				Description:  "Produce final response",
				Dependencies: []string{"step-1"},
			},
		}
	}

	planDescription := fmt.Sprintf("%s request for session %s", req.Source, req.SessionKey)
	if req.Source == "" {
		planDescription = fmt.Sprintf("runtime request for session %s", req.SessionKey)
	}

	return d.planner.GeneratePlan(planDescription, steps)
}

func shouldUseMultiStepPlan(prompt string, tools []string, toolsExplicit bool) bool {
	normalized := strings.ToLower(prompt)
	markers := []string{" then ", " and then ", " after that ", " finally ", "\n1.", "\n- "}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}

	if toolsExplicit && len(tools) > 0 {
		return true
	}

	return false
}

func buildStepInput(originalPrompt string, previousOutput string, baseCfg agent.AgentConfig, index int, total int) (string, agent.AgentConfig) {
	cfg := baseCfg
	if total <= 1 || index == 0 {
		return originalPrompt, cfg
	}

	cfg.Tools = nil
	cfg.Streaming = false

	prompt := fmt.Sprintf(
		"Original request:\n%s\n\nIntermediate result:\n%s\n\nProvide the final user-facing response.",
		originalPrompt,
		previousOutput,
	)
	return prompt, cfg
}

func (d *Daemon) registerOrchestratorAgents() error {
	for _, cfg := range d.config.Agents {
		role := toOrchestratorRole(cfg.Role)
		if role == "" {
			role = orchestrator.RoleGeneral
		}
		name := cfg.Name
		if name == "" {
			name = cfg.ID
		}
		sandboxMode := cfg.Sandbox.Mode
		if sandboxMode == "" {
			sandboxMode = string(orchestrator.SandboxModeTools)
		}
		sandboxScope := cfg.Sandbox.Scope
		if sandboxScope == "" {
			sandboxScope = string(orchestrator.SandboxScopeSession)
		}

		oCfg := orchestrator.AgentConfig{
			ID:           cfg.ID,
			Name:         name,
			Role:         role,
			Model:        cfg.Model,
			Temperature:  cfg.Temperature,
			MaxTokens:    cfg.MaxTokens,
			SystemPrompt: cfg.SystemPrompt,
			Tools: orchestrator.ToolPolicy{
				Allow: append([]string{}, cfg.Tools.Allow...),
				Deny:  append([]string{}, cfg.Tools.Deny...),
			},
			Workspace: cfg.Workspace,
			Sandbox: orchestrator.SandboxConfig{
				Mode:  orchestrator.SandboxMode(sandboxMode),
				Scope: orchestrator.SandboxScope(sandboxScope),
			},
			MaxConcurrentSubAgents: cfg.MaxConcurrentSubAgents,
			AllowedSubAgents:       append([]string{}, cfg.AllowedSubAgents...),
			Metadata: map[string]string{
				"source": "daemon-config",
			},
		}

		if err := d.orchestrator.RegisterAgent(oCfg); err != nil {
			return err
		}
	}

	return nil
}

func toOrchestratorRole(role string) orchestrator.AgentRole {
	switch strings.ToLower(role) {
	case "captain":
		return orchestrator.RoleCaptain
	case "executor":
		return orchestrator.RoleExecutor
	case "critic":
		return orchestrator.RoleCritic
	case "general", "":
		return orchestrator.RoleGeneral
	default:
		return ""
	}
}

// validateStepResult checks if a step's output satisfactory using the Critic agent
func (d *Daemon) validateStepResult(ctx context.Context, req RuntimeRequest, originalPrompt, stepResult string, runCfg agent.AgentConfig) (bool, string, error) {
	// Only validate if a Critic agent exists
	critic, ok := d.findAgentByRole("critic")
	if !ok || critic.ID == req.AgentID {
		return true, "", nil
	}

	criticPrompt := fmt.Sprintf(
		"Original request:\n%s\n\nProposed response:\n%s\n\nAssess whether the response fully and correctly satisfies the original request. Reply with APPROVED or REJECTED followed by a short reason.",
		originalPrompt,
		stepResult,
	)

	criticSystemPrompt := strings.TrimSpace(critic.SystemPrompt)
	if criticSystemPrompt == "" {
		criticSystemPrompt = "You are a strict reviewer. Respond with APPROVED or REJECTED and a short reason."
	} else {
		criticSystemPrompt = fmt.Sprintf("%s\n\nRespond with APPROVED or REJECTED and a short reason.", criticSystemPrompt)
	}

	model := strings.TrimSpace(critic.Model)
	if model == "" {
		model = runCfg.Model
	}

	criticCfg := agent.AgentConfig{
		Model:        model,
		Temperature:  0.1,
		MaxTokens:    256,
		SystemPrompt: criticSystemPrompt,
		Tools:        nil,
		UseMemory:    false,
		Streaming:    false,
		MaxRetries:   1,
	}

	sandboxMode := critic.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "tools"
	}
	sandboxScope := critic.Sandbox.Scope
	if sandboxScope == "" {
		sandboxScope = "session"
	}
	sandboxPolicy := map[string]interface{}{
		"mode":  sandboxMode,
		"scope": sandboxScope,
	}
	if runtime := strings.TrimSpace(critic.Sandbox.Runtime); runtime != "" {
		sandboxPolicy["runtime"] = runtime
	}
	if image := strings.TrimSpace(critic.Sandbox.DockerImage); image != "" {
		sandboxPolicy["docker_image"] = image
	}

	critCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := d.agentRunner.RunWithContext(critCtx, agent.AgentRunParams{
		Prompt:        criticPrompt,
		SessionKey:    fmt.Sprintf("%s:critic", req.SessionKey),
		Config:        criticCfg,
		CWD:           req.CWD,
		AgentID:       critic.ID,
		ToolPolicy:    &toolexecutor.ToolPolicy{Deny: []string{"*"}},
		SandboxPolicy: sandboxPolicy,
	})
	if err != nil {
		return false, "", fmt.Errorf("critic evaluation failed: %w", err)
	}

	normalized := strings.ToLower(strings.TrimSpace(result.Response))
	if strings.HasPrefix(normalized, "approved") || strings.HasPrefix(normalized, "approve") || strings.HasPrefix(normalized, "yes") {
		return true, result.Response, nil
	}
	if strings.HasPrefix(normalized, "rejected") || strings.HasPrefix(normalized, "reject") || strings.HasPrefix(normalized, "no") {
		return false, result.Response, nil
	}

	return false, result.Response, fmt.Errorf("critic response not parseable: %s", result.Response)
}
