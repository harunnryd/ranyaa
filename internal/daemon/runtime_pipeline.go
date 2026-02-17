package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/harun/ranya/pkg/orchestrator"
	"github.com/harun/ranya/pkg/planner"
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
}

func (d *Daemon) dispatchGatewayRequest(ctx context.Context, req gateway.AgentDispatchRequest) (agent.AgentResult, error) {
	result, err := d.router.RouteMessageAndWait(ctx, Message{
		SessionKey: req.SessionKey,
		Source:     req.Source,
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

	agentCfg, err := d.resolveAgentConfig(req.AgentID)
	if err != nil {
		return agent.AgentResult{}, nil, err
	}
	ctx = tracing.WithAgentID(ctx, agentCfg.ID)

	runCfg := mergeRunConfig(agentCfg, req.RunConfig)
	plan, err := d.generateRuntimePlan(req, runCfg)
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

	for i := range plan.Steps {
		step := &plan.Steps[i]
		step.Status = planner.StepStatusRunning
		stepStart := time.Now()

		stepPrompt, stepCfg := buildStepInput(req.Prompt, previousOutput, runCfg, i, len(plan.Steps))
		result, runErr := d.dispatchPlannedStep(ctx, req, agentCfg, stepPrompt, stepCfg)
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

		previousOutput = result.Response
		finalResult = result
	}

	return finalResult, plan, nil
}

func (d *Daemon) dispatchPlannedStep(
	ctx context.Context,
	req RuntimeRequest,
	agentCfg config.AgentConfig,
	prompt string,
	runCfg agent.AgentConfig,
) (agent.AgentResult, error) {
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

	result, runErr := d.agentRunner.RunWithContext(ctx, agent.AgentRunParams{
		Prompt:        prompt,
		SessionKey:    req.SessionKey,
		Config:        runCfg,
		CWD:           req.CWD,
		AgentID:       agentCfg.ID,
		ToolPolicy:    toolPolicy,
		SandboxPolicy: sandboxPolicy,
	})

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

func (d *Daemon) resolveAgentConfig(agentID string) (config.AgentConfig, error) {
	if len(d.config.Agents) == 0 {
		return config.AgentConfig{}, fmt.Errorf("no agents configured")
	}

	if agentID == "" {
		agentID = "default"
	}

	for _, cfg := range d.config.Agents {
		if cfg.ID == agentID {
			return cfg, nil
		}
	}

	if agentID == "default" {
		return d.config.Agents[0], nil
	}

	return config.AgentConfig{}, fmt.Errorf("agent %s not found", agentID)
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

func (d *Daemon) generateRuntimePlan(req RuntimeRequest, runCfg agent.AgentConfig) (*planner.Plan, error) {
	steps := []planner.Step{{
		ID:          "step-1",
		Description: "Execute request and gather results",
	}}

	if shouldUseMultiStepPlan(req.Prompt, runCfg.Tools) {
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

func shouldUseMultiStepPlan(prompt string, tools []string) bool {
	if len(tools) > 0 {
		return true
	}

	normalized := strings.ToLower(prompt)
	markers := []string{" then ", " and then ", " after that ", " finally ", "\n1.", "\n- "}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
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
