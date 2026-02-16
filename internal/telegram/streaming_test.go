package telegram

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewStreaming(t *testing.T) {
	bot := createTestBot(t)
	streaming := NewStreaming(bot, StreamModeOff)

	assert.NotNil(t, streaming)
	assert.Equal(t, bot, streaming.bot)
	assert.Equal(t, StreamModeOff, streaming.mode)
}

func TestStreamModes(t *testing.T) {
	bot := createTestBot(t)

	t.Run("off mode", func(t *testing.T) {
		streaming := NewStreaming(bot, StreamModeOff)
		assert.Equal(t, StreamModeOff, streaming.GetMode())
	})

	t.Run("partial mode", func(t *testing.T) {
		streaming := NewStreaming(bot, StreamModePartial)
		assert.Equal(t, StreamModePartial, streaming.GetMode())
	})

	t.Run("block mode", func(t *testing.T) {
		streaming := NewStreaming(bot, StreamModeBlock)
		assert.Equal(t, StreamModeBlock, streaming.GetMode())
	})
}

func TestSetMode(t *testing.T) {
	bot := createTestBot(t)
	streaming := NewStreaming(bot, StreamModeOff)

	streaming.SetMode(StreamModePartial)
	assert.Equal(t, StreamModePartial, streaming.GetMode())

	streaming.SetMode(StreamModeBlock)
	assert.Equal(t, StreamModeBlock, streaming.GetMode())
}

func TestGetActiveStreams(t *testing.T) {
	bot := createTestBot(t)
	streaming := NewStreaming(bot, StreamModeOff)

	assert.Equal(t, 0, streaming.GetActiveStreams())
}

func TestCleanupStaleStreams(t *testing.T) {
	bot := createTestBot(t)
	streaming := NewStreaming(bot, StreamModePartial)

	// Add a stale stream
	stream := &Stream{
		ChatID:     12345,
		MessageID:  1,
		LastUpdate: time.Now().Add(-10 * time.Minute),
	}
	streaming.streams["12345:1"] = stream

	// Cleanup streams older than 5 minutes
	removed := streaming.CleanupStaleStreams(5 * time.Minute)
	assert.Equal(t, 1, removed)
	assert.Equal(t, 0, streaming.GetActiveStreams())
}

func TestShouldUpdate_RateLimit(t *testing.T) {
	bot := createTestBot(t)
	streaming := NewStreaming(bot, StreamModePartial)

	chatID := int64(12345)

	// First update should be allowed
	assert.True(t, streaming.shouldUpdate(chatID))

	// Mark as updated
	streaming.lastUpdate[chatID] = time.Now()

	// Immediate second update should be blocked
	assert.False(t, streaming.shouldUpdate(chatID))

	// Update after 1 second should be allowed
	streaming.lastUpdate[chatID] = time.Now().Add(-2 * time.Second)
	assert.True(t, streaming.shouldUpdate(chatID))
}
