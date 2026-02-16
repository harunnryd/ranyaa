package toolexecutor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApprovalWorkflow_Integration tests the complete approval workflow
func TestApprovalWorkflow_Integration(t *testing.T) {
	handler := &MockApprovalHandler{
		Response: ApprovalResponse{
			Approved: true,
			Reason:   "integration test approval",
		},
	}
	am := NewApprovalManager(handler)

	req := ApprovalRequest{
		Command: "echo",
		Args:    []string{"hello", "world"},
		Cwd:     "/tmp",
		AgentID: "integration-test-agent",
		Context: map[string]string{
			"session_id": "test-session",
			"user_id":    "test-user",
		},
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, approved)
}

// TestApprovalWorkflow_MultipleRequests tests handling multiple approval requests
func TestApprovalWorkflow_MultipleRequests(t *testing.T) {
	handler := &MockApprovalHandler{
		AutoApprove: true,
		Delay:       10 * time.Millisecond,
	}
	am := NewApprovalManager(handler)

	requests := []ApprovalRequest{
		{Command: "ls", Args: []string{"-la"}, AgentID: "agent-1"},
		{Command: "pwd", Args: []string{}, AgentID: "agent-2"},
		{Command: "echo", Args: []string{"test"}, AgentID: "agent-3"},
	}

	for _, req := range requests {
		approved, err := am.RequestApproval(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, approved)
	}
}

// TestApprovalWorkflow_ConcurrentRequests tests concurrent approval requests
func TestApprovalWorkflow_ConcurrentRequests(t *testing.T) {
	handler := &MockApprovalHandler{
		AutoApprove: true,
		Delay:       20 * time.Millisecond,
	}
	am := NewApprovalManager(handler)

	numRequests := 10
	var wg sync.WaitGroup
	results := make([]bool, numRequests)
	errors := make([]error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			req := ApprovalRequest{
				Command: "test",
				Args:    []string{string(rune(index))},
				AgentID: "concurrent-agent",
			}

			approved, err := am.RequestApproval(context.Background(), req)
			results[index] = approved
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded
	for i := 0; i < numRequests; i++ {
		require.NoError(t, errors[i], "Request %d failed", i)
		assert.True(t, results[i], "Request %d not approved", i)
	}
}
