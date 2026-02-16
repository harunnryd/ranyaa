package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestLoader_LoadManifest(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	loader := NewManifestLoader(logger)

	t.Run("loads minimal valid manifest", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0",
			"main": "main"
		}`

		path := createManifestFile(t, manifest)
		result, err := loader.LoadManifest(path)

		require.NoError(t, err)
		assert.Equal(t, "test-plugin", result.ID)
		assert.Equal(t, "Test Plugin", result.Name)
		assert.Equal(t, "1.0.0", result.Version)
		assert.Equal(t, "main", result.Main)
	})

	t.Run("loads manifest with all optional fields", func(t *testing.T) {
		manifest := `{
			"id": "full-plugin",
			"name": "Full Plugin",
			"version": "2.1.3",
			"description": "A complete plugin",
			"author": "Test Author",
			"main": "index.js",
			"dependencies": [
				{"pluginId": "dep1", "version": "^1.0.0"},
				{"pluginId": "dep2"}
			],
			"permissions": ["filesystem:read", "network:http"],
			"config": {"key": "value"},
			"exports": {
				"tools": ["tool1", "tool2"],
				"hooks": ["hook1"],
				"channels": ["channel1"],
				"providers": ["provider1"],
				"gatewayMethods": ["method1"]
			}
		}`

		path := createManifestFile(t, manifest)
		result, err := loader.LoadManifest(path)

		require.NoError(t, err)
		assert.Equal(t, "full-plugin", result.ID)
		assert.Equal(t, "A complete plugin", result.Description)
		assert.Equal(t, "Test Author", result.Author)
		assert.Len(t, result.Dependencies, 2)
		assert.Len(t, result.Permissions, 2)
		assert.NotNil(t, result.Config)
		assert.NotNil(t, result.Exports)
		assert.Len(t, result.Exports.Tools, 2)
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0"
			"main": "main"
		}` // Missing comma

		path := createManifestFile(t, manifest)
		_, err := loader.LoadManifest(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse manifest JSON")
	})

	t.Run("rejects manifest missing required fields", func(t *testing.T) {
		testCases := []struct {
			name     string
			manifest string
		}{
			{
				name: "missing id",
				manifest: `{
					"name": "Test Plugin",
					"version": "1.0.0",
					"main": "main"
				}`,
			},
			{
				name: "missing name",
				manifest: `{
					"id": "test-plugin",
					"version": "1.0.0",
					"main": "main"
				}`,
			},
			{
				name: "missing version",
				manifest: `{
					"id": "test-plugin",
					"name": "Test Plugin",
					"main": "main"
				}`,
			},
			{
				name: "missing main",
				manifest: `{
					"id": "test-plugin",
					"name": "Test Plugin",
					"version": "1.0.0"
				}`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				path := createManifestFile(t, tc.manifest)
				_, err := loader.LoadManifest(path)

				require.Error(t, err)
				assert.Contains(t, err.Error(), "schema validation")
			})
		}
	})

	t.Run("rejects invalid semver versions", func(t *testing.T) {
		testCases := []struct {
			name    string
			version string
		}{
			{"missing patch", "1.0"},
			{"missing minor and patch", "1"},
			{"with v prefix", "v1.0.0"},
			{"with extra parts", "1.0.0.0"},
			{"non-numeric", "1.0.x"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				manifest := `{
					"id": "test-plugin",
					"name": "Test Plugin",
					"version": "` + tc.version + `",
					"main": "main"
				}`

				path := createManifestFile(t, manifest)
				_, err := loader.LoadManifest(path)

				require.Error(t, err)
			})
		}
	})

	t.Run("rejects invalid plugin IDs", func(t *testing.T) {
		testCases := []struct {
			name string
			id   string
		}{
			{"uppercase", "TestPlugin"},
			{"spaces", "test plugin"},
			{"underscores", "test_plugin"},
			{"special chars", "test@plugin"},
			{"dots", "test.plugin"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				manifest := `{
					"id": "` + tc.id + `",
					"name": "Test Plugin",
					"version": "1.0.0",
					"main": "main"
				}`

				path := createManifestFile(t, manifest)
				_, err := loader.LoadManifest(path)

				require.Error(t, err)
				// Schema validation catches this
				assert.Contains(t, err.Error(), "schema validation")
			})
		}
	})

	t.Run("accepts valid plugin IDs", func(t *testing.T) {
		testCases := []string{
			"simple",
			"with-hyphens",
			"with123numbers",
			"a",
			"plugin-123-test",
		}

		for _, id := range testCases {
			t.Run(id, func(t *testing.T) {
				manifest := `{
					"id": "` + id + `",
					"name": "Test Plugin",
					"version": "1.0.0",
					"main": "main"
				}`

				path := createManifestFile(t, manifest)
				result, err := loader.LoadManifest(path)

				require.NoError(t, err)
				assert.Equal(t, id, result.ID)
			})
		}
	})

	t.Run("rejects unrecognized permissions", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0",
			"main": "main",
			"permissions": ["filesystem:read", "invalid:permission"]
		}`

		path := createManifestFile(t, manifest)
		_, err := loader.LoadManifest(path)

		require.Error(t, err)
		// Schema validation catches this
		assert.Contains(t, err.Error(), "schema validation")
	})

	t.Run("accepts all valid permissions", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0",
			"main": "main",
			"permissions": [
				"filesystem:read",
				"filesystem:write",
				"network:http",
				"network:websocket",
				"process:spawn",
				"database:read",
				"database:write",
				"gateway:register"
			]
		}`

		path := createManifestFile(t, manifest)
		result, err := loader.LoadManifest(path)

		require.NoError(t, err)
		assert.Len(t, result.Permissions, 8)
	})

	t.Run("validates dependency structure", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0",
			"main": "main",
			"dependencies": [
				{"pluginId": "dep1", "version": "^1.0.0"},
				{"pluginId": "dep2"}
			]
		}`

		path := createManifestFile(t, manifest)
		result, err := loader.LoadManifest(path)

		require.NoError(t, err)
		assert.Len(t, result.Dependencies, 2)
		assert.Equal(t, "dep1", result.Dependencies[0].PluginID)
		assert.Equal(t, "^1.0.0", result.Dependencies[0].Version)
		assert.Equal(t, "dep2", result.Dependencies[1].PluginID)
		assert.Empty(t, result.Dependencies[1].Version)
	})

	t.Run("rejects empty dependency pluginId", func(t *testing.T) {
		manifest := `{
			"id": "test-plugin",
			"name": "Test Plugin",
			"version": "1.0.0",
			"main": "main",
			"dependencies": [
				{"pluginId": ""}
			]
		}`

		path := createManifestFile(t, manifest)
		_, err := loader.LoadManifest(path)

		require.Error(t, err)
		// Schema validation catches this
		assert.Contains(t, err.Error(), "schema validation")
	})

	t.Run("handles file not found", func(t *testing.T) {
		_, err := loader.LoadManifest("/nonexistent/plugin.json")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read manifest file")
	})
}

// createManifestFile creates a temporary manifest file for testing
func createManifestFile(t *testing.T, content string) string {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "plugin.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}
