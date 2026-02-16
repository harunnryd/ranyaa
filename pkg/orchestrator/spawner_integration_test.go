package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for spawner with real orchestrator interactions

func TestSpawnerIntegration_MultipleSequentialSpawns(t *testing.T) {
	// Setup
	orchestrator := New(WithMaxConcurrent(5))
	logger := &mockLogger{}

	executionCount := 0
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			executionCount++
			return AgentResult{
				Success: true,
				Output:  map[string]interface{}{"execution": executionCount},
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

	// Spawn multiple agents sequentially
	for i := 0; i < 3; i++ {
		req := SpawnRequest{
			AgentID: "executor",
			Context: AgentContext{
				ParentSessionKey: "parent-session",
				Instructions:     "Execute task",
			},
		}

		result, err := spawner.Spawn(context.Background(), req)

		assert.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "executor", result.AgentID)
	}

	// Verify all executions completed
	assert.Equal(t, 3, executionCount)

	// Cleanup and verify instances are removed
	orchestrator.Cleanup()
	assert.Equal(t, 0, len(orchestrator.agents))
}

func TestSpawnerIntegration_ConcurrentSpawns(t *testing.T) {
	// Setup
	orchestrator := New(WithMaxConcurrent(10))
	logger := &mockLogger{}

	completedChan := make(chan int, 5)
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			completedChan <- 1
			return AgentResult{Success: true}, nil
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

	// Spawn multiple agents concurrently
	numAgents := 5
	resultsChan := make(chan AgentResult, numAgents)
	errorsChan := make(chan error, numAgents)

	for i := 0; i < numAgents; i++ {
		go func(idx int) {
			req := SpawnRequest{
				AgentID: "executor",
				Context: AgentContext{
					ParentSessionKey: "parent-session",
					Instructions:     "Execute task",
				},
			}

			result, err := spawner.Spawn(context.Background(), req)
			if err != nil {
				errorsChan <- err
				return
			}
			resultsChan <- result
		}(i)
	}

	// Wait for all to complete
	completed := 0
	for completed < numAgents {
		select {
		case result := <-resultsChan:
			assert.True(t, result.Success)
			completed++
		case err := <-errorsChan:
			t.Fatalf("Unexpected error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for agents to complete")
		}
	}

	// Verify all completed
	assert.Equal(t, numAgents, completed)
}

func TestSpawnerIntegration_ContextPropagation(t *testing.T) {
	// Setup
	orchestrator := New()
	logger := &mockLogger{}

	var capturedContext AgentContext
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			capturedContext = params.Context
			return AgentResult{Success: true}, nil
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

	// Create request with rich context
	req := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			ParentSessionKey: "parent-session-123",
			Instructions:     "Execute this complex task",
			SharedState: map[string]interface{}{
				"user_id":   "user-456",
				"workspace": "/home/user/project",
				"config": map[string]interface{}{
					"debug": true,
					"level": 2,
				},
			},
			Files:   []string{"file1.go", "file2.go"},
			TraceID: "trace-789",
			RunID:   "run-012",
		},
	}

	result, err := spawner.Spawn(context.Background(), req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, result.Success)

	// Verify context was propagated correctly
	assert.Equal(t, "parent-session-123", capturedContext.ParentSessionKey)
	assert.Equal(t, "Execute this complex task", capturedContext.Instructions)
	assert.Equal(t, "user-456", capturedContext.SharedState["user_id"])
	assert.Equal(t, "/home/user/project", capturedContext.SharedState["workspace"])
	assert.Len(t, capturedContext.Files, 2)
	assert.Contains(t, capturedContext.Files, "file1.go")
	assert.Contains(t, capturedContext.Files, "file2.go")
	assert.Equal(t, "trace-789", capturedContext.TraceID)
	assert.Equal(t, "run-012", capturedContext.RunID)
}
