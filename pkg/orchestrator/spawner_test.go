package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock agent runner for testing
type mockAgentRunner struct {
	runFunc func(ctx context.Context, params AgentRunParams) (AgentResult, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, params)
	}
	return AgentResult{Success: true, Output: "mock result"}, nil
}

func TestNewSpawner(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	logger := &mockLogger{}

	spawner := NewSpawner(orchestrator, runner, logger)

	assert.NotNil(t, spawner)
	assert.Equal(t, orchestrator, spawner.orchestrator)
	assert.Equal(t, runner, spawner.runner)
	assert.Equal(t, logger, spawner.logger)
}

func TestSpawn_Success(t *testing.T) {
	// Setup
	orchestrator := New(WithMaxConcurrent(5))
	logger := &mockLogger{}

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			return AgentResult{
				Success: true,
				Output:  "task completed",
			}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	// Create spawn request
	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			ParentSessionKey: "parent-session-123",
			Instructions:     "Execute this task",
			SharedState:      map[string]interface{}{"key": "value"},
		},
		Timeout: 10,
	}

	// Execute
	result, err := spawner.Spawn(context.Background(), req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "executor", result.AgentID)
	assert.NotEmpty(t, result.InstanceID)
	assert.Equal(t, "task completed", result.Output)
	assert.GreaterOrEqual(t, result.Duration, int64(0))

	// Verify logging
	assert.Contains(t, logger.infos, "Spawning sub-agent")
	assert.Contains(t, logger.infos, "Sub-agent execution completed")
}

func TestSpawn_MissingAgentID(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	req := SpawnRequest{
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent_id is required")
	assert.False(t, result.Success)
}

func TestSpawn_MissingInstructions(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "instructions are required")
	assert.False(t, result.Success)
}

func TestSpawn_AgentNotFound(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	req := SpawnRequest{
		AgentID: "nonexistent",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get agent config")
	assert.False(t, result.Success)
}

func TestSpawn_ParentAgentNotFound(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID:       "executor",
		ParentAgentID: "missing-parent",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get parent agent config")
	assert.False(t, result.Success)
}

func TestSpawn_SubAgentDeniedByParentAllowlist(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	parent := AgentConfig{
		ID:               "captain",
		Name:             "Captain Agent",
		Role:             RoleCaptain,
		Model:            "claude-opus-4",
		AllowedSubAgents: []string{"critic"},
	}
	target := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	require.NoError(t, orchestrator.RegisterAgent(parent))
	require.NoError(t, orchestrator.RegisterAgent(target))

	req := SpawnRequest{
		AgentID:       "executor",
		ParentAgentID: "captain",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not allowed to spawn sub-agent")
	assert.False(t, result.Success)
}

func TestSpawn_SubAgentAllowedByParentAllowlist(t *testing.T) {
	orchestrator := New()
	runner := &mockAgentRunner{}
	spawner := NewSpawner(orchestrator, runner, nil)

	parent := AgentConfig{
		ID:               "captain",
		Name:             "Captain Agent",
		Role:             RoleCaptain,
		Model:            "claude-opus-4",
		AllowedSubAgents: []string{"executor"},
	}
	target := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	require.NoError(t, orchestrator.RegisterAgent(parent))
	require.NoError(t, orchestrator.RegisterAgent(target))

	req := SpawnRequest{
		AgentID:       "executor",
		ParentAgentID: "captain",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "executor", result.AgentID)
}

func TestSpawn_MaxConcurrentReached(t *testing.T) {
	// Setup orchestrator with max 1 concurrent agent
	orchestrator := New(WithMaxConcurrent(1))

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Simulate long-running task
			time.Sleep(100 * time.Millisecond)
			return AgentResult{Success: true}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, nil)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	// Create first instance and set it to running
	instance, err := orchestrator.CreateInstance("executor")
	require.NoError(t, err)
	err = orchestrator.UpdateInstanceStatus(instance.ID, AgentStatusRunning)
	require.NoError(t, err)

	// Try to spawn another agent (should fail)
	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent agents reached")
	assert.False(t, result.Success)
}

func TestSpawn_ExecutionFailure(t *testing.T) {
	orchestrator := New()
	logger := &mockLogger{}

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			return AgentResult{}, errors.New("execution failed")
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent execution failed")
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, logger.errors, "Sub-agent execution failed")
}

func TestSpawn_Timeout(t *testing.T) {
	orchestrator := New()
	logger := &mockLogger{}

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Simulate long-running task
			select {
			case <-time.After(5 * time.Second):
				return AgentResult{Success: true}, nil
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			}
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
		Timeout: 1, // 1 second timeout
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, logger.errors, "Sub-agent execution failed")
}

func TestSpawn_DefaultTimeout(t *testing.T) {
	orchestrator := New()

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Check that context has a deadline
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have a deadline")
			assert.True(t, time.Until(deadline) > 0, "deadline should be in the future")

			return AgentResult{Success: true}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, nil)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
		// No timeout specified, should use default 300s
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSpawn_ChildSessionKey(t *testing.T) {
	orchestrator := New()

	var capturedSessionKey string
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			capturedSessionKey = params.SessionKey
			return AgentResult{Success: true}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, nil)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			ParentSessionKey: "parent-session-123",
			Instructions:     "Execute this task",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, capturedSessionKey, "parent-session-123:sub:")
}

func TestSpawn_TraceAndRunIDs(t *testing.T) {
	orchestrator := New()

	var capturedParams AgentRunParams
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			capturedParams = params
			return AgentResult{Success: true}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, nil)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	// Test with provided trace and run IDs
	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
			TraceID:      "trace-123",
			RunID:        "run-456",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "trace-123", capturedParams.ParentTraceID)
	assert.Equal(t, "run-456", capturedParams.ParentRunID)
}

func TestSpawnAndWait(t *testing.T) {
	orchestrator := New()

	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			return AgentResult{Success: true, Output: "completed"}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, nil)

	// Register an agent
	agentConfig := AgentConfig{
		ID:          "executor",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "claude-sonnet-4",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	err := orchestrator.RegisterAgent(agentConfig)
	require.NoError(t, err)

	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			Instructions: "Execute this task",
		},
	}

	result, err := spawner.SpawnAndWait(req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "completed", result.Output)
}

func TestGenerateChildSessionKey(t *testing.T) {
	spawner := &Spawner{}

	tests := []struct {
		name             string
		parentSessionKey string
		instanceID       string
		expectedContains []string
	}{
		{
			name:             "with parent session",
			parentSessionKey: "parent-123",
			instanceID:       "instance-456",
			expectedContains: []string{"parent-123", "sub:", "instance-456"},
		},
		{
			name:             "without parent session",
			parentSessionKey: "",
			instanceID:       "instance-789",
			expectedContains: []string{"sub:", "instance-789"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spawner.generateChildSessionKey(tt.parentSessionKey, tt.instanceID)

			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}
