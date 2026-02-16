package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCommand(t *testing.T) {
	t.Run("command exists", func(t *testing.T) {
		cmd := GetRootCmd()
		startCmd := cmd.Commands()

		found := false
		for _, c := range startCmd {
			if c.Name() == "start" {
				found = true
				break
			}
		}
		assert.True(t, found, "start command should exist")
	})

	t.Run("help text", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"start", "--help"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		helpText := output.String()
		assert.Contains(t, helpText, "Start the Ranya daemon service")
	})

	t.Run("daemon not running", func(t *testing.T) {
		// Create temp PID file path
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "test.pid")

		// Ensure PID file doesn't exist
		os.Remove(pidFile)

		running := isRunning(pidFile)
		assert.False(t, running)
	})
}

func TestGetPIDFilePath(t *testing.T) {
	path := getPIDFilePath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "ranya.pid")
}

func TestIsRunning(t *testing.T) {
	t.Run("no pid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "nonexistent.pid")

		running := isRunning(pidFile)
		assert.False(t, running)
	})

	t.Run("invalid pid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "invalid.pid")

		err := os.WriteFile(pidFile, []byte("invalid"), 0644)
		require.NoError(t, err)

		running := isRunning(pidFile)
		assert.False(t, running)
	})
}
