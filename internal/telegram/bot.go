package telegram

import (
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/rs/zerolog"
)

// Bot represents a Telegram bot instance
type Bot struct {
	api    *tgbotapi.BotAPI
	config *config.TelegramConfig
	logger zerolog.Logger

	// Handlers
	messageHandler MessageHandler
	commandHandler CommandHandler
	mediaHandler   MediaHandler

	// State
	running bool
	updates tgbotapi.UpdatesChannel
}

// MessageHandler handles incoming messages
type MessageHandler interface {
	HandleMessage(update tgbotapi.Update) error
}

// CommandHandler handles bot commands
type CommandHandler interface {
	HandleCommand(update tgbotapi.Update) error
}

// MediaHandler handles media messages
type MediaHandler interface {
	HandleMedia(update tgbotapi.Update) error
}

// New creates a new Telegram bot instance
func New(cfg *config.TelegramConfig, log *logger.Logger) (*Bot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("telegram config is required")
	}

	if cfg.BotToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}

	// Create bot API instance
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	bot := &Bot{
		api:    api,
		config: cfg,
		logger: log.GetZerolog().With().Str("component", "telegram").Logger(),
	}

	// Log bot info
	bot.logger.Info().
		Str("username", api.Self.UserName).
		Int64("id", api.Self.ID).
		Msg("Telegram bot authenticated")

	return bot, nil
}

// Start starts the bot and begins processing updates
func (b *Bot) Start() error {
	if b.running {
		return fmt.Errorf("bot is already running")
	}

	b.logger.Info().Msg("Starting Telegram bot")

	// Configure update settings
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := b.api.GetUpdatesChan(u)
	b.updates = updates
	b.running = true

	// Start processing updates
	go b.processUpdates()

	b.logger.Info().Msg("Telegram bot started")

	return nil
}

// Stop stops the bot
func (b *Bot) Stop() error {
	if !b.running {
		return fmt.Errorf("bot is not running")
	}

	b.logger.Info().Msg("Stopping Telegram bot")

	b.running = false
	b.api.StopReceivingUpdates()

	b.logger.Info().Msg("Telegram bot stopped")

	return nil
}

// processUpdates processes incoming updates
func (b *Bot) processUpdates() {
	for update := range b.updates {
		if !b.running {
			break
		}

		// Process update
		if err := b.handleUpdate(update); err != nil {
			b.logger.Error().
				Err(err).
				Int("update_id", update.UpdateID).
				Msg("Failed to handle update")
		}
	}
}

// handleUpdate routes an update to the appropriate handler
func (b *Bot) handleUpdate(update tgbotapi.Update) error {
	// Handle message
	if update.Message != nil {
		// Check if it's a command
		if update.Message.IsCommand() && b.commandHandler != nil {
			return b.commandHandler.HandleCommand(update)
		}

		// Check if it has media
		if b.hasMedia(update.Message) && b.mediaHandler != nil {
			return b.mediaHandler.HandleMedia(update)
		}

		// Handle as regular message
		if b.messageHandler != nil {
			return b.messageHandler.HandleMessage(update)
		}
	}

	return nil
}

// hasMedia checks if a message contains media
func (b *Bot) hasMedia(msg *tgbotapi.Message) bool {
	return msg.Photo != nil ||
		msg.Video != nil ||
		msg.Audio != nil ||
		msg.Document != nil ||
		msg.Voice != nil
}

// SendMessage sends a text message
func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)

	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	b.logger.Debug().
		Int64("chat_id", chatID).
		Msg("Message sent")

	return nil
}

// SendMessageWithReply sends a text message as a reply
func (b *Bot) SendMessageWithReply(chatID int64, text string, replyToMessageID int) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyToMessageID

	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	b.logger.Debug().
		Int64("chat_id", chatID).
		Int("reply_to", replyToMessageID).
		Msg("Reply sent")

	return nil
}

// GetBotInfo returns bot information
func (b *Bot) GetBotInfo() map[string]interface{} {
	return map[string]interface{}{
		"username":  b.api.Self.UserName,
		"id":        b.api.Self.ID,
		"firstName": b.api.Self.FirstName,
		"running":   b.running,
	}
}

// SetMessageHandler sets the message handler
func (b *Bot) SetMessageHandler(handler MessageHandler) {
	b.messageHandler = handler
}

// SetCommandHandler sets the command handler
func (b *Bot) SetCommandHandler(handler CommandHandler) {
	b.commandHandler = handler
}

// SetMediaHandler sets the media handler
func (b *Bot) SetMediaHandler(handler MediaHandler) {
	b.mediaHandler = handler
}

// GetAPI returns the underlying bot API
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

// IsRunning returns whether the bot is running
func (b *Bot) IsRunning() bool {
	return b.running
}

// ValidateToken validates a bot token by attempting to authenticate
func ValidateToken(token string) error {
	if token == "" {
		return fmt.Errorf("bot token is empty")
	}

	// Try to create bot API
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("invalid bot token: %w", err)
	}

	// Check if we can get bot info
	if api.Self.UserName == "" {
		return fmt.Errorf("failed to get bot info")
	}

	return nil
}

// WaitForReady waits for the bot to be ready
func (b *Bot) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if b.running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("bot did not become ready within timeout")
}
