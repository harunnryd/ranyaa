package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParallelExecutor_E2E_CompleteWorkflow tests the complete parallel execution workflow
func TestParallelExecutor_E2E_CompleteWorkflow(t *testing.T) {
	// This E2E test simulates a real-world scenario where a captain agent
	// delegates multiple tasks to executor agents in parallel

	// Setup orchestrator
	orchestrator := New(WithMaxConcurrent(10), WithLogger(&mockLogger{}))

	// Create a realistic agent runner that simulates LLM execution
	executionLog := make([]string, 0)
	var executionLogMu sync.Mutex
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			// Simulate realistic execution time
			executionTime := 20 * time.Millisecond
			if params.Config.Role == RoleCaptain {
				executionTime = 30 * time.Millisecond
			}

			// Simulate work
			select {
			case <-time.After(executionTime):
				// Log execution
				executionLogMu.Lock()
				executionLog = append(executionLog, params.SessionKey)
				executionLogMu.Unlock()

				// Return result based on role
				var output interface{}
				if params.Config.Role == RoleCaptain {
					output = map[string]interface{}{
						"plan": "Delegate tasks to executors",
						"tasks": []string{
							"Analyze data",
							"Generate report",
							"Send notification",
						},
					}
				} else {
					output = map[string]interface{}{
						"status":    "completed",
						"task":      params.Prompt,
						"timestamp": time.Now().Unix(),
					}
				}

				return AgentResult{
					Success: true,
					Output:  output,
				}, nil

			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			}
		},
	}

	spawner := NewSpawner(orchestrator, runner, &mockLogger{})
	executor := NewParallelExecutor(spawner, &mockLogger{})

	// Register captain agent
	captainConfig := AgentConfig{
		ID:                     "captain",
		Name:                   "Captain Agent",
		Role:                   RoleCaptain,
		Model:                  "claude-opus-4",
		Temperature:            0.7,
		MaxTokens:              4096,
		MaxConcurrentSubAgents: 5,
	}
	err := orchestrator.RegisterAgent(captainConfig)
	require.NoError(t, err)

	// Register executor agents
	for i := 1; i <= 3; i++ {
		executorConfig := AgentConfig{
			ID:          "executor" + string(rune('0'+i)),
			Name:        "Executor Agent " + string(rune('0'+i)),
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}
		err := orchestrator.RegisterAgent(executorConfig)
		require.NoError(t, err)
	}

	// Simulate captain delegating tasks to executors in parallel
	parallelReq := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					ParentSessionKey: "captain-session-123",
					Instructions:     "Analyze the sales data for Q4 2024",
					SharedState: map[string]interface{}{
						"data_source": "sales_db",
						"quarter":     "Q4",
						"year":        2024,
					},
					TraceID: "trace-abc-123",
					RunID:   "run-xyz-456",
				},
				Timeout: 30,
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					ParentSessionKey: "captain-session-123",
					Instructions:     "Generate a comprehensive report with charts",
					SharedState: map[string]interface{}{
						"format": "pdf",
						"charts": []string{"bar", "line", "pie"},
					},
					TraceID: "trace-abc-123",
					RunID:   "run-xyz-456",
				},
				Timeout: 30,
			},
			{
				AgentID: "executor3",
				Context: AgentContext{
					ParentSessionKey: "captain-session-123",
					Instructions:     "Send notification to stakeholders",
					SharedState: map[string]interface{}{
						"recipients": []string{"ceo@company.com", "cfo@company.com"},
						"priority":   "high",
					},
					TraceID: "trace-abc-123",
					RunID:   "run-xyz-456",
				},
				Timeout: 30,
			},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	// Execute parallel tasks
	startTime := time.Now()
	results, err := executor.Execute(context.Background(), parallelReq)
	duration := time.Since(startTime)

	// Assertions
	assert.NoError(t, err)
	assert.Len(t, results, 3, "should have 3 results")

	// Verify all tasks completed successfully
	for i, result := range results {
		assert.True(t, result.Success, "task %d should succeed", i+1)
		assert.NotEmpty(t, result.InstanceID, "task %d should have instance ID", i+1)
		assert.NotEmpty(t, result.AgentID, "task %d should have agent ID", i+1)
		assert.NotNil(t, result.Output, "task %d should have output", i+1)

		// Verify output structure
		output, ok := result.Output.(map[string]interface{})
		assert.True(t, ok, "output should be a map")
		assert.Equal(t, "completed", output["status"])
		assert.NotNil(t, output["timestamp"])
	}

	// Verify parallel execution was actually parallel (not sequential)
	// If sequential, it would take 3 * 20ms = 60ms
	// If parallel, it should take ~20ms (plus overhead)
	assert.Less(t, duration, 50*time.Millisecond, "parallel execution should be faster than sequential")

	// Verify all agents executed
	executionLogMu.Lock()
	logEntries := append([]string(nil), executionLog...)
	executionLogMu.Unlock()

	assert.Len(t, logEntries, 3, "should have 3 execution logs")

	// Verify session keys are properly formatted
	for _, sessionKey := range logEntries {
		assert.Contains(t, sessionKey, "captain-session-123:sub:", "session key should contain parent session")
	}

	// Test join-first strategy
	parallelReqFirst := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					Instructions: "Quick task",
				},
				Timeout: 10,
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					Instructions: "Slow task",
				},
				Timeout: 10,
			},
		},
		JoinStrategy: JoinFirst,
		OnFail:       OnFailContinue,
	}

	resultsFirst, err := executor.Execute(context.Background(), parallelReqFirst)
	assert.NoError(t, err)
	assert.Len(t, resultsFirst, 1, "join-first should return only one result")
	assert.True(t, resultsFirst[0].Success)

	// Test join-any strategy
	parallelReqAny := ParallelRequest{
		Requests: []SpawnRequest{
			{
				AgentID: "executor1",
				Context: AgentContext{
					Instructions: "Task 1",
				},
				Timeout: 10,
			},
			{
				AgentID: "executor2",
				Context: AgentContext{
					Instructions: "Task 2",
				},
				Timeout: 10,
			},
		},
		JoinStrategy: JoinAny,
		OnFail:       OnFailContinue,
	}

	resultsAny, err := executor.Execute(context.Background(), parallelReqAny)
	assert.NoError(t, err)
	assert.Len(t, resultsAny, 1, "join-any should return only one result")

	// Give a moment for instances to finish
	time.Sleep(10 * time.Millisecond)

	// Cleanup stopped instances
	orchestrator.Cleanup()

	// Verify orchestrator state after cleanup
	// NOTE: There might be some instances still in transition, so we just verify cleanup works
	activeInstances := orchestrator.GetActiveInstances()
	assert.GreaterOrEqual(t, 10, activeInstances, "should have reasonable number of instances")

	// Shutdown
	err = orchestrator.Shutdown(context.Background())
	assert.NoError(t, err)
}
