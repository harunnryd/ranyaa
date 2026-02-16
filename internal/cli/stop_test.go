package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStopCommand(t *testing.T) {
	t.Run("command exists", func(t *testing.T) {
		cmd := GetRootCmd()
		stopCmd := cmd.Commands()

		found := false
		for _, c := range stopCmd {
			if c.Name() == "stop" {
				found = true
				break
			}
		}
		assert.True(t, found, "stop command should exist")
	})

	t.Run("help text", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"stop", "--help"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		helpText := output.String()
		assert.Contains(t, helpText, "Stop the Ranya daemon service")
		assert.Contains(t, helpText, "timeout")
	})
}
