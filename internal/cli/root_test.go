package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand(t *testing.T) {
	t.Run("version flag", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"--version"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		assert.Contains(t, output.String(), "ranya version")
		assert.Contains(t, output.String(), GetVersion())
	})

	t.Run("help flag", func(t *testing.T) {
		cmd := GetRootCmd()
		cmd.SetArgs([]string{"--help"})

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		err := cmd.Execute()
		require.NoError(t, err)

		helpText := output.String()
		assert.Contains(t, helpText, "Ranya")
		assert.Contains(t, helpText, "orchestration")
	})

	t.Run("global flags", func(t *testing.T) {
		cmd := GetRootCmd()

		// Check config flag exists
		configFlag := cmd.PersistentFlags().Lookup("config")
		require.NotNil(t, configFlag)
		assert.Equal(t, "", configFlag.DefValue)

		// Check log-level flag exists
		logLevelFlag := cmd.PersistentFlags().Lookup("log-level")
		require.NotNil(t, logLevelFlag)
		assert.Equal(t, "info", logLevelFlag.DefValue)
	})
}

func TestGetVersion(t *testing.T) {
	version := GetVersion()
	assert.NotEmpty(t, version)
	assert.True(t, strings.HasPrefix(version, "0."))
}
