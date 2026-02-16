package toolexecutor

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
)

// TelegramAPI is an interface for Telegram Bot API operations
type TelegramAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// TelegramApprovalHandler handles approval requests via Telegram inline buttons
type TelegramApprovalHandler struct {
	api    TelegramAPI
	chatID int64

	// Pending approvals
	mu               sync.RWMutex
	pendingApprovals map[string]chan ApprovalResponse
}

// NewTelegramApprovalHandler creates a new Telegram approval handler
func NewTelegramApprovalHandler(api TelegramAPI, chatID int64) *TelegramApprovalHandler {
	return &TelegramApprovalHandler{
		api:              api,
		chatID:           chatID,
		pendingApprovals: make(map[string]chan ApprovalResponse),
	}
}

// RequestApproval sends an approval request to Telegram with inline buttons
func (t *TelegramApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	if t.api == nil {
		return ApprovalResponse{}, fmt.Errorf("telegram API is not initialized")
	}

	// Generate unique callback ID
	callbackID := fmt.Sprintf("approval_%d", time.Now().UnixNano())

	// Create approval message
	text := t.formatApprovalMessage(req)

	// Create inline keyboard with Approve/Deny buttons
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Approve", callbackID+"_approve"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Deny", callbackID+"_deny"),
		),
	)

	// Send message
	msg := tgbotapi.NewMessage(t.chatID, text)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"

	sentMsg, err := t.api.Send(msg)
	if err != nil {
		return ApprovalResponse{}, fmt.Errorf("failed to send approval request: %w", err)
	}

	log.Info().
		Int64("chat_id", t.chatID).
		Int("message_id", sentMsg.MessageID).
		Str("callback_id", callbackID).
		Msg("Approval request sent to Telegram")

	// Create response channel
	responseChan := make(chan ApprovalResponse, 1)

	// Register pending approval
	t.mu.Lock()
	t.pendingApprovals[callbackID] = responseChan
	t.mu.Unlock()

	// Clean up on return
	defer func() {
		t.mu.Lock()
		delete(t.pendingApprovals, callbackID)
		t.mu.Unlock()
	}()

	// Wait for response or timeout
	select {
	case response := <-responseChan:
		// Update message to show result
		t.updateMessageWithResult(sentMsg.MessageID, req, response)
		return response, nil

	case <-ctx.Done():
		// Update message to show timeout
		t.updateMessageWithTimeout(sentMsg.MessageID, req)
		return ApprovalResponse{
			Approved: false,
			Reason:   "timeout",
		}, ctx.Err()
	}
}

// HandleCallback handles callback queries from inline buttons
func (t *TelegramApprovalHandler) HandleCallback(callback *tgbotapi.CallbackQuery) error {
	if callback == nil {
		return fmt.Errorf("callback is nil")
	}

	data := callback.Data

	// Parse callback data
	var callbackID string
	var action string

	if len(data) > 8 && data[len(data)-8:] == "_approve" {
		callbackID = data[:len(data)-8]
		action = "approve"
	} else if len(data) > 5 && data[len(data)-5:] == "_deny" {
		callbackID = data[:len(data)-5]
		action = "deny"
	} else {
		// Not an approval callback
		return nil
	}

	// Find pending approval
	t.mu.RLock()
	responseChan, exists := t.pendingApprovals[callbackID]
	t.mu.RUnlock()

	if !exists {
		// Approval already processed or expired
		t.answerCallback(callback.ID, "This approval request has expired")
		return nil
	}

	// Create response
	var response ApprovalResponse
	if action == "approve" {
		response = ApprovalResponse{
			Approved: true,
			Reason:   fmt.Sprintf("approved by @%s", callback.From.UserName),
		}
	} else {
		response = ApprovalResponse{
			Approved: false,
			Reason:   fmt.Sprintf("denied by @%s", callback.From.UserName),
		}
	}

	// Send response
	select {
	case responseChan <- response:
		// Answer callback
		var answerText string
		if response.Approved {
			answerText = "✅ Approved"
		} else {
			answerText = "❌ Denied"
		}
		t.answerCallback(callback.ID, answerText)

		log.Info().
			Str("callback_id", callbackID).
			Bool("approved", response.Approved).
			Str("user", callback.From.UserName).
			Msg("Approval response received")

	default:
		// Channel full or closed
		t.answerCallback(callback.ID, "Failed to process response")
	}

	return nil
}

// formatApprovalMessage formats the approval request message
func (t *TelegramApprovalHandler) formatApprovalMessage(req ApprovalRequest) string {
	text := "🔐 *Exec Approval Required*\n\n"
	text += fmt.Sprintf("*Command:* `%s`\n", req.Command)

	if len(req.Args) > 0 {
		text += fmt.Sprintf("*Args:* `%v`\n", req.Args)
	}

	if req.Cwd != "" {
		text += fmt.Sprintf("*Working Directory:* `%s`\n", req.Cwd)
	}

	if req.AgentID != "" {
		text += fmt.Sprintf("*Agent:* `%s`\n", req.AgentID)
	}

	if req.Timeout > 0 {
		text += fmt.Sprintf("*Timeout:* `%v`\n", req.Timeout)
	}

	text += "\nDo you want to approve this command?"

	return text
}

// updateMessageWithResult updates the message to show the approval result
func (t *TelegramApprovalHandler) updateMessageWithResult(messageID int, req ApprovalRequest, response ApprovalResponse) {
	var text string
	if response.Approved {
		text = "✅ *Approved*\n\n"
	} else {
		text = "❌ *Denied*\n\n"
	}

	text += fmt.Sprintf("*Command:* `%s`\n", req.Command)
	if len(req.Args) > 0 {
		text += fmt.Sprintf("*Args:* `%v`\n", req.Args)
	}
	text += fmt.Sprintf("*Reason:* %s", response.Reason)

	edit := tgbotapi.NewEditMessageText(t.chatID, messageID, text)
	edit.ParseMode = "Markdown"

	_, err := t.api.Send(edit)
	if err != nil {
		log.Error().
			Err(err).
			Int("message_id", messageID).
			Msg("Failed to update message with result")
	}
}

// updateMessageWithTimeout updates the message to show timeout
func (t *TelegramApprovalHandler) updateMessageWithTimeout(messageID int, req ApprovalRequest) {
	text := "⏱️ *Timeout*\n\n"
	text += fmt.Sprintf("*Command:* `%s`\n", req.Command)
	if len(req.Args) > 0 {
		text += fmt.Sprintf("*Args:* `%v`\n", req.Args)
	}
	text += "*Reason:* Approval request timed out"

	edit := tgbotapi.NewEditMessageText(t.chatID, messageID, text)
	edit.ParseMode = "Markdown"

	_, err := t.api.Send(edit)
	if err != nil {
		log.Error().
			Err(err).
			Int("message_id", messageID).
			Msg("Failed to update message with timeout")
	}
}

// answerCallback answers a callback query
func (t *TelegramApprovalHandler) answerCallback(callbackID string, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	_, err := t.api.Request(callback)
	if err != nil {
		log.Error().
			Err(err).
			Str("callback_id", callbackID).
			Msg("Failed to answer callback")
	}
}

// SetChatID sets the chat ID for approval requests
func (t *TelegramApprovalHandler) SetChatID(chatID int64) {
	t.chatID = chatID
}

// GetChatID returns the current chat ID
func (t *TelegramApprovalHandler) GetChatID() int64 {
	return t.chatID
}
