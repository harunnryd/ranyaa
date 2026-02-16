package telegram

import (
	"os"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
)

func TestNewMedia(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

	assert.NotNil(t, media)
	assert.Equal(t, bot, media.bot)
}

func TestHandleMedia_Photo(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
				{FileID: "photo123", FileSize: 1024},
			},
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestHandleMedia_Video(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
			Video: &tgbotapi.Video{
				FileID:   "video123",
				FileSize: 2048,
			},
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestHandleMedia_Audio(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
			Audio: &tgbotapi.Audio{
				FileID:   "audio123",
				FileSize: 3072,
			},
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestHandleMedia_Document(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
			Document: &tgbotapi.Document{
				FileID:   "doc123",
				FileSize: 4096,
			},
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestHandleMedia_Voice(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
			Voice: &tgbotapi.Voice{
				FileID:   "voice123",
				FileSize: 5120,
			},
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestHandleMedia_NoMedia(t *testing.T) {
	bot := createTestBot(t)
	media := NewMedia(bot)

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
			Text: "Just text",
			Date: 1234567890,
		},
	}

	err := media.HandleMedia(update)
	assert.NoError(t, err)
}

func TestUploadPhoto(t *testing.T) {
	// Skip if no bot token
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("TELEGRAM_BOT_TOKEN not set")
	}

	// This test requires a real bot and would actually send a message
	// So we skip it in normal test runs
	t.Skip("Skipping upload test - requires real bot and chat")
}
