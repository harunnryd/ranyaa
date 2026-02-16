package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParallelExecutor_Integration_JoinAll tests parallel execution with join-all strategy
func TestParallelExecutor_Integration_JoinAll(t *testing.T) {
	// Setup orchestrator
	orchestrator := New(WithMaxConcurrent(10))
	logger := &mockLogger{}

	// Create mock runner that simulates real execution
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			return AgentResult{
				Success: true,
				Output:  "completed: " + params.Prompt,
			}, nil
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)
	executor := NewParallelExecutor(spawner, logger)

	// Register agents
	for i := 1; i <= 3; i++ {
		agentConfig := AgentConfig{
			ID:          "executor" + string(rune('0'+i)),
			Name:        "Executor Agent " + string(rune('0'+i)),
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}
		err := orchestrator.RegisterAgent(agentConfig)
		require.NoError(t, err)
	}

	// Create parallel request
	req := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					ParentSessionKey: "parent-session",
					Instructions:     "task 1",
				},
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					ParentSessionKey: "parent-session",
					Instructions:     "task 2",
				},
			},
			{
				AgentID: "executor3",
				Context: AgentContext{
					ParentSessionKey: "parent-session",
					Instructions:     "task 3",
				},
			},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	// Execute
	results, err := executor.Execute(context.Background(), req)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all agents executed
	for i, result := range results {
		assert.True(t, result.Success, "agent %d should succeed", i)
		assert.NotEmpty(t, result.InstanceID)
		assert.Contains(t, result.Output, "completed")
	}
}

// TestParallelExecutor_Integration_JoinFirst tests parallel execution with join-first strategy
func TestParallelExecutor_Integration_JoinFirst(t *testing.T) {
	// Setup orchestrator
	orchestrator := New(WithMaxConcurrent(10))
	logger := &mockLogger{}

	// Create mock runner with varying execution times
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// First agent is fast, others are slow
			if params.Prompt == "fast task" {
				time.Sleep(10 * time.Millisecond)
			} else {
				time.Sleep(100 * time.Millisecond)
			}

			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			default:
				return AgentResult{
					Success: true,
					Output:  "completed: " + params.Prompt,
				}, nil
			}
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)
	executor := NewParallelExecutor(spawner, logger)

	// Register agents
	for i := 1; i <= 3; i++ {
		agentConfig := AgentConfig{
			ID:          "executor" + string(rune('0'+i)),
			Name:        "Executor Agent " + string(rune('0'+i)),
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}
		err := orchestrator.RegisterAgent(agentConfig)
		require.NoError(t, err)
	}

	// Create parallel request
	req := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					Instructions: "fast task",
				},
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					Instructions: "slow task 1",
				},
			},
			{
				AgentID: "executor3",
				Context: AgentContext{
					Instructions: "slow task 2",
				},
			},
		},
		JoinStrategy: JoinFirst,
		OnFail:       OnFailContinue,
	}

	// Execute
	startTime := time.Now()
	results, err := executor.Execute(context.Background(), req)
	duration := time.Since(startTime)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Contains(t, results[0].Output, "fast task")

	// Should complete quickly (not wait for slow tasks)
	assert.Less(t, duration, 50*time.Millisecond, "should complete quickly with join-first")
}

// TestParallelExecutor_Integration_OnFailAbort tests abort on failure
func TestParallelExecutor_Integration_OnFailAbort(t *testing.T) {
	// Setup orchestrator
	orchestrator := New(WithMaxConcurrent(10))
	logger := &mockLogger{}

	// Create mock runner where one agent fails
	executionCount := 0
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			executionCount++

			// First agent fails immediately
			if params.Prompt == "failing task" {
				return AgentResult{
					Success: false,
					Error:   "intentional failure",
				}, assert.AnError
			}

			// Other agents take longer
			time.Sleep(50 * time.Millisecond)

			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			default:
				return AgentResult{
					Success: true,
					Output:  "completed: " + params.Prompt,
				}, nil
			}
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)
	executor := NewParallelExecutor(spawner, logger)

	// Register agents
	for i := 1; i <= 3; i++ {
		agentConfig := AgentConfig{
			ID:          "executor" + string(rune('0'+i)),
			Name:        "Executor Agent " + string(rune('0'+i)),
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}
		err := orchestrator.RegisterAgent(agentConfig)
		require.NoError(t, err)
	}

	// Create parallel request with abort on failure
	req := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					Instructions: "failing task",
				},
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					Instructions: "task 2",
				},
			},
			{
				AgentID: "executor3",
				Context: AgentContext{
					Instructions: "task 3",
				},
			},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailAbort,
	}

	// Execute
	results, err := executor.Execute(context.Background(), req)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parallel execution aborted")
	assert.Len(t, results, 3) // All results returned even on abort

	// At least one should have failed
	failedCount := 0
	for _, result := range results {
		if !result.Success {
			failedCount++
		}
	}
	assert.Greater(t, failedCount, 0, "at least one agent should have failed")
}
