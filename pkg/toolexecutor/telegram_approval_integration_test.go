package toolexecutor

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramApprovalHandler_Integration_ApprovalWorkflow(t *testing.T) {
	// Create handler with nil API (for testing)
	handler := NewTelegramApprovalHandler(nil, 123456)

	// Test 1: Register pending approval and handle callback
	callbackID := "approval_test_123"
	responseChan := make(chan ApprovalResponse, 1)
	handler.pendingApprovals[callbackID] = responseChan

	// Simulate callback
	callback := &tgbotapi.CallbackQuery{
		ID:   "callback_1",
		Data: callbackID + "_approve",
		From: &tgbotapi.User{
			UserName: "testuser",
		},
	}

	err := handler.HandleCallback(callback)
	require.NoError(t, err)

	// Check response
	select {
	case response := <-responseChan:
		assert.True(t, response.Approved)
		assert.Contains(t, response.Reason, "approved by @testuser")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestTelegramApprovalHandler_Integration_MultipleApprovals(t *testing.T) {
	handler := NewTelegramApprovalHandler(nil, 123456)

	// Create multiple pending approvals
	approvals := []struct {
		callbackID string
		action     string
		expected   bool
	}{
		{"approval_1", "approve", true},
		{"approval_2", "deny", false},
		{"approval_3", "approve", true},
	}

	for _, approval := range approvals {
		responseChan := make(chan ApprovalResponse, 1)
		handler.pendingApprovals[approval.callbackID] = responseChan

		// Handle callback
		callback := &tgbotapi.CallbackQuery{
			ID:   "callback_" + approval.callbackID,
			Data: approval.callbackID + "_" + approval.action,
			From: &tgbotapi.User{
				UserName: "testuser",
			},
		}

		err := handler.HandleCallback(callback)
		require.NoError(t, err)

		// Check response
		select {
		case response := <-responseChan:
			assert.Equal(t, approval.expected, response.Approved)
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for response for %s", approval.callbackID)
		}
	}
}

func TestTelegramApprovalHandler_Integration_ConcurrentCallbacks(t *testing.T) {
	handler := NewTelegramApprovalHandler(nil, 123456)

	// Create multiple pending approvals
	numApprovals := 10
	responseChans := make([]chan ApprovalResponse, numApprovals)

	for i := 0; i < numApprovals; i++ {
		callbackID := "approval_concurrent_" + string(rune('0'+i))
		responseChan := make(chan ApprovalResponse, 1)
		responseChans[i] = responseChan
		handler.pendingApprovals[callbackID] = responseChan
	}

	// Handle callbacks concurrently
	done := make(chan bool, numApprovals)
	for i := 0; i < numApprovals; i++ {
		go func(idx int) {
			callbackID := "approval_concurrent_" + string(rune('0'+idx))
			callback := &tgbotapi.CallbackQuery{
				ID:   "callback_" + callbackID,
				Data: callbackID + "_approve",
				From: &tgbotapi.User{
					UserName: "testuser",
				},
			}

			err := handler.HandleCallback(callback)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all callbacks to complete
	for i := 0; i < numApprovals; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for concurrent callbacks")
		}
	}

	// Check all responses
	for i := 0; i < numApprovals; i++ {
		select {
		case response := <-responseChans[i]:
			assert.True(t, response.Approved)
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for response %d", i)
		}
	}
}
