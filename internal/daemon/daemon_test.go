package daemon

import (
	"testing"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDaemon creates a daemon for testing with Telegram disabled
func createTestDaemon(t *testing.T) (*Daemon, *logger.Logger) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.AnthropicAPIKey = "sk-test-key"
	cfg.Channels.Telegram.Enabled = false // Disable Telegram for tests

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	return daemon, log
}

func TestNew(t *testing.T) {
	daemon, log := createTestDaemon(t)
	defer log.Close()

	assert.NotNil(t, daemon)
	assert.NotNil(t, daemon.queue)
	assert.NotNil(t, daemon.sessionMgr)
	assert.NotNil(t, daemon.eventLoop)
	assert.NotNil(t, daemon.router)
	assert.NotNil(t, daemon.lifecycle)
}

func TestDaemonStartStop(t *testing.T) {
	daemon, log := createTestDaemon(t)
	defer log.Close()

	// Start daemon
	err := daemon.Start()
	require.NoError(t, err)

	// Check status
	status := daemon.Status()
	assert.True(t, status.Running)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop daemon
	err = daemon.Stop()
	require.NoError(t, err)

	// Check status
	status = daemon.Status()
	assert.False(t, status.Running)
}

func TestDaemonStatus(t *testing.T) {
	daemon, log := createTestDaemon(t)
	defer log.Close()

	// Status before start
	status := daemon.Status()
	assert.False(t, status.Running)
	assert.Equal(t, time.Duration(0), status.Uptime)

	// Start daemon
	err := daemon.Start()
	require.NoError(t, err)
	defer daemon.Stop()

	// Status after start
	time.Sleep(100 * time.Millisecond)
	status = daemon.Status()
	assert.True(t, status.Running)
	assert.Greater(t, status.Uptime, time.Duration(0))
}

func TestDaemonGetters(t *testing.T) {
	daemon, log := createTestDaemon(t)
	defer log.Close()

	assert.NotNil(t, daemon.GetConfig())
	assert.NotNil(t, daemon.GetLogger())
	assert.NotNil(t, daemon.GetQueue())
	assert.NotNil(t, daemon.GetSessionManager())
	assert.NotNil(t, daemon.GetRouter())
}
