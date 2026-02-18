package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/channels"
	"github.com/harun/ranya/pkg/pairing"
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
	bot             *telegram.Bot
	logger          zerolog.Logger
	handler         *telegram.Handler
	commands        *telegram.Commands
	clearSession    func(sessionKey string) error
	dedupe          *messageDedupeCache
	pairing         *pairing.Manager
	dmPolicy        string
	streamMode      string
	streamMinChars  int
	streamMinPeriod time.Duration
	pairingPrompt   string
	pairingSuccess  string
	resetSuccess    string
	subscribeEvents runtimeEventSubscriber
	sessionMainKey  string
	dmScope         string
}

type runtimeEventSubscriber func(sessionKey string) (<-chan agent.RuntimeEvent, func())

func newTelegramIngressChannel(
	bot *telegram.Bot,
	commands *telegram.Commands,
	cfg config.TelegramConfig,
	sessionCfg config.SessionConfig,
	pairingManager *pairing.Manager,
	subscribe runtimeEventSubscriber,
	clearSession func(sessionKey string) error,
	logger zerolog.Logger,
) channels.Channel {
	dedupeTTL := time.Duration(cfg.DedupeTTLSeconds) * time.Second
	if dedupeTTL <= 0 {
		dedupeTTL = 5 * time.Minute
	}

	streamMode := strings.ToLower(strings.TrimSpace(cfg.StreamMode))
	if streamMode == "" {
		streamMode = "partial"
	}

	minPeriod := time.Duration(cfg.StreamMinInterval) * time.Millisecond
	if minPeriod <= 0 {
		minPeriod = 2 * time.Second
	}
	minChars := cfg.StreamMinChars
	if minChars <= 0 {
		minChars = 200
	}

	policy := strings.ToLower(strings.TrimSpace(cfg.DMPolicy))
	if policy == "" {
		policy = "pairing"
	}

	pairingPrompt := strings.TrimSpace(cfg.PairingPrompt)
	if pairingPrompt == "" {
		pairingPrompt = "⚠️ Please pair this device first. Send /pair to begin."
	}

	pairingSuccess := strings.TrimSpace(cfg.PairingSuccessText)
	if pairingSuccess == "" {
		pairingSuccess = "✅ Device paired. You can now send messages."
	}

	if pairingManager == nil {
		pairingManager = newInMemoryPairingManager(cfg.Allowlist)
	}

	return &telegramIngressChannel{
		bot:             bot,
		logger:          logger.With().Str("component", "channels.telegram").Logger(),
		commands:        commands,
		clearSession:    clearSession,
		dedupe:          newMessageDedupeCache(dedupeTTL),
		pairing:         pairingManager,
		dmPolicy:        policy,
		streamMode:      streamMode,
		streamMinChars:  minChars,
		streamMinPeriod: minPeriod,
		pairingPrompt:   pairingPrompt,
		pairingSuccess:  pairingSuccess,
		resetSuccess:    "✨ New conversation started.",
		subscribeEvents: subscribe,
		sessionMainKey:  strings.TrimSpace(sessionCfg.MainKey),
		dmScope:         strings.ToLower(strings.TrimSpace(sessionCfg.DmScope)),
	}
}

func (c *telegramIngressChannel) Name() string {
	return "telegram"
}

