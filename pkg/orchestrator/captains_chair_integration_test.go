package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// SimpleAgentRunner is a simple implementation for integration testing
type SimpleAgentRunner struct {
	responses map[string]string // Map role to response
}

func (s *SimpleAgentRunner) Run(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return AgentResult{
			Success: false,
		}, ctx.Err()
	default:
	}

	// Get response based on role
	response, ok := s.responses[string(params.Config.Role)]
	if !ok {
		response = fmt.Sprintf("Default response from %s", params.Config.Role)
	}

	return AgentResult{
		AgentID:  params.Config.ID,
		Success:  true,
		Output:   response,
		Duration: 10,
	}, nil
}

// Test 1: Full integration test with real orchestrator and spawner
func TestCaptainsChair_Integration_FullFlow(t *testing.T) {
	// Setup
	runner := &SimpleAgentRunner{
		responses: map[string]string{
			string(RoleCaptain):  "Captain response",
			string(RoleExecutor): "Executor response",
			string(RoleCritic):   "Critic response",
		},
	}

	logger := &MockLogger{}
	logger.On("Info", mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Debug", mock.Anything, mock.Anything).Return()

	orchestrator := New(WithLogger(logger), WithMaxConcurrent(10))
	spawner := NewSpawner(orchestrator, runner, logger)
	chair := NewCaptainsChair(orchestrator, spawner, logger)

	// Register agents
	orchestrator.RegisterAgent(AgentConfig{
		ID:          "captain-1",
		Name:        "Captain Agent",
		Role:        RoleCaptain,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	orchestrator.RegisterAgent(AgentConfig{
		ID:          "executor-1",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	orchestrator.RegisterAgent(AgentConfig{
		ID:          "critic-1",
		Name:        "Critic Agent",
		Role:        RoleCritic,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	// Execute Captain's Chair
	req := CaptainsChairRequest{
		CaptainID:  "captain-1",
		ExecutorID: "executor-1",
		CriticID:   "critic-1",
		Task:       "Write a hello world program",
		Context: AgentContext{
			ParentSessionKey: "integration-test-session",
			SharedState:      make(map[string]interface{}),
		},
	}

	result, err := chair.Execute(context.Background(), req)

	// Assertions
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)

	// Verify all stages completed
	assert.True(t, result.PlanResult.Success)
	assert.Equal(t, "Captain response", result.PlanResult.Output)

	assert.True(t, result.ExecutorResult.Success)
	assert.Equal(t, "Executor response", result.ExecutorResult.Output)

	assert.True(t, result.CriticResult.Success)
	assert.Equal(t, "Critic response", result.CriticResult.Output)

	assert.True(t, result.FinalResult.Success)
	assert.Equal(t, "Captain response", result.FinalResult.Output)

	// Verify timing
	assert.Greater(t, result.TotalDuration, int64(0))
	assert.Greater(t, result.TotalDuration, int64(40)) // At least 4 stages * 10ms
}

// Test 2: Integration test with executor failure and error handling
func TestCaptainsChair_Integration_ExecutorFailure(t *testing.T) {
	// Setup runner that fails on executor
	runner := &FailingAgentRunner{
		failOn: RoleExecutor,
	}

	logger := &MockLogger{}
	logger.On("Info", mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Debug", mock.Anything, mock.Anything).Return()

	orchestrator := New(WithLogger(logger), WithMaxConcurrent(10))
	spawner := NewSpawner(orchestrator, runner, logger)
	chair := NewCaptainsChair(orchestrator, spawner, logger)

	// Register agents
	orchestrator.RegisterAgent(AgentConfig{
		ID:          "captain-2",
		Name:        "Captain Agent",
		Role:        RoleCaptain,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	orchestrator.RegisterAgent(AgentConfig{
		ID:          "executor-2",
		Name:        "Executor Agent",
		Role:        RoleExecutor,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	orchestrator.RegisterAgent(AgentConfig{
		ID:          "critic-2",
		Name:        "Critic Agent",
		Role:        RoleCritic,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	// Execute Captain's Chair
	req := CaptainsChairRequest{
		CaptainID:  "captain-2",
		ExecutorID: "executor-2",
		CriticID:   "critic-2",
		Task:       "Test task",
		Context: AgentContext{
			ParentSessionKey: "integration-test-session-2",
			SharedState:      make(map[string]interface{}),
		},
	}

	result, err := chair.Execute(context.Background(), req)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution failed")
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "execution failed")

	// Verify plan succeeded but executor failed
	assert.True(t, result.PlanResult.Success)
	assert.False(t, result.ExecutorResult.Success)

	// Critic and final should not have run
	assert.False(t, result.CriticResult.Success)
	assert.False(t, result.FinalResult.Success)
}

// FailingAgentRunner fails on a specific role
type FailingAgentRunner struct {
	failOn AgentRole
}

func (f *FailingAgentRunner) Run(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	time.Sleep(10 * time.Millisecond)

	if params.Config.Role == f.failOn {
		return AgentResult{
			Success: false,
		}, fmt.Errorf("%s execution failed", f.failOn)
	}

	return AgentResult{
		AgentID:  params.Config.ID,
		Success:  true,
		Output:   fmt.Sprintf("Success from %s", params.Config.Role),
		Duration: 10,
	}, nil
}
