package logger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("create logger with console output", func(t *testing.T) {
		cfg := Config{
			Level:   "info",
			Console: true,
			Pretty:  false,
		}

		logger, err := New(cfg)
		require.NoError(t, err)
		assert.NotNil(t, logger)

		if logger != nil {
			logger.Close()
		}
	})

	t.Run("create logger with file output", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		cfg := Config{
			Level:   "debug",
			File:    logFile,
			Console: false,
		}

		logger, err := New(cfg)
		require.NoError(t, err)
		assert.NotNil(t, logger)

		// Write a log message
		logger.Info().Msg("test message")

		logger.Close()

		// Verify file was created
		_, err = os.Stat(logFile)
		assert.NoError(t, err)
	})

	t.Run("create logger with redaction", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		cfg := Config{
			Level:     "info",
			File:      logFile,
			Console:   false,
			Redaction: true,
		}

		logger, err := New(cfg)
		require.NoError(t, err)
		assert.NotNil(t, logger)
		assert.NotNil(t, logger.redactor)

		logger.Close()
	})
}

func TestLoggerMethods(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:   "debug",
		File:    logFile,
		Console: false,
	}

	logger, err := New(cfg)
	require.NoError(t, err)
	defer logger.Close()

	t.Run("debug", func(t *testing.T) {
		event := logger.Debug()
		assert.NotNil(t, event)
		event.Msg("debug message")
	})

	t.Run("info", func(t *testing.T) {
		event := logger.Info()
		assert.NotNil(t, event)
		event.Msg("info message")
	})

	t.Run("warn", func(t *testing.T) {
		event := logger.Warn()
		assert.NotNil(t, event)
		event.Msg("warn message")
	})

	t.Run("error", func(t *testing.T) {
		event := logger.Error()
		assert.NotNil(t, event)
		event.Msg("error message")
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "info", cfg.Level)
	assert.True(t, cfg.Console)
	assert.True(t, cfg.Pretty)
	assert.True(t, cfg.Redaction)
	assert.Equal(t, 100, cfg.MaxSize)
	assert.Equal(t, 7, cfg.MaxAge)
	assert.True(t, cfg.Compress)
}

func TestLoggerWith(t *testing.T) {
	cfg := Config{
		Level:   "info",
		Console: false,
	}

	logger, err := New(cfg)
	require.NoError(t, err)
	defer logger.Close()

	ctx := logger.With()
	assert.NotNil(t, ctx)

	childLogger := ctx.Str("component", "test").Logger()
	assert.NotNil(t, childLogger)
}

func TestGetZerolog(t *testing.T) {
	cfg := Config{
		Level:   "info",
		Console: false,
	}

	logger, err := New(cfg)
	require.NoError(t, err)
	defer logger.Close()

	zl := logger.GetZerolog()
	assert.Equal(t, zerolog.InfoLevel, zl.GetLevel())
}
