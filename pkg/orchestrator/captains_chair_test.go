package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockAgentRunner is a mock implementation of AgentRunner
type MockAgentRunner struct {
	mock.Mock
}

func (m *MockAgentRunner) Run(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(AgentResult), args.Error(1)
}

// MockLogger is a mock implementation of Logger
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(msg string, fields ...interface{}) {
	m.Called(msg, fields)
}

func (m *MockLogger) Error(msg string, err error, fields ...interface{}) {
	m.Called(msg, err, fields)
}

func (m *MockLogger) Debug(msg string, fields ...interface{}) {
	m.Called(msg, fields)
}

// setupCaptainsChair creates a test setup with orchestrator, spawner, and captain's chair
func setupCaptainsChair(t *testing.T, runner AgentRunner) (*Orchestrator, *Spawner, *CaptainsChair) {
	logger := &MockLogger{}
	logger.On("Info", mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Debug", mock.Anything, mock.Anything).Return()

	orchestrator := New(WithLogger(logger), WithMaxConcurrent(10))
	spawner := NewSpawner(orchestrator, runner, logger)
	chair := NewCaptainsChair(orchestrator, spawner, logger)

	// Register test agents
	captainConfig := AgentConfig{
		ID:          "test-captain",
		Name:        "Test Captain",
		Role:        RoleCaptain,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	require.NoError(t, orchestrator.RegisterAgent(captainConfig))

	executorConfig := AgentConfig{
		ID:          "test-executor",
		Name:        "Test Executor",
		Role:        RoleExecutor,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	require.NoError(t, orchestrator.RegisterAgent(executorConfig))

	criticConfig := AgentConfig{
		ID:          "test-critic",
		Name:        "Test Critic",
		Role:        RoleCritic,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   4096,
	}
	require.NoError(t, orchestrator.RegisterAgent(criticConfig))

	return orchestrator, spawner, chair
}

// Test 1: Successful Captain's Chair execution
func TestCaptainsChair_Execute_Success(t *testing.T) {
	mockRunner := &MockAgentRunner{}
	_, _, chair := setupCaptainsChair(t, mockRunner)

	// Mock successful responses for all stages
	mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
		return params.Config.Role == RoleCaptain && params.Context.SharedState["plan"] == nil
	})).Return(AgentResult{
		Success: true,
		Output:  "Step 1: Do X\nStep 2: Do Y",
	}, nil)

	mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
		return params.Config.Role == RoleExecutor
	})).Return(AgentResult{
		Success: true,
		Output:  "Executed X and Y successfully",
	}, nil)

	mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
		return params.Config.Role == RoleCritic
	})).Return(AgentResult{
		Success: true,
		Output:  "Work looks good, no issues found",
	}, nil)

	mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
		return params.Config.Role == RoleCaptain && params.Context.SharedState["execution"] != nil
	})).Return(AgentResult{
		Success: true,
		Output:  "Final result: Task completed successfully",
	}, nil)

	req := CaptainsChairRequest{
		CaptainID:  "test-captain",
		ExecutorID: "test-executor",
		CriticID:   "test-critic",
		Task:       "Test task",
		Context: AgentContext{
			ParentSessionKey: "test-session",
			SharedState:      make(map[string]interface{}),
		},
	}

	result, err := chair.Execute(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.True(t, result.PlanResult.Success)
	assert.True(t, result.ExecutorResult.Success)
	assert.True(t, result.CriticResult.Success)
	assert.True(t, result.FinalResult.Success)
	assert.GreaterOrEqual(t, result.TotalDuration, int64(0))

	mockRunner.AssertExpectations(t)
}