func (c *telegramIngressChannel) Start(ctx context.Context, dispatch channels.DispatchFunc) error {
	if c.bot == nil {
		return fmt.Errorf("telegram bot is required")
	}
	if dispatch == nil {
		return fmt.Errorf("dispatch function is required")
	}

	if c.dedupe != nil {
		c.dedupe.Start()
	}
	if c.commands != nil {
		c.commands.Register("pair", c.handlePairCommand)
		c.commands.Register("new", c.handleResetCommand)
		c.commands.Register("reset", c.handleResetCommand)
	}

	handler := telegram.NewHandler(c.bot)
	handler.SetOnMessage(func(msgCtx telegram.MessageContext) error {
		content := strings.TrimSpace(msgCtx.Text)
		if content == "" {
			return nil
		}
		command := firstTelegramToken(content)

		peerID := strconv.FormatInt(msgCtx.ChatID, 10)
		if strings.EqualFold(command, "/pair") {
			return c.handlePairMessage(handler, msgCtx)
		}
		if !c.isAllowedPeer(peerID) {
			if c.dmPolicy == "pairing" {
				prompt, err := c.pairingPromptForPeer(peerID)
				if err != nil {
					return handler.SendResponse(msgCtx, fmt.Sprintf("Failed to create pairing request: %v", err))
				}
				return handler.SendResponse(msgCtx, prompt)
			}
			denied := c.deniedAccessMessage()
			if denied == "" {
				return nil
			}
			return handler.SendResponse(msgCtx, denied)
		}
		if isResetTrigger(command) {
			return c.handleResetMessage(handler, msgCtx)
		}

		msgKey := fmt.Sprintf("telegram:%d:%d", msgCtx.ChatID, msgCtx.MessageID)
		if c.dedupe != nil {
			if c.dedupe.IsDuplicate(msgKey) {
				c.logger.Debug().
					Str("message_key", msgKey).
					Msg("Skipping duplicate Telegram ingress message")
				return nil
			}
			c.dedupe.Mark(msgKey)
		}

		sessionKey := c.resolveSessionKey(msgCtx)
		stream := c.startStream(msgCtx.ChatID, sessionKey)

		// Generate request ID for idempotency
		requestID := fmt.Sprintf("telegram:%d:%d:%d", msgCtx.ChatID, msgCtx.MessageID, time.Now().Unix())

		// Create context with tracing and lifecycle
		taskCtx := tracing.WithRequestID(ctx, requestID)
		taskCtx = tracing.WithSessionKey(taskCtx, sessionKey)
		taskCtx = tracing.NewRequestContext(taskCtx)

		result, err := dispatch(taskCtx, channels.InboundMessage{
			Channel:    "telegram",
			SessionKey: sessionKey,
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
				"request_id": requestID,
			},
		})
		if err != nil {
			c.logger.Error().Err(err).Str("session_key", sessionKey).Msg("Telegram ingress dispatch failed")
			failure := fmt.Sprintf("Request failed: %v", err)
			if stream != nil {
				c.finishStream(stream, failure)
				return nil
			}
			return handler.SendResponse(msgCtx, failure)
		}

		response := channelResponseText(result)
		if stream != nil {
			c.finishStream(stream, response)
			return nil
		}
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
	if c.dedupe != nil {
		c.dedupe.Stop()
	}
	if c.bot != nil {
		c.bot.SetMessageHandler(nil)
	}
	c.handler = nil
	return nil
}

func (c *telegramIngressChannel) handlePairCommand(cmdCtx telegram.CommandContext) error {
	switch c.dmPolicy {
	case "disabled":
		return c.commands.SendResponse(cmdCtx, "⚠️ Telegram access is disabled.")
	case "allowlist":
		peerID := strconv.FormatInt(cmdCtx.ChatID, 10)
		if c.isAllowlistedPeer(peerID) {
			return c.commands.SendResponse(cmdCtx, "✅ This chat is already allowlisted.")
		}
		return c.commands.SendResponse(cmdCtx, "⚠️ This chat is not allowlisted.")
	case "open":
		return c.commands.SendResponse(cmdCtx, "✅ Pairing is not required.")
	default:
		peerID := strconv.FormatInt(cmdCtx.ChatID, 10)
		prompt, err := c.pairingPromptForPeer(peerID)
		if err != nil {
			return c.commands.SendResponse(cmdCtx, fmt.Sprintf("Failed to create pairing request: %v", err))
		}
		return c.commands.SendResponse(cmdCtx, prompt)
	}
}

func (c *telegramIngressChannel) handleResetCommand(cmdCtx telegram.CommandContext) error {
	peerID := strconv.FormatInt(cmdCtx.ChatID, 10)
	if !c.isAllowedPeer(peerID) {
		denied := c.deniedAccessMessage()
		if denied == "" {
			return nil
		}
		return c.commands.SendResponse(cmdCtx, denied)
	}

	sessionKey := c.resolveCommandSessionKey(cmdCtx.ChatID, cmdCtx.UserID, false)
	if err := c.resetSession(sessionKey); err != nil {
		return c.commands.SendResponse(cmdCtx, fmt.Sprintf("Failed to reset conversation: %v", err))
	}
	return c.commands.SendResponse(cmdCtx, c.resetSuccess)
}

func (c *telegramIngressChannel) handlePairMessage(handler *telegram.Handler, msgCtx telegram.MessageContext) error {
	switch c.dmPolicy {
	case "disabled":
		return handler.SendResponse(msgCtx, "⚠️ Telegram access is disabled.")
	case "allowlist":
		peerID := strconv.FormatInt(msgCtx.ChatID, 10)
		if c.isAllowlistedPeer(peerID) {
			return handler.SendResponse(msgCtx, "✅ This chat is already allowlisted.")
		}
		return handler.SendResponse(msgCtx, "⚠️ This chat is not allowlisted.")
	case "open":
		return handler.SendResponse(msgCtx, "✅ Pairing is not required.")
	default:
		peerID := strconv.FormatInt(msgCtx.ChatID, 10)
		prompt, err := c.pairingPromptForPeer(peerID)
		if err != nil {
			return handler.SendResponse(msgCtx, fmt.Sprintf("Failed to create pairing request: %v", err))
		}
		return handler.SendResponse(msgCtx, prompt)
	}
}

func (c *telegramIngressChannel) handleResetMessage(handler *telegram.Handler, msgCtx telegram.MessageContext) error {
	sessionKey := c.resolveCommandSessionKey(msgCtx.ChatID, msgCtx.UserID, msgCtx.IsGroup)
	if err := c.resetSession(sessionKey); err != nil {
		return handler.SendResponse(msgCtx, fmt.Sprintf("Failed to reset conversation: %v", err))
	}
	return handler.SendResponse(msgCtx, c.resetSuccess)
}

func (c *telegramIngressChannel) resetSession(sessionKey string) error {
	if c.clearSession == nil {
		return nil
	}
	return c.clearSession(sessionKey)
}

func (c *telegramIngressChannel) deniedAccessMessage() string {
	switch c.dmPolicy {
	case "disabled":
		return "⚠️ Telegram access is disabled."
	case "allowlist":
		return "⚠️ This chat is not allowed."
	case "open":
		return ""
	default:
		return ""
	}
}

func firstTelegramToken(content string) string {
	fields := strings.Fields(strings.TrimSpace(content))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}

func isResetTrigger(token string) bool {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "/new", "/reset":
		return true
	default:
		return false
	}
}

