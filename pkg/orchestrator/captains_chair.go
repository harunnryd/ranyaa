package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CaptainsChair implements the Captain's Chair coordination pattern
// Flow: Captain → Executor → Critic → Captain (synthesis)
type CaptainsChair struct {
	orchestrator *Orchestrator
	spawner      *Spawner
	logger       Logger
}

// NewCaptainsChair creates a new Captain's Chair pattern executor
func NewCaptainsChair(orchestrator *Orchestrator, spawner *Spawner, logger Logger) *CaptainsChair {
	return &CaptainsChair{
		orchestrator: orchestrator,
		spawner:      spawner,
		logger:       logger,
	}
}

// CaptainsChairRequest defines the request for Captain's Chair execution
type CaptainsChairRequest struct {
	CaptainID  string       // ID of the captain agent
	ExecutorID string       // ID of the executor agent
	CriticID   string       // ID of the critic agent
	Task       string       // Task description
	Context    AgentContext // Shared context
	Timeout    int          // Timeout in seconds (default 900s = 15 minutes)
}

// CaptainsChairResult contains the results from all stages
type CaptainsChairResult struct {
	PlanResult     AgentResult `json:"plan_result"`
	ExecutorResult AgentResult `json:"executor_result"`
	CriticResult   AgentResult `json:"critic_result"`
	FinalResult    AgentResult `json:"final_result"`
	TotalDuration  int64       `json:"total_duration_ms"`
	Success        bool        `json:"success"`
	Error          string      `json:"error,omitempty"`
}

// Execute runs the Captain's Chair pattern
func (c *CaptainsChair) Execute(ctx context.Context, req CaptainsChairRequest) (CaptainsChairResult, error) {
	startTime := time.Now()
	result := CaptainsChairResult{}

	// Validate request
	if err := c.validateRequest(req); err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, err
	}

	// Set default timeout
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 900 // Default 15 minutes
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Generate trace ID for the entire flow
	traceID := req.Context.TraceID
	if traceID == "" {
		traceID = uuid.New().String()
	}

	if c.logger != nil {
		c.logger.Info("Starting Captain's Chair execution",
			"captain_id", req.CaptainID,
			"executor_id", req.ExecutorID,
			"critic_id", req.CriticID,
			"trace_id", traceID,
			"timeout", timeout)
	}

	// Stage 1: Captain creates plan
	planResult, err := c.captainCreatePlan(execCtx, req, traceID)
	result.PlanResult = planResult
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("plan creation failed: %v", err)
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, err
	}

	// Stage 2: Executor executes the plan
	executorResult, err := c.executorExecute(execCtx, req, planResult, traceID)
	result.ExecutorResult = executorResult
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("execution failed: %v", err)
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, err
	}

	// Stage 3: Critic reviews the execution
	criticResult, err := c.criticReview(execCtx, req, executorResult, traceID)
	result.CriticResult = criticResult
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("review failed: %v", err)
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, err
	}

	// Stage 4: Captain synthesizes final result
	finalResult, err := c.captainSynthesize(execCtx, req, executorResult, criticResult, traceID)
	result.FinalResult = finalResult
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("synthesis failed: %v", err)
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, err
	}

	result.Success = true
	result.TotalDuration = time.Since(startTime).Milliseconds()

	if c.logger != nil {
		c.logger.Info("Captain's Chair execution completed",
			"trace_id", traceID,
			"total_duration_ms", result.TotalDuration,
			"success", result.Success)
	}

	return result, nil
}

// validateRequest validates the Captain's Chair request
func (c *CaptainsChair) validateRequest(req CaptainsChairRequest) error {
	if req.CaptainID == "" {
		return fmt.Errorf("captain_id is required")
	}

	if req.ExecutorID == "" {
		return fmt.Errorf("executor_id is required")
	}

	if req.CriticID == "" {
		return fmt.Errorf("critic_id is required")
	}

	if req.Task == "" {
		return fmt.Errorf("task is required")
	}

	// Verify agents exist and have correct roles
	captain, err := c.orchestrator.GetAgent(req.CaptainID)
	if err != nil {
		return fmt.Errorf("captain agent not found: %w", err)
	}
	if captain.Role != RoleCaptain {
		return fmt.Errorf("agent %s is not a captain (role: %s)", req.CaptainID, captain.Role)
	}

	executor, err := c.orchestrator.GetAgent(req.ExecutorID)
	if err != nil {
		return fmt.Errorf("executor agent not found: %w", err)
	}
	if executor.Role != RoleExecutor {
		return fmt.Errorf("agent %s is not an executor (role: %s)", req.ExecutorID, executor.Role)
	}

	critic, err := c.orchestrator.GetAgent(req.CriticID)
	if err != nil {
		return fmt.Errorf("critic agent not found: %w", err)
	}
	if critic.Role != RoleCritic {
		return fmt.Errorf("agent %s is not a critic (role: %s)", req.CriticID, critic.Role)
	}

	return nil
}

