package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginDiscovery_DiscoverPlugins(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	discovery := NewPluginDiscovery(logger)

	t.Run("discovers plugins from all directories", func(t *testing.T) {
		// Create temp directories
		tempDir := t.TempDir()
		builtinDir := filepath.Join(tempDir, "builtin")
		workspaceDir := filepath.Join(tempDir, "workspace")
		extraDir := filepath.Join(tempDir, "extra")

		// Create plugin directories with plugin.json
		createTestPlugin(t, builtinDir, "plugin1")
		createTestPlugin(t, builtinDir, "plugin2")
		createTestPlugin(t, workspaceDir, "plugin3")
		createTestPlugin(t, extraDir, "plugin4")

		// Discover plugins
		config := PluginDiscoveryConfig{
			BuiltinDir:   builtinDir,
			WorkspaceDir: workspaceDir,
			ExtraDirs:    []string{extraDir},
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		assert.Len(t, discovered, 4)

		// Verify sources
		sources := make(map[PluginSource]int)
		for _, plugin := range discovered {
			sources[plugin.Source]++
		}
		assert.Equal(t, 2, sources[SourceBuiltin])
		assert.Equal(t, 1, sources[SourceWorkspace])
		assert.Equal(t, 1, sources[SourceExtra])
	})

	t.Run("skips directories without plugin.json", func(t *testing.T) {
		tempDir := t.TempDir()
		builtinDir := filepath.Join(tempDir, "builtin")

		// Create plugin with plugin.json
		createTestPlugin(t, builtinDir, "valid-plugin")

		// Create directory without plugin.json
		invalidDir := filepath.Join(builtinDir, "invalid-plugin")
		require.NoError(t, os.MkdirAll(invalidDir, 0755))

		config := PluginDiscoveryConfig{
			BuiltinDir: builtinDir,
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		assert.Len(t, discovered, 1)
		assert.Equal(t, "valid-plugin", discovered[0].ID)
	})

	t.Run("handles missing directories gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentDir := filepath.Join(tempDir, "nonexistent")

		config := PluginDiscoveryConfig{
			BuiltinDir: nonExistentDir,
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		assert.Empty(t, discovered)
	})

	t.Run("handles empty directories", func(t *testing.T) {
		tempDir := t.TempDir()
		emptyDir := filepath.Join(tempDir, "empty")
		require.NoError(t, os.MkdirAll(emptyDir, 0755))

		config := PluginDiscoveryConfig{
			BuiltinDir: emptyDir,
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		assert.Empty(t, discovered)
	})

	t.Run("handles multiple extra directories", func(t *testing.T) {
		tempDir := t.TempDir()
		extra1 := filepath.Join(tempDir, "extra1")
		extra2 := filepath.Join(tempDir, "extra2")

		createTestPlugin(t, extra1, "plugin1")
		createTestPlugin(t, extra2, "plugin2")

		config := PluginDiscoveryConfig{
			ExtraDirs: []string{extra1, extra2},
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		assert.Len(t, discovered, 2)

		for _, plugin := range discovered {
			assert.Equal(t, SourceExtra, plugin.Source)
		}
	})

	t.Run("returns correct manifest paths", func(t *testing.T) {
		tempDir := t.TempDir()
		builtinDir := filepath.Join(tempDir, "builtin")
		createTestPlugin(t, builtinDir, "test-plugin")

		config := PluginDiscoveryConfig{
			BuiltinDir: builtinDir,
		}

		discovered, err := discovery.DiscoverPlugins(config)
		require.NoError(t, err)
		require.Len(t, discovered, 1)

		expectedPath := filepath.Join(builtinDir, "test-plugin", "plugin.json")
		assert.Equal(t, expectedPath, discovered[0].ManifestPath)
	})
}

// createTestPlugin creates a test plugin directory with a plugin.json file
func createTestPlugin(t *testing.T, baseDir, pluginName string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, pluginName)
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	manifestPath := filepath.Join(pluginDir, "plugin.json")
	manifestContent := `{
		"id": "` + pluginName + `",
		"name": "Test Plugin",
		"version": "1.0.0",
		"main": "main"
	}`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))
}
