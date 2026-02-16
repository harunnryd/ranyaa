package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
)

func TestNewCommands(t *testing.T) {
	bot := createTestBot(t)
	commands := NewCommands(bot)

	assert.NotNil(t, commands)
	assert.Equal(t, bot, commands.bot)
	assert.NotNil(t, commands.handlers)
}

func TestRegisterCommand(t *testing.T) {
	bot := createTestBot(t)
	commands := NewCommands(bot)

	called := false
	handler := func(ctx CommandContext) error {
		called = true
		return nil
	}

	commands.Register("test", handler)
	assert.Len(t, commands.handlers, 1)

	// Trigger the command
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
			Text: "/test",
			Date: 1234567890,
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}

	err := commands.HandleCommand(update)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestHandleCommand_WithArgs(t *testing.T) {
	bot := createTestBot(t)
	commands := NewCommands(bot)

	var receivedCtx CommandContext
	handler := func(ctx CommandContext) error {
		receivedCtx = ctx
		return nil
	}

	commands.Register("echo", handler)

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
			Text: "/echo hello world",
			Date: 1234567890,
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}

	err := commands.HandleCommand(update)
	assert.NoError(t, err)
	assert.Equal(t, "echo", receivedCtx.Command)
	assert.Equal(t, []string{"hello", "world"}, receivedCtx.Args)
	assert.Equal(t, "hello world", receivedCtx.RawArgs)
}

func TestUnregisterCommand(t *testing.T) {
	bot := createTestBot(t)
	commands := NewCommands(bot)

	handler := func(ctx CommandContext) error {
		return nil
	}

	commands.Register("test", handler)
	assert.Len(t, commands.handlers, 1)

	commands.Unregister("test")
	assert.Len(t, commands.handlers, 0)
}

func TestGetRegisteredCommands(t *testing.T) {
	bot := createTestBot(t)
	commands := NewCommands(bot)

	commands.Register("start", func(ctx CommandContext) error { return nil })
	commands.Register("help", func(ctx CommandContext) error { return nil })

	registered := commands.GetRegisteredCommands()
	assert.Len(t, registered, 2)
	assert.Contains(t, registered, "start")
	assert.Contains(t, registered, "help")
}
