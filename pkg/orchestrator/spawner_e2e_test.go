package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E test for spawner with realistic multi-agent workflow

func TestSpawnerE2E_CaptainExecutorCriticWorkflow(t *testing.T) {
	// This test simulates a realistic Captain's Chair pattern:
	// Captain spawns Executor -> Executor completes task -> Captain spawns Critic -> Critic reviews

	// Setup orchestrator
	orchestrator := New(WithMaxConcurrent(10))
	logger := &mockLogger{}

	// Track execution flow
	executionLog := []string{}

	// Create a realistic agent runner that simulates different agent behaviors
	runner := &mockAgentRunner{
		runFunc: func(ctx context.Context, params AgentRunParams) (AgentResult, error) {
			agentRole := params.Config.Role

			switch agentRole {
			case RoleCaptain:
				executionLog = append(executionLog, "captain:planning")
				// Captain creates a plan
				return AgentResult{
					Success: true,
					Output: map[string]interface{}{
						"plan":       "1. Analyze requirements\n2. Implement solution\n3. Test implementation",
						"next_agent": "executor",
					},
				}, nil

			case RoleExecutor:
				executionLog = append(executionLog, "executor:executing")
				// Executor performs the task
				time.Sleep(50 * time.Millisecond) // Simulate work
				return AgentResult{
					Success: true,
					Output: map[string]interface{}{
						"result":         "Implementation completed",
						"files_modified": []string{"main.go", "utils.go"},
						"tests_passed":   true,
					},
				}, nil

			case RoleCritic:
				executionLog = append(executionLog, "critic:reviewing")
				// Critic reviews the work
				time.Sleep(30 * time.Millisecond) // Simulate review

				// Check if executor output is in shared state
				executorOutput, ok := params.Context.SharedState["executor_output"]
				if !ok {
					return AgentResult{
						Success: false,
						Error:   "No executor output to review",
					}, fmt.Errorf("missing executor output")
				}

				return AgentResult{
					Success: true,
					Output: map[string]interface{}{
						"review":          "Code looks good",
						"issues":          []string{},
						"approved":        true,
						"reviewed_output": executorOutput,
					},
				}, nil

			default:
				return AgentResult{}, fmt.Errorf("unknown agent role: %s", agentRole)
			}
		},
	}

	spawner := NewSpawner(orchestrator, runner, logger)

	// Register all three agent types
	agents := []AgentConfig{
		{
			ID:                     "captain",
			Name:                   "Captain Agent",
			Role:                   RoleCaptain,
			Model:                  "claude-opus-4",
			Temperature:            0.7,
			MaxTokens:              4096,
			MaxConcurrentSubAgents: 5,
		},
		{
			ID:          "executor",
			Name:        "Executor Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		},
		{
			ID:          "critic",
			Name:        "Critic Agent",
			Role:        RoleCritic,
			Model:       "claude-sonnet-4",
			Temperature: 0.5,
			MaxTokens:   2048,
		},
	}

	for _, agent := range agents {
		err := orchestrator.RegisterAgent(agent)
		require.NoError(t, err)
	}

	// Step 1: Captain creates plan
	captainReq := SpawnRequest{
		AgentID: "captain",
		Context: AgentContext{
			ParentSessionKey: "main-session",
			Instructions:     "Create a plan to implement feature X",
			TraceID:          "trace-001",
			RunID:            "run-001",
		},
		Timeout: 10,
	}

	captainResult, err := spawner.Spawn(context.Background(), captainReq)
	require.NoError(t, err)
	assert.True(t, captainResult.Success)

	captainOutput, ok := captainResult.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, captainOutput, "plan")

	// Step 2: Spawn executor based on captain's plan
	executorReq := SpawnRequest{
		AgentID: "executor",
		Context: AgentContext{
			ParentSessionKey: "main-session",
			Instructions:     "Execute the plan: " + captainOutput["plan"].(string),
			SharedState: map[string]interface{}{
				"captain_plan": captainOutput["plan"],
			},
			TraceID: "trace-001",
			RunID:   "run-002",
		},
		Timeout: 10,
	}

	executorResult, err := spawner.Spawn(context.Background(), executorReq)
	require.NoError(t, err)
	assert.True(t, executorResult.Success)

	executorOutput, ok := executorResult.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, executorOutput, "result")
	assert.Equal(t, true, executorOutput["tests_passed"])

	// Step 3: Spawn critic to review executor's work
	criticReq := SpawnRequest{
		AgentID: "critic",
		Context: AgentContext{
			ParentSessionKey: "main-session",
			Instructions:     "Review the implementation",
			SharedState: map[string]interface{}{
				"executor_output": executorOutput,
			},
			TraceID: "trace-001",
			RunID:   "run-003",
		},
		Timeout: 10,
	}

	criticResult, err := spawner.Spawn(context.Background(), criticReq)
	require.NoError(t, err)
	assert.True(t, criticResult.Success)

	criticOutput, ok := criticResult.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, criticOutput, "review")
	assert.Equal(t, true, criticOutput["approved"])

	// Verify execution flow
	assert.Equal(t, []string{
		"captain:planning",
		"executor:executing",
		"critic:reviewing",
	}, executionLog)

	// Verify all agents completed successfully
	assert.True(t, captainResult.Success)
	assert.True(t, executorResult.Success)
	assert.True(t, criticResult.Success)

	// Verify trace IDs are consistent
	assert.NotEmpty(t, captainResult.InstanceID)
	assert.NotEmpty(t, executorResult.InstanceID)
	assert.NotEmpty(t, criticResult.InstanceID)

	// Verify all instances are different
	assert.NotEqual(t, captainResult.InstanceID, executorResult.InstanceID)
	assert.NotEqual(t, executorResult.InstanceID, criticResult.InstanceID)

	// Cleanup
	orchestrator.Cleanup()
	assert.Equal(t, 0, len(orchestrator.agents))

	// Verify logging captured all key events
	assert.GreaterOrEqual(t, len(logger.infos), 6) // 3 spawns + 3 completions
}
