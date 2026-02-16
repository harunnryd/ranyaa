package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLifecycleManager(t *testing.T) {
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

	lm := NewLifecycleManager(daemon)
	assert.NotNil(t, lm)
	assert.Equal(t, daemon, lm.daemon)
	assert.Equal(t, filepath.Join(tmpDir, "ranya.pid"), lm.pidFile)
}

func TestLifecycleManagerStartStop(t *testing.T) {
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

	lm := NewLifecycleManager(daemon)

	// Start
	err = lm.Start()
	require.NoError(t, err)

	// Verify PID file exists
	_, err = os.Stat(lm.pidFile)
	assert.NoError(t, err)

	// Stop
	err = lm.Stop()
	require.NoError(t, err)

	// Verify PID file is removed
	_, err = os.Stat(lm.pidFile)
	assert.True(t, os.IsNotExist(err))
}

func TestLifecycleManagerGetPID(t *testing.T) {
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

	lm := NewLifecycleManager(daemon)

	// Start to create PID file
	err = lm.Start()
	require.NoError(t, err)
	defer lm.Stop()

	// Get PID
	pid, err := lm.GetPID()
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), pid)
}
