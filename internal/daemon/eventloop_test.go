package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEventLoop(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	eventLoop := NewEventLoop(daemon)
	assert.NotNil(t, eventLoop)
	assert.Equal(t, daemon, eventLoop.daemon)
}

func TestEventLoopRun(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	eventLoop := NewEventLoop(daemon)

	// Run event loop with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		eventLoop.Run(ctx)
		close(done)
	}()

	// Wait for event loop to finish
	select {
	case <-done:
		// Event loop stopped as expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Event loop did not stop in time")
	}
}

func TestEventLoopHandleShutdown(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	eventLoop := NewEventLoop(daemon)

	// Should not panic
	eventLoop.HandleShutdown()
}
