package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusCommand(t *testing.T) {
	t.Run("command exists", func(t *testing.T) {
		cmd := GetRootCmd()
		statusCmd := cmd.Commands()

		found := false
		for _, c := range statusCmd {
			if c.Name() == "status" {
				found = true
				break
			}
		}
		assert.True(t, found, "status command should exist")
	})

	t.Run("help text", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"status", "--help"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		helpText := output.String()
		assert.Contains(t, helpText, "status")
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 2*time.Minute + 30*time.Second, "2m30s"},
		{"hours minutes seconds", 3*time.Hour + 15*time.Minute + 20*time.Second, "3h15m20s"},
		{"zero", 0, "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}
