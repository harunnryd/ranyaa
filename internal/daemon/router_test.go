package daemon

import (
	"context"
	"testing"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.AnthropicAPIKey = "sk-test-key"

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	router := NewRouter(daemon)
	assert.NotNil(t, router)
	assert.Equal(t, daemon, router.daemon)
}

func TestRouteMessage(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.AnthropicAPIKey = "sk-test-key"

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	router := NewRouter(daemon)

	msg := Message{
		SessionKey: "test-session",
		Source:     "telegram",
		Content:    "Hello, world!",
		Metadata:   map[string]interface{}{},
	}

	err = router.RouteMessage(context.Background(), msg)
	assert.NoError(t, err)
}
