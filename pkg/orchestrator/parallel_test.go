package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockSpawner is a mock implementation of Spawner for testing
type mockSpawner struct {
	spawnFunc func(ctx context.Context, req SpawnRequest) (AgentResult, error)
	mu        sync.Mutex
	calls     []SpawnRequest
}

func (m *mockSpawner) Spawn(ctx context.Context, req SpawnRequest) (AgentResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()

	if m.spawnFunc != nil {
		return m.spawnFunc(ctx, req)
	}
	return AgentResult{Success: true, Output: "mock result"}, nil
}

// TestParallelExecutor_Execute_EmptyRequests tests that empty requests return an error
func TestParallelExecutor_Execute_EmptyRequests(t *testing.T) {
	spawner := &mockSpawner{}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests:     []SpawnRequest{},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no requests provided")
	assert.Empty(t, results)
}

// TestParallelExecutor_Execute_InvalidJoinStrategy tests invalid join strategy
func TestParallelExecutor_Execute_InvalidJoinStrategy(t *testing.T) {
	spawner := &mockSpawner{}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
		},
		JoinStrategy: "invalid",
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid join strategy")
	assert.Empty(t, results)
}

// TestParallelExecutor_Execute_InvalidOnFailStrategy tests invalid on-fail strategy
func TestParallelExecutor_Execute_InvalidOnFailStrategy(t *testing.T) {
	spawner := &mockSpawner{}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
		},
		JoinStrategy: JoinAll,
		OnFail:       "invalid",
	}

	results, err := executor.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid on-fail strategy")
	assert.Empty(t, results)
}

// TestParallelExecutor_JoinAll_AllSuccess tests join-all with all agents succeeding
func TestParallelExecutor_JoinAll_AllSuccess(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "agent1", results[0].AgentID)
	assert.Equal(t, "agent2", results[1].AgentID)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)
}

// TestParallelExecutor_JoinAll_PartialFailure_Continue tests join-all with partial failure and continue
func TestParallelExecutor_JoinAll_PartialFailure_Continue(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			if req.AgentID == "agent2" {
				return AgentResult{
					AgentID:    req.AgentID,
					InstanceID: "instance-" + req.AgentID,
					Success:    false,
					Error:      "execution failed",
				}, errors.New("execution failed")
			}
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.NoError(t, err) // Continue strategy doesn't return error
	assert.Len(t, results, 2)
	assert.Equal(t, "agent1", results[0].AgentID)
	assert.Equal(t, "agent2", results[1].AgentID)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
}

// TestParallelExecutor_JoinAll_PartialFailure_Abort tests join-all with partial failure and abort
func TestParallelExecutor_JoinAll_PartialFailure_Abort(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    false,
				Error:      "execution failed",
			}, errors.New("execution failed")
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailAbort,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parallel execution aborted")
	assert.Len(t, results, 2) // Results still returned even on abort
}

// TestParallelExecutor_JoinFirst_FirstSuccess tests join-first with first agent succeeding
func TestParallelExecutor_JoinFirst_FirstSuccess(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			if req.AgentID == "agent1" {
				// First agent succeeds quickly
				return AgentResult{
					AgentID:    req.AgentID,
					InstanceID: "instance-" + req.AgentID,
					Success:    true,
					Output:     "result-" + req.AgentID,
				}, nil
			}
			// Second agent is slower
			time.Sleep(100 * time.Millisecond)
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinFirst,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "agent1", results[0].AgentID)
	assert.True(t, results[0].Success)
}

// TestParallelExecutor_JoinFirst_AllFail tests join-first with all agents failing
func TestParallelExecutor_JoinFirst_AllFail(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    false,
				Error:      "execution failed",
			}, errors.New("execution failed")
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinFirst,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all agents failed")
	assert.Empty(t, results)
}

// TestParallelExecutor_JoinAny_Success tests join-any with first completion
func TestParallelExecutor_JoinAny_Success(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			if req.AgentID == "agent1" {
				// First agent completes quickly
				return AgentResult{
					AgentID:    req.AgentID,
					InstanceID: "instance-" + req.AgentID,
					Success:    true,
					Output:     "result-" + req.AgentID,
				}, nil
			}
			// Second agent is slower
			time.Sleep(100 * time.Millisecond)
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinAny,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "agent1", results[0].AgentID)
}

// TestParallelExecutor_JoinAny_FirstFails tests join-any with first agent failing
func TestParallelExecutor_JoinAny_FirstFails(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			if req.AgentID == "agent1" {
				// First agent fails quickly
				return AgentResult{
					AgentID:    req.AgentID,
					InstanceID: "instance-" + req.AgentID,
					Success:    false,
					Error:      "execution failed",
				}, errors.New("execution failed")
			}
			// Second agent is slower
			time.Sleep(100 * time.Millisecond)
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
			{AgentID: "agent2", Context: AgentContext{Instructions: "task2"}},
		},
		JoinStrategy: JoinAny,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(context.Background(), req)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "agent1", results[0].AgentID)
	assert.False(t, results[0].Success) // Join-any returns even failures
}

// TestParallelExecutor_ContextCancellation tests that context cancellation is handled
func TestParallelExecutor_ContextCancellation(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			<-ctx.Done() // Wait for cancellation
			return AgentResult{}, context.Canceled
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
		},
		JoinStrategy: JoinFirst,
		OnFail:       OnFailContinue,
	}

	results, err := executor.Execute(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
	assert.Empty(t, results)
}

// TestParallelExecutor_ExecuteAndWait tests the convenience method
func TestParallelExecutor_ExecuteAndWait(t *testing.T) {
	spawner := &mockSpawner{
		spawnFunc: func(ctx context.Context, req SpawnRequest) (AgentResult, error) {
			return AgentResult{
				AgentID:    req.AgentID,
				InstanceID: "instance-" + req.AgentID,
				Success:    true,
				Output:     "result-" + req.AgentID,
			}, nil
		},
	}
	logger := &mockLogger{}
	executor := NewParallelExecutor(spawner, logger)

	req := ParallelRequest{
		Requests: []SpawnRequest{
			{AgentID: "agent1", Context: AgentContext{Instructions: "task1"}},
		},
		JoinStrategy: JoinAll,
		OnFail:       OnFailContinue,
	}

	results, err := executor.ExecuteAndWait(req)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "agent1", results[0].AgentID)
}