func (c *telegramIngressChannel) resolveSessionKey(msgCtx telegram.MessageContext) string {
	return c.resolveCommandSessionKey(msgCtx.ChatID, msgCtx.UserID, msgCtx.IsGroup)
}

func (c *telegramIngressChannel) resolveCommandSessionKey(chatID int64, userID int64, isGroup bool) string {
	if isGroup {
		return fmt.Sprintf("telegram:group:%d", chatID)
	}

	mainKey := c.sessionMainKey
	if mainKey == "" {
		mainKey = "main"
	}
	switch c.dmScope {
	case "per-peer":
		return fmt.Sprintf("telegram:dm:%d", userID)
	case "per-channel-peer":
		return fmt.Sprintf("telegram:%d:dm:%d", chatID, userID)
	case "main":
		fallthrough
	default:
		return mainKey
	}
}

func (c *telegramIngressChannel) isAllowedPeer(peerID string) bool {
	switch c.dmPolicy {
	case "open":
		return true
	case "allowlist":
		return c.isAllowlistedPeer(peerID)
	case "disabled":
		return false
	default:
		if c.pairing == nil {
			return false
		}
		return c.pairing.IsAllowed(peerID)
	}
}

func (c *telegramIngressChannel) isAllowlistedPeer(peerID string) bool {
	if c.pairing == nil {
		return false
	}
	return c.pairing.IsAllowed(peerID)
}

func (c *telegramIngressChannel) pairingPromptForPeer(peerID string) (string, error) {
	if c.pairing == nil {
		return c.pairingPrompt, nil
	}

	req, _, err := c.pairing.EnsurePending(peerID)
	if err != nil {
		if errors.Is(err, pairing.ErrPendingLimitReached) {
			return "⚠️ Pairing queue is full. Try again later.", nil
		}
		if errors.Is(err, pairing.ErrAlreadyAllowlisted) {
			return "✅ This chat is already paired.", nil
		}
		return "", err
	}

	return formatPairingPrompt(c.pairingPrompt, req.Code), nil
}

func formatPairingPrompt(base string, code string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return fmt.Sprintf("Pairing code: %s", code)
	}
	if strings.Contains(base, "{{code}}") {
		return strings.ReplaceAll(base, "{{code}}", code)
	}
	return fmt.Sprintf("%s\nPairing code: %s", base, code)
}

func newInMemoryPairingManager(allowlist []int64) *pairing.Manager {
	bootstrap := make([]string, 0, len(allowlist))
	for _, id := range allowlist {
		bootstrap = append(bootstrap, strconv.FormatInt(id, 10))
	}
	manager, err := pairing.NewManager(pairing.ManagerOptions{
		Channel:            "telegram",
		BootstrapAllowlist: bootstrap,
	})
	if err != nil {
		return nil
	}
	return manager
}

type telegramRuntimeStream struct {
	chatID      int64
	messageID   int
	mode        string
	minChars    int
	minInterval time.Duration

	events <-chan agent.RuntimeEvent
	cancel func()

	sentChars int
	buffer    strings.Builder

	stopOnce sync.Once
	stopCh   chan struct{}
	done     chan struct{}
}

func (s *telegramRuntimeStream) stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.stopCh)
	})
}

