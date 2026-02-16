package toolexecutor

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockBotAPI is a mock Telegram Bot API for testing
type MockBotAPI struct {
	sentMessages   []tgbotapi.Chattable
	editedMessages []tgbotapi.Chattable
	callbacks      []tgbotapi.CallbackConfig
}

func (m *MockBotAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{
		MessageID: 123,
		Chat:      &tgbotapi.Chat{ID: 456},
	}, nil
}

func (m *MockBotAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	if callback, ok := c.(tgbotapi.CallbackConfig); ok {
		m.callbacks = append(m.callbacks, callback)
	} else {
		m.editedMessages = append(m.editedMessages, c)
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestNewTelegramApprovalHandler(t *testing.T) {
	api := &tgbotapi.BotAPI{}
	chatID := int64(123456)

	handler := NewTelegramApprovalHandler(api, chatID)

	assert.NotNil(t, handler)
	assert.Equal(t, api, handler.api)
	assert.Equal(t, chatID, handler.chatID)
	assert.NotNil(t, handler.pendingApprovals)
	assert.Empty(t, handler.pendingApprovals)
}

func TestTelegramApprovalHandler_FormatApprovalMessage(t *testing.T) {
	handler := &TelegramApprovalHandler{}

	tests := []struct {
		name     string
		req      ApprovalRequest
		contains []string
	}{
		{
			name: "basic command",
			req: ApprovalRequest{
				Command: "ls",
			},
			contains: []string{"Exec Approval Required", "Command:", "ls"},
		},
		{
			name: "command with args",
			req: ApprovalRequest{
				Command: "rm",
				Args:    []string{"-rf", "/tmp/test"},
			},
			contains: []string{"Command:", "rm", "Args:", "-rf", "/tmp/test"},
		},
		{
			name: "command with cwd",
			req: ApprovalRequest{
				Command: "make",
				Cwd:     "/home/user/project",
			},
			contains: []string{"Command:", "make", "Working Directory:", "/home/user/project"},
		},
		{
			name: "command with agent",
			req: ApprovalRequest{
				Command: "git",
				Args:    []string{"push"},
				AgentID: "executor-1",
			},
			contains: []string{"Command:", "git", "Agent:", "executor-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := handler.formatApprovalMessage(tt.req)

			for _, substr := range tt.contains {
				assert.Contains(t, text, substr)
			}
		})
	}
}

func TestTelegramApprovalHandler_HandleCallback_Approve(t *testing.T) {
	mockAPI := &MockBotAPI{}
	handler := &TelegramApprovalHandler{
		api:              mockAPI,
		chatID:           123456,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}

	// Create pending approval
	callbackID := "approval_123"
	responseChan := make(chan ApprovalResponse, 1)
	handler.pendingApprovals[callbackID] = responseChan

	// Create callback query
	callback := &tgbotapi.CallbackQuery{
		ID:   "callback_1",
		Data: callbackID + "_approve",
		From: &tgbotapi.User{
			UserName: "testuser",
		},
	}

	// Handle callback
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

func TestTelegramApprovalHandler_HandleCallback_Deny(t *testing.T) {
	mockAPI := &MockBotAPI{}
	handler := &TelegramApprovalHandler{
		api:              mockAPI,
		chatID:           123456,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}

	// Create pending approval
	callbackID := "approval_456"
	responseChan := make(chan ApprovalResponse, 1)
	handler.pendingApprovals[callbackID] = responseChan

	// Create callback query
	callback := &tgbotapi.CallbackQuery{
		ID:   "callback_2",
		Data: callbackID + "_deny",
		From: &tgbotapi.User{
			UserName: "testuser",
		},
	}

	// Handle callback
	err := handler.HandleCallback(callback)
	require.NoError(t, err)

	// Check response
	select {
	case response := <-responseChan:
		assert.False(t, response.Approved)
		assert.Contains(t, response.Reason, "denied by @testuser")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestTelegramApprovalHandler_HandleCallback_Expired(t *testing.T) {
	mockAPI := &MockBotAPI{}
	handler := &TelegramApprovalHandler{
		api:              mockAPI,
		chatID:           123456,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}

	// Create callback query for non-existent approval
	callback := &tgbotapi.CallbackQuery{
		ID:   "callback_3",
		Data: "approval_999_approve",
		From: &tgbotapi.User{
			UserName: "testuser",
		},
	}

	// Handle callback (should not error)
	err := handler.HandleCallback(callback)
	require.NoError(t, err)
}

func TestTelegramApprovalHandler_SetGetChatID(t *testing.T) {
	handler := &TelegramApprovalHandler{
		chatID: 123,
	}

	assert.Equal(t, int64(123), handler.GetChatID())

	handler.SetChatID(456)
	assert.Equal(t, int64(456), handler.GetChatID())
}

func TestTelegramApprovalHandler_HandleCallback_NilCallback(t *testing.T) {
	handler := &TelegramApprovalHandler{
		api:              (*tgbotapi.BotAPI)(nil),
		chatID:           123456,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}

	err := handler.HandleCallback(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback is nil")
}

func TestTelegramApprovalHandler_HandleCallback_InvalidData(t *testing.T) {
	handler := &TelegramApprovalHandler{
		api:              (*tgbotapi.BotAPI)(nil),
		chatID:           123456,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}

	callback := &tgbotapi.CallbackQuery{
		ID:   "callback_4",
		Data: "invalid_data",
		From: &tgbotapi.User{
			UserName: "testuser",
		},
	}

	// Should not error, just ignore
	err := handler.HandleCallback(callback)
	require.NoError(t, err)
}