// Test 2: Validation errors
func TestCaptainsChair_Execute_ValidationErrors(t *testing.T) {
	mockRunner := &MockAgentRunner{}
	_, _, chair := setupCaptainsChair(t, mockRunner)

	tests := []struct {
		name    string
		req     CaptainsChairRequest
		wantErr string
	}{
		{
			name: "missing captain ID",
			req: CaptainsChairRequest{
				ExecutorID: "test-executor",
				CriticID:   "test-critic",
				Task:       "Test task",
			},
			wantErr: "captain_id is required",
		},
		{
			name: "missing executor ID",
			req: CaptainsChairRequest{
				CaptainID: "test-captain",
				CriticID:  "test-critic",
				Task:      "Test task",
			},
			wantErr: "executor_id is required",
		},
		{
			name: "missing critic ID",
			req: CaptainsChairRequest{
				CaptainID:  "test-captain",
				ExecutorID: "test-executor",
				Task:       "Test task",
			},
			wantErr: "critic_id is required",
		},
		{
			name: "missing task",
			req: CaptainsChairRequest{
				CaptainID:  "test-captain",
				ExecutorID: "test-executor",
				CriticID:   "test-critic",
			},
			wantErr: "task is required",
		},
		{
			name: "non-existent captain",
			req: CaptainsChairRequest{
				CaptainID:  "non-existent",
				ExecutorID: "test-executor",
				CriticID:   "test-critic",
				Task:       "Test task",
			},
			wantErr: "captain agent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := chair.Execute(context.Background(), tt.req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.False(t, result.Success)
			assert.NotEmpty(t, result.Error)
		})
	}
}

// Test 3: Execution stage failures
func TestCaptainsChair_Execute_StageFailures(t *testing.T) {
	tests := []struct {
		name      string
		failStage AgentRole
		wantErr   string
	}{
		{
			name:      "plan creation failure",
			failStage: RoleCaptain,
			wantErr:   "plan creation failed",
		},
		{
			name:      "executor failure",
			failStage: RoleExecutor,
			wantErr:   "execution failed",
		},
		{
			name:      "critic failure",
			failStage: RoleCritic,
			wantErr:   "review failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := &MockAgentRunner{}
			_, _, chair := setupCaptainsChair(t, mockRunner)

			// Mock success for stages before the failing one
			mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
				return params.Config.Role != tt.failStage
			})).Return(AgentResult{
				Success: true,
				Output:  "Success",
			}, nil)

			// Mock failure for the specific stage
			mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
				return params.Config.Role == tt.failStage
			})).Return(AgentResult{
				Success: false,
			}, errors.New(tt.wantErr))

			req := CaptainsChairRequest{
				CaptainID:  "test-captain",
				ExecutorID: "test-executor",
				CriticID:   "test-critic",
				Task:       "Test task",
				Context: AgentContext{
					ParentSessionKey: "test-session",
					SharedState:      make(map[string]interface{}),
				},
			}

			result, err := chair.Execute(context.Background(), req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.False(t, result.Success)
			assert.Contains(t, result.Error, tt.wantErr)
			assert.GreaterOrEqual(t, result.TotalDuration, int64(0))
		})
	}
}

// Test 4: Timeout handling
func TestCaptainsChair_Execute_Timeout(t *testing.T) {
	mockRunner := &MockAgentRunner{}
	_, _, chair := setupCaptainsChair(t, mockRunner)

	// Mock a slow plan creation that will timeout
	mockRunner.On("Run", mock.Anything, mock.MatchedBy(func(params AgentRunParams) bool {
		return params.Config.Role == RoleCaptain
	})).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		// Wait for context to be cancelled
		<-ctx.Done()
	}).Return(AgentResult{
		Success: false,
	}, context.DeadlineExceeded)

	req := CaptainsChairRequest{
		CaptainID:  "test-captain",
		ExecutorID: "test-executor",
		CriticID:   "test-critic",
		Task:       "Test task",
		Context: AgentContext{
			ParentSessionKey: "test-session",
			SharedState:      make(map[string]interface{}),
		},
		Timeout: 1, // 1 second timeout
	}

	result, err := chair.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Greater(t, result.TotalDuration, int64(0))

	mockRunner.AssertExpectations(t)
}