// captainCreatePlan has the captain create a plan for the task
func (c *CaptainsChair) captainCreatePlan(ctx context.Context, req CaptainsChairRequest, traceID string) (AgentResult, error) {
	if c.logger != nil {
		c.logger.Info("Captain creating plan", "trace_id", traceID)
	}

	instructions := fmt.Sprintf(`You are the captain agent. Create a detailed plan for the following task:

Task: %s

Break down the task into clear steps that an executor agent can follow. Be specific and actionable.
Do not execute the task yourself - only create the plan.`, req.Task)

	spawnReq := SpawnRequest{
		AgentID:       req.CaptainID,
		ParentAgentID: req.CaptainID,
		Context: AgentContext{
			ParentSessionKey: req.Context.ParentSessionKey,
			SharedState:      req.Context.SharedState,
			Files:            req.Context.Files,
			Instructions:     instructions,
			TraceID:          traceID,
			RunID:            uuid.New().String(),
		},
		Timeout: 300, // 5 minutes for planning
	}

	return c.spawner.Spawn(ctx, spawnReq)
}

// executorExecute has the executor execute the plan
func (c *CaptainsChair) executorExecute(ctx context.Context, req CaptainsChairRequest, planResult AgentResult, traceID string) (AgentResult, error) {
	if c.logger != nil {
		c.logger.Info("Executor executing plan", "trace_id", traceID)
	}

	instructions := fmt.Sprintf(`You are the executor agent. Execute the following plan:

Original Task: %s

Plan from Captain:
%v

Execute the plan precisely and report your results.`, req.Task, planResult.Output)

	// Merge shared state with plan result
	sharedState := make(map[string]interface{})
	for k, v := range req.Context.SharedState {
		sharedState[k] = v
	}
	sharedState["plan"] = planResult.Output

	spawnReq := SpawnRequest{
		AgentID:       req.ExecutorID,
		ParentAgentID: req.CaptainID,
		Context: AgentContext{
			ParentSessionKey: req.Context.ParentSessionKey,
			SharedState:      sharedState,
			Files:            req.Context.Files,
			Instructions:     instructions,
			TraceID:          traceID,
			RunID:            uuid.New().String(),
		},
		Timeout: 600, // 10 minutes for execution
	}

	return c.spawner.Spawn(ctx, spawnReq)
}

// criticReview has the critic review the executor's work
func (c *CaptainsChair) criticReview(ctx context.Context, req CaptainsChairRequest, executorResult AgentResult, traceID string) (AgentResult, error) {
	if c.logger != nil {
		c.logger.Info("Critic reviewing execution", "trace_id", traceID)
	}

	instructions := fmt.Sprintf(`You are the critic agent. Review the following execution:

Original Task: %s

Executor's Result:
%v

Review the work for:
1. Correctness - Does it accomplish the task?
2. Quality - Is it well done?
3. Completeness - Is anything missing?
4. Issues - Are there any problems or errors?

Provide constructive feedback and identify any issues that need correction.`, req.Task, executorResult.Output)

	// Merge shared state with executor result
	sharedState := make(map[string]interface{})
	for k, v := range req.Context.SharedState {
		sharedState[k] = v
	}
	sharedState["execution"] = executorResult.Output

	spawnReq := SpawnRequest{
		AgentID:       req.CriticID,
		ParentAgentID: req.CaptainID,
		Context: AgentContext{
			ParentSessionKey: req.Context.ParentSessionKey,
			SharedState:      sharedState,
			Files:            req.Context.Files,
			Instructions:     instructions,
			TraceID:          traceID,
			RunID:            uuid.New().String(),
		},
		Timeout: 300, // 5 minutes for review
	}

	return c.spawner.Spawn(ctx, spawnReq)
}

// captainSynthesize has the captain synthesize the final result
func (c *CaptainsChair) captainSynthesize(ctx context.Context, req CaptainsChairRequest, executorResult, criticResult AgentResult, traceID string) (AgentResult, error) {
	if c.logger != nil {
		c.logger.Info("Captain synthesizing final result", "trace_id", traceID)
	}

	instructions := fmt.Sprintf(`You are the captain agent. Synthesize the final result:

Original Task: %s

Executor's Result:
%v

Critic's Review:
%v

Based on the executor's work and the critic's review, provide the final result.
If the critic identified issues, address them in your synthesis.
Provide a clear, complete final answer to the original task.`, req.Task, executorResult.Output, criticResult.Output)

	// Merge all results into shared state
	sharedState := make(map[string]interface{})
	for k, v := range req.Context.SharedState {
		sharedState[k] = v
	}
	sharedState["execution"] = executorResult.Output
	sharedState["review"] = criticResult.Output

	spawnReq := SpawnRequest{
		AgentID:       req.CaptainID,
		ParentAgentID: req.CaptainID,
		Context: AgentContext{
			ParentSessionKey: req.Context.ParentSessionKey,
			SharedState:      sharedState,
			Files:            req.Context.Files,
			Instructions:     instructions,
			TraceID:          traceID,
			RunID:            uuid.New().String(),
		},
		Timeout: 300, // 5 minutes for synthesis
	}

	return c.spawner.Spawn(ctx, spawnReq)
}
