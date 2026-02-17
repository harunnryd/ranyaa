package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/channels"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTelegramCall struct {
	Method    string
	Text      string
	ChatID    int64
	MessageID int
}

type fakeTelegramAPI struct {
	mu            sync.Mutex
	nextMessageID int
	calls         []fakeTelegramCall
}

func (f *fakeTelegramAPI) appendCall(call fakeTelegramCall) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeTelegramAPI) snapshot() []fakeTelegramCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeTelegramCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func newTelegramTestBot(t *testing.T) (*telegram.Bot, *telegram.Commands, *fakeTelegramAPI) {
	t.Helper()

	state := &fakeTelegramAPI{nextMessageID: 100}
	token := "123456:test-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())

		path := strings.TrimPrefix(r.URL.Path, "/bot"+token+"/")
		switch path {
		case "getMe":
			writeTelegramResponse(t, w, map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"id":         999001,
					"is_bot":     true,
					"first_name": "RanyaTest",
					"username":   "ranya_test_bot",
				},
			})
		case "sendMessage":
			chatID, _ := strconv.ParseInt(r.Form.Get("chat_id"), 10, 64)
			text := r.Form.Get("text")
			state.mu.Lock()
			state.nextMessageID++
			messageID := state.nextMessageID
			state.mu.Unlock()
			state.appendCall(fakeTelegramCall{
				Method:    "sendMessage",
				Text:      text,
				ChatID:    chatID,
				MessageID: messageID,
			})
			writeTelegramResponse(t, w, map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"message_id": messageID,
					"date":       time.Now().Unix(),
					"text":       text,
					"chat": map[string]interface{}{
						"id":   chatID,
						"type": "private",
					},
				},
			})
		case "editMessageText":
			chatID, _ := strconv.ParseInt(r.Form.Get("chat_id"), 10, 64)
			messageID, _ := strconv.Atoi(r.Form.Get("message_id"))
			text := r.Form.Get("text")
			state.appendCall(fakeTelegramCall{
				Method:    "editMessageText",
				Text:      text,
				ChatID:    chatID,
				MessageID: messageID,
			})
			writeTelegramResponse(t, w, map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"message_id": messageID,
					"date":       time.Now().Unix(),
					"text":       text,
					"chat": map[string]interface{}{
						"id":   chatID,
						"type": "private",
					},
				},
			})
		default:
			writeTelegramResponse(t, w, map[string]interface{}{
				"ok":          true,
				"description": "handled by fake test server",
				"result":      map[string]interface{}{},
			})
		}
	}))
	t.Cleanup(server.Close)

	log, err := logger.New(logger.Config{Level: "debug", Console: false})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})

	cfg := &config.TelegramConfig{
		BotToken:    token,
		APIEndpoint: server.URL + "/bot%s/%s",
	}
	bot, err := telegram.New(cfg, log)
	require.NoError(t, err)

	commands := telegram.NewCommands(bot)
	bot.SetCommandHandler(commands)

	return bot, commands, state
}

func writeTelegramResponse(t *testing.T, w http.ResponseWriter, payload map[string]interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(payload))
}

func TestTelegramIngressMessageDeduplicationIntegration(t *testing.T) {
	bot, _, _ := newTelegramTestBot(t)
	channel := newTelegramIngressChannel(
		bot,
		nil,
		config.TelegramConfig{
			DMPolicy:         "open",
			DedupeTTLSeconds: 300,
			StreamMode:       "off",
		},
		nil,
		zerolog.Nop(),
	).(*telegramIngressChannel)

	var dispatchCount atomic.Int32
	err := channel.Start(context.Background(), func(_ context.Context, _ channels.InboundMessage) (interface{}, error) {
		dispatchCount.Add(1)
		return nil, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = channel.Stop(context.Background())
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 42,
			From: &tgbotapi.User{
				ID:       1001,
				UserName: "alice",
			},
			Chat: &tgbotapi.Chat{
				ID:   555001,
				Type: "private",
			},
			Text: "run this once",
			Date: int(time.Now().Unix()),
		},
	}

	require.NoError(t, channel.handler.HandleMessage(update))
	require.NoError(t, channel.handler.HandleMessage(update))

	assert.Equal(t, int32(1), dispatchCount.Load())
}

