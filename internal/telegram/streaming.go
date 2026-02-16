package telegram

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

// StreamMode defines how messages are streamed
type StreamMode string

const (
	StreamModeOff     StreamMode = "off"     // Send complete message
	StreamModePartial StreamMode = "partial" // Update message as chunks arrive
	StreamModeBlock   StreamMode = "block"   // Send blocks as they complete
)

// Streaming handles streaming message updates
type Streaming struct {
	bot    *Bot
	logger zerolog.Logger
	mode   StreamMode

	// Active streams
	streams map[string]*Stream
	mu      sync.RWMutex

	// Rate limiting
	lastUpdate map[int64]time.Time
	updateMu   sync.Mutex
}

// Stream represents an active streaming message
type Stream struct {
	ChatID     int64
	MessageID  int
	Content    strings.Builder
	LastUpdate time.Time
	BlockCount int
	mu         sync.Mutex
}

// NewStreaming creates a new streaming handler
func NewStreaming(bot *Bot, mode StreamMode) *Streaming {
	return &Streaming{
		bot:        bot,
		logger:     bot.logger.With().Str("module", "streaming").Logger(),
		mode:       mode,
		streams:    make(map[string]*Stream),
		lastUpdate: make(map[int64]time.Time),
	}
}

// StartStream starts a new streaming message
func (s *Streaming) StartStream(chatID int64, initialText string) (*Stream, error) {
	if s.mode == StreamModeOff {
		// Just send the message normally
		msg := tgbotapi.NewMessage(chatID, initialText)
		sent, err := s.bot.api.Send(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to send message: %w", err)
		}

		stream := &Stream{
			ChatID:     chatID,
			MessageID:  sent.MessageID,
			LastUpdate: time.Now(),
		}
		stream.Content.WriteString(initialText)

		return stream, nil
	}

	// Send initial message
	msg := tgbotapi.NewMessage(chatID, initialText)
	sent, err := s.bot.api.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to send initial message: %w", err)
	}

	stream := &Stream{
		ChatID:     chatID,
		MessageID:  sent.MessageID,
		LastUpdate: time.Now(),
	}
	stream.Content.WriteString(initialText)

	// Store stream
	streamKey := fmt.Sprintf("%d:%d", chatID, sent.MessageID)
	s.mu.Lock()
	s.streams[streamKey] = stream
	s.mu.Unlock()

	s.logger.Debug().
		Int64("chat_id", chatID).
		Int("message_id", sent.MessageID).
		Str("mode", string(s.mode)).
		Msg("Stream started")

	return stream, nil
}

// AppendChunk appends a chunk to the stream
func (s *Streaming) AppendChunk(stream *Stream, chunk string) error {
	if s.mode == StreamModeOff {
		// No streaming, just accumulate
		stream.mu.Lock()
		stream.Content.WriteString(chunk)
		stream.mu.Unlock()
		return nil
	}

	stream.mu.Lock()
	stream.Content.WriteString(chunk)
	content := stream.Content.String()
	stream.mu.Unlock()

	// Check rate limit
	if !s.shouldUpdate(stream.ChatID) {
		return nil
	}

	// Update message based on mode
	switch s.mode {
	case StreamModePartial:
		return s.updateMessage(stream, content)
	case StreamModeBlock:
		// Only update on block boundaries (e.g., newlines)
		if strings.HasSuffix(chunk, "\n") || strings.HasSuffix(chunk, "\n\n") {
			return s.updateMessage(stream, content)
		}
	}

	return nil
}

// FinishStream completes the stream with final content
func (s *Streaming) FinishStream(stream *Stream, finalText string) error {
	stream.mu.Lock()
	if finalText != "" {
		stream.Content.Reset()
		stream.Content.WriteString(finalText)
	}
	content := stream.Content.String()
	stream.mu.Unlock()

	// Always send final update
	err := s.updateMessage(stream, content)

	// Remove from active streams
	streamKey := fmt.Sprintf("%d:%d", stream.ChatID, stream.MessageID)
	s.mu.Lock()
	delete(s.streams, streamKey)
	s.mu.Unlock()

	s.logger.Debug().
		Int64("chat_id", stream.ChatID).
		Int("message_id", stream.MessageID).
		Msg("Stream finished")

	return err
}

// updateMessage updates the message content
func (s *Streaming) updateMessage(stream *Stream, content string) error {
	edit := tgbotapi.NewEditMessageText(stream.ChatID, stream.MessageID, content)

	_, err := s.bot.api.Send(edit)
	if err != nil {
		// Ignore "message is not modified" errors
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		return fmt.Errorf("failed to update message: %w", err)
	}

	stream.mu.Lock()
	stream.LastUpdate = time.Now()
	stream.mu.Unlock()

	s.updateMu.Lock()
	s.lastUpdate[stream.ChatID] = time.Now()
	s.updateMu.Unlock()

	return nil
}

// shouldUpdate checks if we should update based on rate limits
func (s *Streaming) shouldUpdate(chatID int64) bool {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	lastUpdate, exists := s.lastUpdate[chatID]
	if !exists {
		return true
	}

	// Rate limit: max 1 update per second per chat
	return time.Since(lastUpdate) >= time.Second
}

// SetMode sets the streaming mode
func (s *Streaming) SetMode(mode StreamMode) {
	s.mode = mode
	s.logger.Info().Str("mode", string(mode)).Msg("Streaming mode changed")
}

// GetMode returns the current streaming mode
func (s *Streaming) GetMode() StreamMode {
	return s.mode
}

// GetActiveStreams returns the number of active streams
func (s *Streaming) GetActiveStreams() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.streams)
}

// CleanupStaleStreams removes streams that haven't been updated recently
func (s *Streaming) CleanupStaleStreams(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, stream := range s.streams {
		stream.mu.Lock()
		age := now.Sub(stream.LastUpdate)
		stream.mu.Unlock()

		if age > maxAge {
			delete(s.streams, key)
			removed++
		}
	}

	if removed > 0 {
		s.logger.Info().Int("removed", removed).Msg("Cleaned up stale streams")
	}

	return removed
}
