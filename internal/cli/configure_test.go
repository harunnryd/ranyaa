package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureCommand(t *testing.T) {
	t.Run("command exists", func(t *testing.T) {
		cmd := GetRootCmd()
		configureCmd := cmd.Commands()

		found := false
		for _, c := range configureCmd {
			if c.Name() == "configure" {
				found = true
				break
			}
		}
		assert.True(t, found, "configure command should exist")
	})

	t.Run("help text", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"configure", "--help"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		helpText := output.String()
		assert.Contains(t, helpText, "interactive configuration wizard")
	})
}
