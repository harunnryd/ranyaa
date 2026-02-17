package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/channels"
	"github.com/rs/zerolog"
)

func (d *Daemon) initializeChannelRegistry() error {
	d.channelRegistry = channels.NewRegistry(d.dispatchIngressMessage)

	baseChannels := []string{"gateway", "cron", "webhook"}
	for _, name := range baseChannels {
		if err := d.registerChannel(channels.NewDirectChannel(name)); err != nil {
			return err
		}
	}

	// Keep telegram channel addressable in routing even when bot ingress is disabled.
	if !d.config.Channels.Telegram.Enabled {
		if err := d.registerChannel(channels.NewDirectChannel("telegram")); err != nil {
			return err
		}
	}

	return nil
}

func (d *Daemon) registerChannel(ch channels.Channel) error {
	if d.channelRegistry == nil {
		return fmt.Errorf("channel registry is not initialized")
	}
	return d.channelRegistry.Register(ch)
}

func (d *Daemon) dispatchIngressMessage(ctx context.Context, msg channels.InboundMessage) (interface{}, error) {
	return d.router.RouteMessageAndWait(ctx, Message{
		SessionKey: msg.SessionKey,
		Source:     msg.Channel,
		Content:    msg.Content,
		Metadata:   msg.Metadata,
		AgentID:    msg.AgentID,
		RunConfig:  msg.RunConfig,
		CWD:        msg.CWD,
	})
}

type telegramIngressChannel struct {
	bot     *telegram.Bot
	logger  zerolog.Logger
	handler *telegram.Handler
}

func newTelegramIngressChannel(bot *telegram.Bot, logger zerolog.Logger) channels.Channel {
	return &telegramIngressChannel{
		bot:    bot,
		logger: logger.With().Str("component", "channels.telegram").Logger(),
	}
}

func (c *telegramIngressChannel) Name() string {
	return "telegram"
}

func (c *telegramIngressChannel) Start(_ context.Context, dispatch channels.DispatchFunc) error {
	if c.bot == nil {
		return fmt.Errorf("telegram bot is required")
	}
	if dispatch == nil {
		return fmt.Errorf("dispatch function is required")
	}

	handler := telegram.NewHandler(c.bot)
	handler.SetOnMessage(func(msgCtx telegram.MessageContext) error {
		content := strings.TrimSpace(msgCtx.Text)
		if content == "" {
			return nil
		}

		peerID := strconv.FormatInt(msgCtx.ChatID, 10)
		result, err := dispatch(context.Background(), channels.InboundMessage{
			Channel:    "telegram",
			SessionKey: fmt.Sprintf("telegram:%d", msgCtx.ChatID),
			Content:    content,
			Metadata: map[string]interface{}{
				"channel":    "telegram",
				"peer":       peerID,
				"peer_id":    peerID,
				"chat_id":    peerID,
				"user_id":    strconv.FormatInt(msgCtx.UserID, 10),
				"username":   msgCtx.Username,
				"is_group":   msgCtx.IsGroup,
				"is_mention": msgCtx.IsMention,
			},
		})
		if err != nil {
			c.logger.Error().Err(err).Str("session_key", fmt.Sprintf("telegram:%d", msgCtx.ChatID)).Msg("Telegram ingress dispatch failed")
			return handler.SendResponse(msgCtx, fmt.Sprintf("Request failed: %v", err))
		}

		response := channelResponseText(result)
		if response == "" {
			return nil
		}
		return handler.SendResponse(msgCtx, response)
	})

	c.handler = handler
	c.bot.SetMessageHandler(handler)
	return nil
}

func (c *telegramIngressChannel) Stop(_ context.Context) error {
	if c.bot != nil {
		c.bot.SetMessageHandler(nil)
	}
	c.handler = nil
	return nil
}

func channelResponseText(result interface{}) string {
	switch value := result.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case agent.AgentResult:
		return strings.TrimSpace(value.Response)
	case *agent.AgentResult:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(value.Response)
	case map[string]interface{}:
		if response, ok := value["response"].(string); ok {
			return strings.TrimSpace(response)
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
	}

	if stringer, ok := result.(fmt.Stringer); ok {
		return strings.TrimSpace(stringer.String())
	}

	encoded, err := json.Marshal(result)
	if err == nil {
		return string(encoded)
	}

	return strings.TrimSpace(fmt.Sprintf("%v", result))
}
