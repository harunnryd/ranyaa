package toolexecutor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testApprovalForwarder struct {
	pending chan PendingApproval
}

func (f *testApprovalForwarder) ForwardApproval(_ context.Context, pending PendingApproval) error {
	f.pending <- pending
	return nil
}

func TestParseApprovalAction(t *testing.T) {
	action, err := ParseApprovalAction("allow-once")
	require.NoError(t, err)
	assert.Equal(t, ApprovalActionAllowOnce, action)

	action, err = ParseApprovalAction("ALLOW-ALWAYS")
	require.NoError(t, err)
	assert.Equal(t, ApprovalActionAllowAlways, action)

	_, err = ParseApprovalAction("invalid")
	require.Error(t, err)
}

func TestChatApprovalHandler_AllowOnce(t *testing.T) {
	forwarder := &testApprovalForwarder{pending: make(chan PendingApproval, 1)}
	handler := NewChatApprovalHandler(forwarder, nil)
	manager := NewApprovalManager(handler)

	done := make(chan struct{})
	var approved bool
	var err error

	go func() {
		defer close(done)
		approved, err = manager.RequestApproval(context.Background(), ApprovalRequest{
			Command: "shell_exec",
			Args:    []string{"echo", "hello"},
		})
	}()

	pending := <-forwarder.pending
	require.NotEmpty(t, pending.ID)

	require.NoError(t, manager.ResolveApproval(pending.ID, ApprovalActionAllowOnce, "@alice"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval result")
	}

	require.NoError(t, err)
	assert.True(t, approved)
}

func TestChatApprovalHandler_Deny(t *testing.T) {
	forwarder := &testApprovalForwarder{pending: make(chan PendingApproval, 1)}
	handler := NewChatApprovalHandler(forwarder, nil)
	manager := NewApprovalManager(handler)

	done := make(chan struct{})
	var approved bool
	var err error

	go func() {
		defer close(done)
		approved, err = manager.RequestApproval(context.Background(), ApprovalRequest{
			Command: "shell_exec",
			Args:    []string{"rm", "-rf", "/tmp/test"},
		})
	}()

	pending := <-forwarder.pending
	require.NoError(t, manager.ResolveApproval(pending.ID, ApprovalActionDeny, "@bob"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval result")
	}

	require.NoError(t, err)
	assert.False(t, approved)
}

func TestChatApprovalHandler_AllowAlwaysPersistsAllowlist(t *testing.T) {
	forwarder := &testApprovalForwarder{pending: make(chan PendingApproval, 1)}
	allowlistPath := filepath.Join(t.TempDir(), "exec-approvals.json")
	allowlist, err := NewAllowlistManager(allowlistPath)
	require.NoError(t, err)

	handler := NewChatApprovalHandler(forwarder, allowlist)
	manager := NewApprovalManager(handler)

	done := make(chan struct{})
	var approved bool
	var approveErr error

	go func() {
		defer close(done)
		approved, approveErr = manager.RequestApproval(context.Background(), ApprovalRequest{
			Command: "shell_exec",
			Args:    []string{"echo", "persist"},
		})
	}()

	pending := <-forwarder.pending
	require.NoError(t, manager.ResolveApproval(pending.ID, ApprovalActionAllowAlways, "@carol"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval result")
	}

	require.NoError(t, approveErr)
	assert.True(t, approved)
	assert.True(t, allowlist.IsAllowed("shell_exec", []string{"echo", "persist"}))
}