func TestTelegramIngressStreamingIntegrationEmitsMultipleUpdatesBeforeFinal(t *testing.T) {
	bot, _, fakeAPI := newTelegramTestBot(t)

	eventCh := make(chan agent.RuntimeEvent, 16)
	var closeOnce sync.Once
	subscribe := func(_ string) (<-chan agent.RuntimeEvent, func()) {
		return eventCh, func() {
			closeOnce.Do(func() {
				close(eventCh)
			})
		}
	}

	finalResponse := "Part one. Part two. Part three. Final."
	channel := newTelegramIngressChannel(
		bot,
		nil,
		config.TelegramConfig{
			DMPolicy:          "open",
			DedupeTTLSeconds:  300,
			StreamMode:        "partial",
			StreamMinInterval: 10,
			StreamMinChars:    8,
		},
		subscribe,
		zerolog.Nop(),
	).(*telegramIngressChannel)

	err := channel.Start(context.Background(), func(_ context.Context, _ channels.InboundMessage) (interface{}, error) {
		eventCh <- agent.RuntimeEvent{
			Event:   "assistant",
			Stream:  agent.RuntimeStreamAssistant,
			Phase:   "output",
			Content: "Part one. ",
		}
		time.Sleep(15 * time.Millisecond)
		eventCh <- agent.RuntimeEvent{
			Event:   "assistant",
			Stream:  agent.RuntimeStreamAssistant,
			Phase:   "output",
			Content: "Part two. ",
		}
		time.Sleep(15 * time.Millisecond)
		eventCh <- agent.RuntimeEvent{
			Event:   "assistant",
			Stream:  agent.RuntimeStreamAssistant,
			Phase:   "output",
			Content: "Part three.",
		}
		closeOnce.Do(func() {
			close(eventCh)
		})
		return agent.AgentResult{Response: finalResponse}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = channel.Stop(context.Background())
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 77,
			From: &tgbotapi.User{
				ID:       2001,
				UserName: "streamer",
			},
			Chat: &tgbotapi.Chat{
				ID:   555002,
				Type: "private",
			},
			Text: "stream please",
			Date: int(time.Now().Unix()),
		},
	}

	require.NoError(t, channel.handler.HandleMessage(update))

	calls := fakeAPI.snapshot()
	editTexts := make([]string, 0)
	for _, call := range calls {
		if call.Method == "editMessageText" {
			editTexts = append(editTexts, call.Text)
		}
	}

	require.GreaterOrEqual(t, len(editTexts), 2, "expected partial and final edit updates")
	assert.Equal(t, finalResponse, editTexts[len(editTexts)-1], "last edit must be final response")
	foundPartial := false
	for _, text := range editTexts[:len(editTexts)-1] {
		if text != finalResponse {
			foundPartial = true
			break
		}
	}
	assert.True(t, foundPartial, "expected at least one partial update before final")
}

func TestTelegramIngressPairingWorkflowIntegration(t *testing.T) {
	bot, commands, fakeAPI := newTelegramTestBot(t)
	channel := newTelegramIngressChannel(
		bot,
		commands,
		config.TelegramConfig{
			DMPolicy:           "pairing",
			DedupeTTLSeconds:   300,
			StreamMode:         "off",
			PairingPrompt:      "Pair first with /pair",
			PairingSuccessText: "Paired successfully",
		},
		nil,
		zerolog.Nop(),
	).(*telegramIngressChannel)

	var dispatchCount atomic.Int32
	err := channel.Start(context.Background(), func(_ context.Context, _ channels.InboundMessage) (interface{}, error) {
		dispatchCount.Add(1)
		return agent.AgentResult{Response: "processed"}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = channel.Stop(context.Background())
	})

	privateChat := &tgbotapi.Chat{ID: 777001, Type: "private"}
	fromUser := &tgbotapi.User{ID: 3001, UserName: "newuser"}

	initialMessage := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 101,
			From:      fromUser,
			Chat:      privateChat,
			Text:      "hello",
			Date:      int(time.Now().Unix()),
		},
	}
	require.NoError(t, channel.handler.HandleMessage(initialMessage))
	assert.Equal(t, int32(0), dispatchCount.Load(), "message should be denied until paired")

	pairCommand := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 102,
			From:      fromUser,
			Chat:      privateChat,
			Text:      "/pair",
			Date:      int(time.Now().Unix()),
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}
	require.NoError(t, commands.HandleCommand(pairCommand))

	allowedMessage := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 103,
			From:      fromUser,
			Chat:      privateChat,
			Text:      "now run",
			Date:      int(time.Now().Unix()),
		},
	}
	require.NoError(t, channel.handler.HandleMessage(allowedMessage))
	assert.Equal(t, int32(1), dispatchCount.Load(), "paired user should be dispatched")

	calls := fakeAPI.snapshot()
	var texts []string
	for _, call := range calls {
		if call.Method == "sendMessage" {
			texts = append(texts, call.Text)
		}
	}
	assert.Contains(t, texts, "Pair first with /pair")
	assert.Contains(t, texts, "Paired successfully")
	assert.Contains(t, texts, "processed")
}
