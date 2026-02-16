package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

// CommandHandler handles bot commands
type Commands struct {
	bot      *Bot
	logger   zerolog.Logger
	handlers map[string]CommandFunc
}

// CommandFunc is a function that handles a command
type CommandFunc func(CommandContext) error

// CommandContext contains command metadata
type CommandContext struct {
	ChatID    int64
	MessageID int
	UserID    int64
	Username  string
	Command   string
	Args      []string
	RawArgs   string
}

// NewCommands creates a new command handler
func NewCommands(bot *Bot) *Commands {
	return &Commands{
		bot:      bot,
		logger:   bot.logger.With().Str("module", "commands").Logger(),
		handlers: make(map[string]CommandFunc),
	}
}

// HandleCommand processes incoming commands
func (c *Commands) HandleCommand(update tgbotapi.Update) error {
	if update.Message == nil || !update.Message.IsCommand() {
		return nil
	}

	msg := update.Message
	command := msg.Command()
	args := strings.Fields(msg.CommandArguments())

	ctx := CommandContext{
		ChatID:    msg.Chat.ID,
		MessageID: msg.MessageID,
		UserID:    msg.From.ID,
		Username:  msg.From.UserName,
		Command:   command,
		Args:      args,
		RawArgs:   msg.CommandArguments(),
	}

	c.logger.Debug().
		Int64("chat_id", ctx.ChatID).
		Str("command", command).
		Strs("args", args).
		Msg("Command received")

	// Find and execute handler
	handler, exists := c.handlers[command]
	if !exists {
		return c.sendUnknownCommand(ctx)
	}

	return handler(ctx)
}

// Register registers a command handler
func (c *Commands) Register(command string, handler CommandFunc) {
	c.handlers[command] = handler
	c.logger.Info().Str("command", command).Msg("Command registered")
}

// Unregister removes a command handler
func (c *Commands) Unregister(command string) {
	delete(c.handlers, command)
	c.logger.Info().Str("command", command).Msg("Command unregistered")
}

// SetCommands sets the bot's command list in Telegram
func (c *Commands) SetCommands(commands []tgbotapi.BotCommand) error {
	cfg := tgbotapi.NewSetMyCommands(commands...)
	_, err := c.bot.api.Request(cfg)
	if err != nil {
		return fmt.Errorf("failed to set commands: %w", err)
	}

	c.logger.Info().Int("count", len(commands)).Msg("Bot commands updated")
	return nil
}

// sendUnknownCommand sends an unknown command response
func (c *Commands) sendUnknownCommand(ctx CommandContext) error {
	text := fmt.Sprintf("Unknown command: /%s", ctx.Command)
	return c.bot.SendMessageWithReply(ctx.ChatID, text, ctx.MessageID)
}

// SendResponse sends a response to a command
func (c *Commands) SendResponse(ctx CommandContext, text string) error {
	return c.bot.SendMessageWithReply(ctx.ChatID, text, ctx.MessageID)
}

// GetRegisteredCommands returns all registered commands
func (c *Commands) GetRegisteredCommands() []string {
	commands := make([]string, 0, len(c.handlers))
	for cmd := range c.handlers {
		commands = append(commands, cmd)
	}
	return commands
}
