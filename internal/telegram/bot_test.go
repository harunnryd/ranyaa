package telegram

import (
	"os"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	log, err := logger.New(logger.Config{
		Level:   "info",
		Console: true,
	})
	require.NoError(t, err)

	t.Run("valid config", func(t *testing.T) {
		// Skip if no bot token
		token := os.Getenv("TELEGRAM_BOT_TOKEN")
		if token == "" {
			t.Skip("TELEGRAM_BOT_TOKEN not set")
		}

		cfg := &config.TelegramConfig{
			BotToken: token,
		}

		bot, err := New(cfg, log)
		require.NoError(t, err)
		assert.NotNil(t, bot)
		assert.NotNil(t, bot.api)
		assert.Equal(t, cfg, bot.config)
	})

	t.Run("nil config", func(t *testing.T) {
		bot, err := New(nil, log)
		assert.Error(t, err)
		assert.Nil(t, bot)
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("empty bot token", func(t *testing.T) {
		cfg := &config.TelegramConfig{
			BotToken: "",
		}

		bot, err := New(cfg, log)
		assert.Error(t, err)
		assert.Nil(t, bot)
		assert.Contains(t, err.Error(), "bot token is required")
	})

	t.Run("invalid bot token", func(t *testing.T) {
		cfg := &config.TelegramConfig{
			BotToken: "invalid-token",
		}

		bot, err := New(cfg, log)
		assert.Error(t, err)
		assert.Nil(t, bot)
	})
}

func TestBotStartStop(t *testing.T) {
	// Skip if no bot token
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("TELEGRAM_BOT_TOKEN not set")
	}

	log, err := logger.New(logger.Config{
		Level:   "info",
		Console: true,
	})
	require.NoError(t, err)

	cfg := &config.TelegramConfig{
		BotToken: token,
	}

	bot, err := New(cfg, log)
	require.NoError(t, err)

	// Test start
	err = bot.Start()
	assert.NoError(t, err)
	assert.True(t, bot.IsRunning())

	// Wait for bot to be ready
	err = bot.WaitForReady(5 * time.Second)
	assert.NoError(t, err)

	// Test start again (should fail)
	err = bot.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stop
	err = bot.Stop()
	assert.NoError(t, err)
	assert.False(t, bot.IsRunning())

	// Test stop again (should fail)
	err = bot.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestGetBotInfo(t *testing.T) {
	// Skip if no bot token
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("TELEGRAM_BOT_TOKEN not set")
	}

	log, err := logger.New(logger.Config{
		Level:   "info",
		Console: true,
	})
	require.NoError(t, err)

	cfg := &config.TelegramConfig{
		BotToken: token,
	}

	bot, err := New(cfg, log)
	require.NoError(t, err)

	info := bot.GetBotInfo()
	assert.NotNil(t, info)
	assert.NotEmpty(t, info["username"])
	assert.NotZero(t, info["id"])
	assert.False(t, info["running"].(bool))
}

func TestValidateToken(t *testing.T) {
	t.Run("empty token", func(t *testing.T) {
		err := ValidateToken("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("invalid token", func(t *testing.T) {
		err := ValidateToken("invalid-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("valid token", func(t *testing.T) {
		token := os.Getenv("TELEGRAM_BOT_TOKEN")
		if token == "" {
			t.Skip("TELEGRAM_BOT_TOKEN not set")
		}

		err := ValidateToken(token)
		assert.NoError(t, err)
	})
}

func TestHasMedia(t *testing.T) {
	log, err := logger.New(logger.Config{
		Level:   "info",
		Console: true,
	})
	require.NoError(t, err)

	// Create a bot with dummy config (won't connect)
	bot := &Bot{
		logger: log.GetZerolog().With().Str("component", "telegram").Logger(),
	}

	t.Run("no media", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Text: "Hello",
		}
		assert.False(t, bot.hasMedia(msg))
	})

	t.Run("has photo", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Photo: []tgbotapi.PhotoSize{{FileID: "test"}},
		}
		assert.True(t, bot.hasMedia(msg))
	})

	t.Run("has video", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Video: &tgbotapi.Video{FileID: "test"},
		}
		assert.True(t, bot.hasMedia(msg))
	})

	t.Run("has audio", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Audio: &tgbotapi.Audio{FileID: "test"},
		}
		assert.True(t, bot.hasMedia(msg))
	})

	t.Run("has document", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Document: &tgbotapi.Document{FileID: "test"},
		}
		assert.True(t, bot.hasMedia(msg))
	})

	t.Run("has voice", func(t *testing.T) {
		msg := &tgbotapi.Message{
			Voice: &tgbotapi.Voice{FileID: "test"},
		}
		assert.True(t, bot.hasMedia(msg))
	})
}
