package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerTriggerExecutesHookScript(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "startup.txt")
	hookScript := "echo startup > " + outputPath

	manager, err := NewManager(Config{
		Enabled: true,
		Logger:  zerolog.Nop(),
		Hooks: []Hook{
			{
				ID:      "startup",
				Event:   "daemon:startup",
				Script:  hookScript,
				Enabled: true,
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, manager.Trigger(context.Background(), "daemon:startup", nil))

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "startup\n", string(content))
}

func TestManagerTriggerInjectsEventDataIntoEnvironment(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "env.txt")
	hookScript := "echo \"$RANYA_HOOK_EVENT:$RANYA_HOOK_DATA_TASK_ID\" > " + outputPath

	manager, err := NewManager(Config{
		Enabled: true,
		Logger:  zerolog.Nop(),
		Hooks: []Hook{
			{
				ID:      "command",
				Event:   "command:new",
				Script:  hookScript,
				Enabled: true,
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, manager.Trigger(context.Background(), "command:new", map[string]interface{}{
		"task_id": "main-42",
	}))

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "command:new:main-42\n", string(content))
}

func TestManagerTriggerReturnsJoinedErrors(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled: true,
		Logger:  zerolog.Nop(),
		Hooks: []Hook{
			{
				ID:      "fail-1",
				Event:   "agent:bootstrap",
				Script:  "exit 2",
				Enabled: true,
			},
			{
				ID:      "fail-2",
				Event:   "agent:bootstrap",
				Script:  "exit 3",
				Enabled: true,
			},
		},
	})
	require.NoError(t, err)

	err = manager.Trigger(context.Background(), "agent:bootstrap", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook fail-1 failed")
	assert.Contains(t, err.Error(), "hook fail-2 failed")
}

func TestManagerTriggerRespectsTimeout(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled: true,
		Logger:  zerolog.Nop(),
		Hooks: []Hook{
			{
				ID:      "timeout",
				Event:   "daemon:startup",
				Script:  "sleep 1",
				Enabled: true,
				Timeout: 30 * time.Millisecond,
			},
		},
	})
	require.NoError(t, err)

	err = manager.Trigger(context.Background(), "daemon:startup", nil)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "signal: killed"),
		"expected timeout-related error, got: %v",
		err,
	)
}
