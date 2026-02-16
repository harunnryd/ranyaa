package telegram

import (
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

// Handler implements message handling for Telegram
type Handler struct {
	bot    *Bot
	logger zerolog.Logger

	// Callback for processing messages
	onMessage func(MessageContext) error
}

// MessageContext contains message metadata
type MessageContext struct {
	ChatID      int64
	MessageID   int
	UserID      int64
	Username    string
	Text        string
	Timestamp   time.Time
	IsGroup     bool
	IsMention   bool
	ReplyToID   int
	MediaType   string
	MediaFileID string
}

// NewHandler creates a new message handler
func NewHandler(bot *Bot) *Handler {
	return &Handler{
		bot:    bot,
		logger: bot.logger.With().Str("module", "handler").Logger(),
	}
}

// HandleMessage processes incoming messages
func (h *Handler) HandleMessage(update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	msg := update.Message

	// Parse message context
	ctx := MessageContext{
		ChatID:    msg.Chat.ID,
		MessageID: msg.MessageID,
		UserID:    msg.From.ID,
		Username:  msg.From.UserName,
		Text:      msg.Text,
		Timestamp: time.Unix(int64(msg.Date), 0),
		IsGroup:   msg.Chat.IsGroup() || msg.Chat.IsSuperGroup(),
	}

	// Check if bot is mentioned (for groups)
	if ctx.IsGroup {
		ctx.IsMention = h.isMentioned(msg)
	}

	// Check for reply
	if msg.ReplyToMessage != nil {
		ctx.ReplyToID = msg.ReplyToMessage.MessageID
	}

	// Check for media
	if msg.Photo != nil {
		ctx.MediaType = "photo"
		if len(msg.Photo) > 0 {
			ctx.MediaFileID = msg.Photo[len(msg.Photo)-1].FileID
		}
	} else if msg.Video != nil {
		ctx.MediaType = "video"
		ctx.MediaFileID = msg.Video.FileID
	} else if msg.Audio != nil {
		ctx.MediaType = "audio"
		ctx.MediaFileID = msg.Audio.FileID
	} else if msg.Document != nil {
		ctx.MediaType = "document"
		ctx.MediaFileID = msg.Document.FileID
	} else if msg.Voice != nil {
		ctx.MediaType = "voice"
		ctx.MediaFileID = msg.Voice.FileID
	}

	h.logger.Debug().
		Int64("chat_id", ctx.ChatID).
		Int64("user_id", ctx.UserID).
		Str("username", ctx.Username).
		Str("media_type", ctx.MediaType).
		Bool("is_group", ctx.IsGroup).
		Bool("is_mention", ctx.IsMention).
		Msg("Message received")

	// Call callback if set
	if h.onMessage != nil {
		return h.onMessage(ctx)
	}

	return nil
}

// isMentioned checks if the bot is mentioned in a message
func (h *Handler) isMentioned(msg *tgbotapi.Message) bool {
	// Check entities for mentions
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			mention := msg.Text[entity.Offset : entity.Offset+entity.Length]
			if mention == "@"+h.bot.api.Self.UserName {
				return true
			}
		}
	}

	return false
}

// SetOnMessage sets the message callback
func (h *Handler) SetOnMessage(callback func(MessageContext) error) {
	h.onMessage = callback
}

// SendResponse sends a response to a message
func (h *Handler) SendResponse(ctx MessageContext, text string) error {
	return h.bot.SendMessageWithReply(ctx.ChatID, text, ctx.MessageID)
}

// SendTyping sends typing action
func (h *Handler) SendTyping(chatID int64) error {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, err := h.bot.api.Send(action)
	if err != nil {
		return fmt.Errorf("failed to send typing action: %w", err)
	}
	return nil
}

// ParseCaption extracts caption from a message
func ParseCaption(msg *tgbotapi.Message) string {
	if msg.Caption != "" {
		return msg.Caption
	}
	return msg.Text
}

// GetChatInfo returns chat information
func (h *Handler) GetChatInfo(chatID int64) (map[string]interface{}, error) {
	chat, err := h.bot.api.GetChat(tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: chatID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get chat info: %w", err)
	}

	return map[string]interface{}{
		"id":        chat.ID,
		"type":      chat.Type,
		"title":     chat.Title,
		"username":  chat.UserName,
		"firstName": chat.FirstName,
		"lastName":  chat.LastName,
	}, nil
}
