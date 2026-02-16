package toolexecutor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApprovalManager(t *testing.T) {
	handler := &MockApprovalHandler{AutoApprove: true}
	am := NewApprovalManager(handler)

	assert.NotNil(t, am)
	assert.Equal(t, 60*time.Second, am.GetDefaultTimeout())
}

func TestApprovalManager_RequestApproval_Approved(t *testing.T) {
	handler := &MockApprovalHandler{
		Response: ApprovalResponse{
			Approved: true,
			Reason:   "test approval",
		},
	}
	am := NewApprovalManager(handler)

	req := ApprovalRequest{
		Command: "ls",
		Args:    []string{"-la"},
		Cwd:     "/tmp",
		AgentID: "test-agent",
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, approved)
}

func TestApprovalManager_RequestApproval_Denied(t *testing.T) {
	handler := &MockApprovalHandler{
		Response: ApprovalResponse{
			Approved: false,
			Reason:   "test denial",
		},
	}
	am := NewApprovalManager(handler)

	req := ApprovalRequest{
		Command: "rm",
		Args:    []string{"-rf", "/"},
		Cwd:     "/",
		AgentID: "test-agent",
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, approved)
}

func TestApprovalManager_RequestApproval_Timeout(t *testing.T) {
	handler := &MockApprovalHandler{
		Delay: 2 * time.Second, // Longer than timeout
	}
	am := NewApprovalManager(handler)
	am.SetDefaultTimeout(100 * time.Millisecond)

	req := ApprovalRequest{
		Command: "sleep",
		Args:    []string{"10"},
		AgentID: "test-agent",
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.Error(t, err)
	assert.False(t, approved)
	assert.Contains(t, err.Error(), "timed out")
}

func TestApprovalManager_RequestApproval_Error(t *testing.T) {
	handler := &MockApprovalHandler{
		Error: errors.New("handler error"),
	}
	am := NewApprovalManager(handler)

	req := ApprovalRequest{
		Command: "test",
		AgentID: "test-agent",
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.Error(t, err)
	assert.False(t, approved)
	assert.Contains(t, err.Error(), "handler error")
}

func TestApprovalManager_SetDefaultTimeout(t *testing.T) {
	handler := &MockApprovalHandler{AutoApprove: true}
	am := NewApprovalManager(handler)

	newTimeout := 30 * time.Second
	am.SetDefaultTimeout(newTimeout)

	assert.Equal(t, newTimeout, am.GetDefaultTimeout())
}

func TestApprovalManager_NoHandler(t *testing.T) {
	am := NewApprovalManager(nil)

	req := ApprovalRequest{
		Command: "test",
		AgentID: "test-agent",
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.Error(t, err)
	assert.False(t, approved)
	assert.Contains(t, err.Error(), "no approval handler")
}

func TestApprovalManager_CustomTimeout(t *testing.T) {
	handler := &MockApprovalHandler{
		Delay: 50 * time.Millisecond,
		Response: ApprovalResponse{
			Approved: true,
			Reason:   "approved",
		},
	}
	am := NewApprovalManager(handler)

	req := ApprovalRequest{
		Command: "test",
		AgentID: "test-agent",
		Timeout: 200 * time.Millisecond, // Custom timeout
	}

	approved, err := am.RequestApproval(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, approved)
}

func TestMockApprovalHandler_AutoApprove(t *testing.T) {
	handler := &MockApprovalHandler{
		AutoApprove: true,
	}

	req := ApprovalRequest{
		Command: "test",
		AgentID: "test-agent",
	}

	response, err := handler.RequestApproval(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, response.Approved)
	assert.Equal(t, "auto-approved", response.Reason)
}

func TestMockApprovalHandler_ContextCancellation(t *testing.T) {
	handler := &MockApprovalHandler{
		Delay: 1 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := ApprovalRequest{
		Command: "test",
		AgentID: "test-agent",
	}

	_, err := handler.RequestApproval(ctx, req)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// MockApprovalHandler is a mock implementation of ApprovalHandler for testing
type MockApprovalHandler struct {
	Response    ApprovalResponse
	Error       error
	Delay       time.Duration
	AutoApprove bool
}

func (m *MockApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	// Simulate delay if specified
	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return ApprovalResponse{}, ctx.Err()
		}
	}

	// Return error if specified
	if m.Error != nil {
		return ApprovalResponse{}, m.Error
	}

	// Auto-approve if enabled
	if m.AutoApprove {
		return ApprovalResponse{
			Approved: true,
			Reason:   "auto-approved",
		}, nil
	}

	// Return configured response
	return m.Response, nil
}
