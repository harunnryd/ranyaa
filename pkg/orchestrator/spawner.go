package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Spawner handles sub-agent spawning and lifecycle management
type Spawner struct {
	orchestrator *Orchestrator
	runner       AgentRunner
	logger       Logger
}

// AgentRunner interface for executing agents
type AgentRunner interface {
	Run(ctx context.Context, params AgentRunParams) (AgentResult, error)
}

// AgentRunParams defines parameters for agent execution
type AgentRunParams struct {
	SessionKey    string
	Prompt        string
	Config        AgentConfig
	Context       AgentContext
	ParentRunID   string
	ParentTraceID string
}

// NewSpawner creates a new Spawner instance
func NewSpawner(orchestrator *Orchestrator, runner AgentRunner, logger Logger) *Spawner {
	return &Spawner{
		orchestrator: orchestrator,
		runner:       runner,
		logger:       logger,
	}
}

// Spawn spawns a sub-agent with the given request
func (s *Spawner) Spawn(ctx context.Context, req SpawnRequest) (AgentResult, error) {
	// Validate request
	if req.AgentID == "" {
		return AgentResult{}, fmt.Errorf("agent_id is required")
	}

	if req.Context.Instructions == "" {
		return AgentResult{}, fmt.Errorf("instructions are required")
	}

	// Set default timeout if not specified
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 300 // Default 300 seconds
	}

	// Get agent configuration
	agentConfig, err := s.orchestrator.GetAgent(req.AgentID)
	if err != nil {
		return AgentResult{}, fmt.Errorf("failed to get agent config: %w", err)
	}

	// Check if we can spawn more agents
	if !s.orchestrator.CanSpawnMore() {
		return AgentResult{}, fmt.Errorf("max concurrent agents reached")
	}

	// Create agent instance
	instance, err := s.orchestrator.CreateInstance(req.AgentID)
	if err != nil {
		return AgentResult{}, fmt.Errorf("failed to create agent instance: %w", err)
	}

	// Generate child session key
	childSessionKey := s.generateChildSessionKey(req.Context.ParentSessionKey, instance.ID)

	// Generate trace and run IDs if not provided
	traceID := req.Context.TraceID
	if traceID == "" {
		traceID = uuid.New().String()
	}

	runID := req.Context.RunID
	if runID == "" {
		runID = uuid.New().String()
	}

	// Log spawn event
	if s.logger != nil {
		s.logger.Info("Spawning sub-agent",
			"instance_id", instance.ID,
			"agent_id", req.AgentID,
			"parent_session", req.Context.ParentSessionKey,
			"child_session", childSessionKey,
			"trace_id", traceID,
			"run_id", runID,
			"timeout", timeout)
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Update instance status to running
	if err := s.orchestrator.UpdateInstanceStatus(instance.ID, AgentStatusRunning); err != nil {
		return AgentResult{}, fmt.Errorf("failed to update instance status: %w", err)
	}

	// Prepare agent run parameters
	runParams := AgentRunParams{
		SessionKey:    childSessionKey,
		Prompt:        req.Context.Instructions,
		Config:        agentConfig,
		Context:       req.Context,
		ParentRunID:   runID,
		ParentTraceID: traceID,
	}

	// Execute agent
	startTime := time.Now()
	result, err := s.runner.Run(execCtx, runParams)
	duration := time.Since(startTime)

	// Update result with instance information
	result.InstanceID = instance.ID
	result.AgentID = req.AgentID
	result.Duration = duration.Milliseconds()

	// Handle execution result
	if err != nil {
		// Update instance status to failed
		_ = s.orchestrator.UpdateInstanceStatus(instance.ID, AgentStatusFailed)

		if s.logger != nil {
			s.logger.Error("Sub-agent execution failed",
				err,
				"instance_id", instance.ID,
				"agent_id", req.AgentID,
				"duration_ms", duration.Milliseconds())
		}

		result.Success = false
		result.Error = err.Error()
		return result, fmt.Errorf("agent execution failed: %w", err)
	}

	// Update instance status to stopped (completed)
	_ = s.orchestrator.UpdateInstanceStatus(instance.ID, AgentStatusStopped)

	if s.logger != nil {
		s.logger.Info("Sub-agent execution completed",
			"instance_id", instance.ID,
			"agent_id", req.AgentID,
			"duration_ms", duration.Milliseconds(),
			"success", result.Success)
	}

	result.Success = true
	return result, nil
}

// SpawnAndWait spawns a sub-agent and waits for completion
// This is a convenience method that wraps Spawn with a background context
func (s *Spawner) SpawnAndWait(req SpawnRequest) (AgentResult, error) {
	return s.Spawn(context.Background(), req)
}

// generateChildSessionKey generates a unique session key for a child agent
func (s *Spawner) generateChildSessionKey(parentSessionKey, instanceID string) string {
	if parentSessionKey == "" {
		return fmt.Sprintf("sub:%s", instanceID)
	}
	return fmt.Sprintf("%s:sub:%s", parentSessionKey, instanceID)
}
