package telegram

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestBot(t *testing.T) *Bot {
	log, err := logger.New(logger.Config{
		Level:   "info",
		Console: true,
	})
	require.NoError(t, err)

	// Create a bot with dummy API (won't actually connect)
	bot := &Bot{
		logger: log.GetZerolog().With().Str("component", "telegram").Logger(),
		api: &tgbotapi.BotAPI{
			Self: tgbotapi.User{
				UserName: "testbot",
				ID:       123456789,
			},
		},
	}

	return bot
}

func TestNewHandler(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	assert.NotNil(t, handler)
	assert.Equal(t, bot, handler.bot)
}

func TestHandleMessage_TextMessage(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	var receivedCtx MessageContext
	handler.SetOnMessage(func(ctx MessageContext) error {
		receivedCtx = ctx
		return nil
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From: &tgbotapi.User{
				ID:       12345,
				UserName: "testuser",
			},
			Chat: &tgbotapi.Chat{
				ID:   67890,
				Type: "private",
			},
			Text: "Hello bot",
			Date: int(time.Now().Unix()),
		},
	}

	err := handler.HandleMessage(update)
	assert.NoError(t, err)
	assert.Equal(t, int64(67890), receivedCtx.ChatID)
	assert.Equal(t, int64(12345), receivedCtx.UserID)
	assert.Equal(t, "testuser", receivedCtx.Username)
	assert.Equal(t, "Hello bot", receivedCtx.Text)
	assert.False(t, receivedCtx.IsGroup)
}

func TestHandleMessage_GroupMessage(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	var receivedCtx MessageContext
	handler.SetOnMessage(func(ctx MessageContext) error {
		receivedCtx = ctx
		return nil
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From: &tgbotapi.User{
				ID:       12345,
				UserName: "testuser",
			},
			Chat: &tgbotapi.Chat{
				ID:   67890,
				Type: "group",
			},
			Text: "Hello everyone",
			Date: int(time.Now().Unix()),
		},
	}

	err := handler.HandleMessage(update)
	assert.NoError(t, err)
	assert.True(t, receivedCtx.IsGroup)
}

func TestHandleMessage_WithMention(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	var receivedCtx MessageContext
	handler.SetOnMessage(func(ctx MessageContext) error {
		receivedCtx = ctx
		return nil
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From: &tgbotapi.User{
				ID:       12345,
				UserName: "testuser",
			},
			Chat: &tgbotapi.Chat{
				ID:   67890,
				Type: "group",
			},
			Text: "@testbot hello",
			Date: int(time.Now().Unix()),
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "mention",
					Offset: 0,
					Length: 8,
				},
			},
		},
	}

	err := handler.HandleMessage(update)
	assert.NoError(t, err)
	assert.True(t, receivedCtx.IsMention)
}

func TestHandleMessage_WithReply(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	var receivedCtx MessageContext
	handler.SetOnMessage(func(ctx MessageContext) error {
		receivedCtx = ctx
		return nil
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 2,
			From: &tgbotapi.User{
				ID:       12345,
				UserName: "testuser",
			},
			Chat: &tgbotapi.Chat{
				ID:   67890,
				Type: "private",
			},
			Text: "Reply text",
			Date: int(time.Now().Unix()),
			ReplyToMessage: &tgbotapi.Message{
				MessageID: 1,
			},
		},
	}

	err := handler.HandleMessage(update)
	assert.NoError(t, err)
	assert.Equal(t, 1, receivedCtx.ReplyToID)
}

func TestHandleMessage_WithPhoto(t *testing.T) {
	bot := createTestBot(t)
	handler := NewHandler(bot)

	var receivedCtx MessageContext
	handler.SetOnMessage(func(ctx MessageContext) error {
		receivedCtx = ctx
		return nil
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From: &tgbotapi.User{
				ID:       12345,
				UserName: "testuser",
			},
			Chat: &tgbotapi.Chat{
				ID:   67890,
				Type: "private",
			},
			Photo: []tgbotapi.PhotoSize{
				{FileID: "photo123"},
			},
			Date: int(time.Now().Unix()),
		},
	}

	err := handler.HandleMessage(update)
	assert.NoError(t, err)
	assert.Equal(t, "photo", receivedCtx.MediaType)
	assert.Equal(t, "photo123", receivedCtx.MediaFileID)
}

func TestParseCaption(t *testing.T) {
	t.Run("with caption", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Caption: "Photo caption",
			Text:    "Text",
		}
		assert.Equal(t, "Photo caption", ParseCaption(msg))
	})

	t.Run("without caption", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Text: "Text only",
		}
		assert.Equal(t, "Text only", ParseCaption(msg))
	})

	t.Run("empty", func(t *testing.T) {
		msg := &tgbotapi.Message{}
		assert.Equal(t, "", ParseCaption(msg))
	})
}