func (c *telegramIngressChannel) startStream(chatID int64, sessionKey string) *telegramRuntimeStream {
	if c.streamMode == "off" || c.subscribeEvents == nil {
		return nil
	}

	initial, err := c.bot.SendMessageRaw(chatID, "…")
	if err != nil {
		c.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("Failed to initialize Telegram stream message")
		return nil
	}

	events, cancel := c.subscribeEvents(sessionKey)
	stream := &telegramRuntimeStream{
		chatID:      chatID,
		messageID:   initial.MessageID,
		mode:        c.streamMode,
		minChars:    c.streamMinChars,
		minInterval: c.streamMinPeriod,
		events:      events,
		cancel:      cancel,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
	go c.streamToTelegram(stream)
	return stream
}

func (c *telegramIngressChannel) finishStream(stream *telegramRuntimeStream, finalText string) {
	if stream == nil {
		return
	}
	stream.stop()
	<-stream.done

	finalText = strings.TrimSpace(finalText)
	if finalText == "" {
		finalText = strings.TrimSpace(stream.buffer.String())
	}
	if finalText == "" {
		return
	}
	if err := c.bot.EditMessage(stream.chatID, stream.messageID, finalText); err != nil && !strings.Contains(err.Error(), "message is not modified") {
		c.logger.Warn().Err(err).Int64("chat_id", stream.chatID).Int("message_id", stream.messageID).Msg("Failed to send final Telegram stream update")
	}
}

func (c *telegramIngressChannel) streamToTelegram(stream *telegramRuntimeStream) {
	ticker := time.NewTicker(stream.minInterval)
	defer ticker.Stop()
	defer close(stream.done)

	for {
		select {
		case <-stream.stopCh:
			c.flushStream(stream, true, true)
			return
		case <-ticker.C:
			if stream.buffer.Len()-stream.sentChars > 0 {
				c.flushStream(stream, false, true)
			}
		case evt, ok := <-stream.events:
			if !ok {
				c.flushStream(stream, true, true)
				return
			}
			if evt.Stream != agent.RuntimeStreamAssistant || evt.Phase != "output" {
				continue
			}
			if strings.TrimSpace(evt.Content) == "" {
				continue
			}
			stream.buffer.WriteString(evt.Content)
			if stream.mode == "block" {
				c.flushStream(stream, false, false)
				continue
			}
			if stream.buffer.Len()-stream.sentChars >= stream.minChars {
				c.flushStream(stream, false, false)
			}
		}
	}
}

func (c *telegramIngressChannel) flushStream(stream *telegramRuntimeStream, force bool, fromTicker bool) {
	if stream.mode == "block" {
		c.flushStreamBlock(stream, force, fromTicker)
		return
	}
	c.flushStreamPartial(stream)
}

func (c *telegramIngressChannel) flushStreamPartial(stream *telegramRuntimeStream) {
	content := strings.TrimSpace(stream.buffer.String())
	if content == "" {
		return
	}
	if len(content) <= stream.sentChars {
		return
	}
	if err := c.bot.EditMessage(stream.chatID, stream.messageID, content); err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return
		}
		c.logger.Warn().Err(err).Int64("chat_id", stream.chatID).Int("message_id", stream.messageID).Msg("Failed to update Telegram stream message")
		return
	}
	stream.sentChars = len(content)
}

func (c *telegramIngressChannel) flushStreamBlock(stream *telegramRuntimeStream, force bool, fromTicker bool) {
	raw := stream.buffer.String()
	if strings.TrimSpace(raw) == "" {
		return
	}
	if len(raw) <= stream.sentChars {
		return
	}

	cutoff := len(raw)
	if !force {
		segment := raw[stream.sentChars:]
		boundary := findBlockBoundaryOffset(segment)
		if boundary <= 0 {
			if !fromTicker {
				return
			}
			if len(segment) < stream.minChars {
				return
			}
			cutoff = len(raw)
		} else {
			if !fromTicker && boundary < stream.minChars {
				return
			}
			cutoff = stream.sentChars + boundary
		}
	}

	content := strings.TrimSpace(raw[:cutoff])
	if content == "" {
		stream.sentChars = cutoff
		return
	}

	if err := c.bot.EditMessage(stream.chatID, stream.messageID, content); err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			stream.sentChars = cutoff
			return
		}
		c.logger.Warn().Err(err).Int64("chat_id", stream.chatID).Int("message_id", stream.messageID).Msg("Failed to update Telegram stream message")
		return
	}

	stream.sentChars = cutoff
}

func findBlockBoundaryOffset(text string) int {
	if text == "" {
		return 0
	}

	delimiters := []string{"\n\n", "\n", ". ", "! ", "? "}
	best := 0
	for _, delim := range delimiters {
		idx := strings.LastIndex(text, delim)
		if idx < 0 {
			continue
		}
		end := idx + len(delim)
		if end > best {
			best = end
		}
	}
	return best
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
