package toolexecutor

import (
	"context"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTelegramApprovalHandler_E2E_CompleteWorkflow tests the complete approval workflow
// NOTE: This test uses a mock bot API since we can't test with a real Telegram bot in CI
func TestTelegramApprovalHandler_E2E_CompleteWorkflow(t *testing.T) {
	// Scenario 1: Complete approval workflow with approve
	t.Run("complete workflow - approve", func(t *testing.T) {
		mockAPI := &MockBotAPI{}
		handler := NewTelegramApprovalHandler(mockAPI, 123456)
		manager := NewApprovalManager(handler)

		// Start approval request in background
		req := ApprovalRequest{
			Command: "git",
			Args:    []string{"push", "origin", "main"},
			Cwd:     "/home/user/project",
			AgentID: "executor-1",
			Timeout: 5 * time.Second,
			Context: map[string]string{
				"branch": "main",
				"remote": "origin",
			},
		}

		// Create channels for coordination
		approvalDone := make(chan bool, 1)
		var approved bool
		var approvalErr error

		// Request approval in background
		go func() {
			ctx := context.Background()
			approved, approvalErr = manager.RequestApproval(ctx, req)
			approvalDone <- true
		}()

		// Wait a bit for approval request to be registered
		time.Sleep(100 * time.Millisecond)

		// Find the pending approval (in real scenario, this would come from Telegram callback)
		handler.mu.RLock()
		var callbackID string
		for id := range handler.pendingApprovals {
			callbackID = id
			break
		}
		handler.mu.RUnlock()

		require.NotEmpty(t, callbackID, "no pending approval found")

		// Simulate user clicking "Approve" button
		callback := &tgbotapi.CallbackQuery{
			ID:   "callback_test",
			Data: callbackID + "_approve",
			From: &tgbotapi.User{
				UserName: "testuser",
			},
		}

		err := handler.HandleCallback(callback)
		require.NoError(t, err)

		// Wait for approval to complete
		select {
		case <-approvalDone:
			require.NoError(t, approvalErr)
			assert.True(t, approved)
		case <-time.After(2 * time.Second):
			t.Fatal("approval did not complete in time")
		}
	})

	// Scenario 2: Complete approval workflow with deny
	t.Run("complete workflow - deny", func(t *testing.T) {
		mockAPI := &MockBotAPI{}
		handler := NewTelegramApprovalHandler(mockAPI, 123456)
		manager := NewApprovalManager(handler)

		req := ApprovalRequest{
			Command: "rm",
			Args:    []string{"-rf", "/"},
			AgentID: "executor-1",
			Timeout: 5 * time.Second,
		}

		// Create channels for coordination
		approvalDone := make(chan bool, 1)
		var approved bool
		var approvalErr error

		// Request approval in background
		go func() {
			ctx := context.Background()
			approved, approvalErr = manager.RequestApproval(ctx, req)
			approvalDone <- true
		}()

		// Wait a bit for approval request to be registered
		time.Sleep(100 * time.Millisecond)

		// Find the pending approval
		handler.mu.RLock()
		var callbackID string
		for id := range handler.pendingApprovals {
			callbackID = id
			break
		}
		handler.mu.RUnlock()

		require.NotEmpty(t, callbackID, "no pending approval found")

		// Simulate user clicking "Deny" button
		callback := &tgbotapi.CallbackQuery{
			ID:   "callback_test",
			Data: callbackID + "_deny",
			From: &tgbotapi.User{
				UserName: "testuser",
			},
		}

		err := handler.HandleCallback(callback)
		require.NoError(t, err)

		// Wait for approval to complete
		select {
		case <-approvalDone:
			require.NoError(t, approvalErr)
			assert.False(t, approved)
		case <-time.After(2 * time.Second):
			t.Fatal("approval did not complete in time")
		}
	})

	// Scenario 3: Timeout scenario
	t.Run("complete workflow - timeout", func(t *testing.T) {
		mockAPI := &MockBotAPI{}
		handler := NewTelegramApprovalHandler(mockAPI, 123456)
		manager := NewApprovalManager(handler)
		manager.SetDefaultTimeout(200 * time.Millisecond)

		req := ApprovalRequest{
			Command: "dangerous-command",
			AgentID: "executor-1",
		}

		ctx := context.Background()
		approved, err := manager.RequestApproval(ctx, req)

		assert.Error(t, err)
		assert.False(t, approved)
		assert.Contains(t, err.Error(), "timed out")
	})

	// Scenario 4: Multiple concurrent approvals
	t.Run("multiple concurrent approvals", func(t *testing.T) {
		mockAPI := &MockBotAPI{}
		handler := NewTelegramApprovalHandler(mockAPI, 123456)
		manager := NewApprovalManager(handler)

		numRequests := 5
		results := make(chan bool, numRequests)
		errors := make(chan error, numRequests)

		// Start multiple approval requests
		for i := 0; i < numRequests; i++ {
			go func(idx int) {
				req := ApprovalRequest{
					Command: "test",
					Args:    []string{string(rune('0' + idx))},
					AgentID: "executor-1",
					Timeout: 5 * time.Second,
				}

				ctx := context.Background()
				approved, err := manager.RequestApproval(ctx, req)
				if err != nil {
					errors <- err
				} else {
					results <- approved
				}
			}(i)
		}

		// Wait for all requests to be registered
		deadline := time.After(2 * time.Second)
		for {
			handler.mu.RLock()
			pendingCount := len(handler.pendingApprovals)
			handler.mu.RUnlock()

			if pendingCount == numRequests {
				break
			}

			select {
			case <-deadline:
				t.Fatalf("expected %d pending approvals, got %d", numRequests, pendingCount)
			case <-time.After(10 * time.Millisecond):
			}
		}

		// Approve all pending requests
		handler.mu.RLock()
		callbackIDs := make([]string, 0, len(handler.pendingApprovals))
		for id := range handler.pendingApprovals {
			callbackIDs = append(callbackIDs, id)
		}
		handler.mu.RUnlock()

		for _, callbackID := range callbackIDs {
			callback := &tgbotapi.CallbackQuery{
				ID:   "callback_" + callbackID,
				Data: callbackID + "_approve",
				From: &tgbotapi.User{
					UserName: "testuser",
				},
			}

			err := handler.HandleCallback(callback)
			require.NoError(t, err)
		}

		// Collect results
		for i := 0; i < numRequests; i++ {
			select {
			case approved := <-results:
				assert.True(t, approved)
			case err := <-errors:
				t.Fatalf("approval %d failed: %v", i, err)
			case <-time.After(3 * time.Second):
				t.Fatalf("approval %d timed out", i)
			}
		}
	})
}
