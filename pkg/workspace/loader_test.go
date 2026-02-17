package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceLoader_LoadFile_Markdown(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"AGENTS.md": "# Agent Instructions\n\nYou are an AI assistant.",
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "AGENTS.md")

	loaded, err := loader.LoadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, filePath, loaded.Path)
	assert.Equal(t, "# Agent Instructions\n\nYou are an AI assistant.", loaded.Content)
	assert.Equal(t, FileTypeAgents, loaded.Type)
	assert.NotEmpty(t, loaded.Hash)
	assert.Greater(t, loaded.Size, int64(0))
}

func TestWorkspaceLoader_LoadFile_YAML(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"config/test.yaml": "key: value\nport: 8080\n",
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "config/test.yaml")

	loaded, err := loader.LoadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, FileTypeConfig, loaded.Type)
	assert.NotNil(t, loaded.ParsedContent)

	// Check parsed content
	parsed, ok := loaded.ParsedContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "value", parsed["key"])
	assert.Equal(t, 8080, parsed["port"])
}

func TestWorkspaceLoader_LoadFile_JSON(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"config/test.json": `{"name": "test", "version": "1.0.0"}`,
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "config/test.json")

	loaded, err := loader.LoadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, FileTypeConfig, loaded.Type)
	assert.NotNil(t, loaded.ParsedContent)

	// Check parsed content
	parsed, ok := loaded.ParsedContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test", parsed["name"])
	assert.Equal(t, "1.0.0", parsed["version"])
}

func TestWorkspaceLoader_LoadFile_PluginManifest(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"plugins/test/plugin.json": `{"id": "test", "name": "Test Plugin", "version": "1.0.0"}`,
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "plugins/test/plugin.json")

	loaded, err := loader.LoadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, FileTypePluginManifest, loaded.Type)
	assert.NotNil(t, loaded.ParsedContent)
}

func TestWorkspaceLoader_LoadFile_InvalidYAML(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"config/invalid.yaml": generateInvalidYAML(),
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "config/invalid.yaml")

	_, err := loader.LoadFile(filePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestWorkspaceLoader_LoadFile_InvalidJSON(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"plugins/test/plugin.json": generateInvalidJSON(),
	})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "plugins/test/plugin.json")

	_, err := loader.LoadFile(filePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestWorkspaceLoader_LoadFile_NonExistent(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)
	filePath := filepath.Join(ws.Path, "nonexistent.md")

	_, err := loader.LoadFile(filePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestWorkspaceLoader_LoadFile_TooLarge(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	// Create a file larger than MaxFileSize
	largeFile := filepath.Join(ws.Path, "large.txt")
	content := generateLargeFile(MaxFileSize + 1000)
	err := os.WriteFile(largeFile, []byte(content), 0644)
	require.NoError(t, err)

	loader := NewWorkspaceLoader(ws.Path)

	_, err = loader.LoadFile(largeFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestWorkspaceLoader_DetectFileType(t *testing.T) {
	loader := NewWorkspaceLoader("/workspace")

	tests := []struct {
		path     string
		expected WorkspaceFileType
	}{
		{"/workspace/AGENTS.md", FileTypeAgents},
		{"/workspace/SOUL.md", FileTypeSoul},
		{"/workspace/TOOLS.md", FileTypeTools},
		{"/workspace/MEMORY.md", FileTypeMemory},
		{"/workspace/WEBHOOKS.md", FileTypeWebhooks},
		{"/workspace/CRON.md", FileTypeCron},
		{"/workspace/PLUGINS.md", FileTypePlugins},
		{"/workspace/SKILL.md", FileTypeSkill},
		{"/workspace/config/test.yaml", FileTypeConfig},
		{"/workspace/config/test.yml", FileTypeConfig},
		{"/workspace/config/test.json", FileTypeConfig},
		{"/workspace/plugins/test/plugin.json", FileTypePluginManifest},
		{"/workspace/plugins/test/main.go", FileTypePluginCode},
		{"/workspace/other.txt", FileTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := loader.detectFileType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkspaceLoader_ComputeHash(t *testing.T) {
	loader := NewWorkspaceLoader("/workspace")

	content1 := "test content"
	content2 := "test content"
	content3 := "different content"

	hash1 := loader.computeHash(content1)
	hash2 := loader.computeHash(content2)
	hash3 := loader.computeHash(content3)

	// Same content should produce same hash
	assert.Equal(t, hash1, hash2)

	// Different content should produce different hash
	assert.NotEqual(t, hash1, hash3)

	// Hash should be hex string
	assert.Len(t, hash1, 64) // SHA-256 produces 64 hex characters
}

func TestWorkspaceLoader_ValidatePath_DirectoryTraversal(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)

	// Test directory traversal attempts
	tests := []string{
		filepath.Join(ws.Path, "../outside.txt"),
		filepath.Join(ws.Path, "subdir/../../outside.txt"),
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			err := loader.validatePath(path)
			assert.Error(t, err)
			// Can be either "directory traversal" or "outside workspace"
			errMsg := err.Error()
			assert.True(t,
				strings.Contains(errMsg, "directory traversal") ||
					strings.Contains(errMsg, "outside workspace"),
				"error should mention directory traversal or outside workspace")
		})
	}
}

func TestWorkspaceLoader_ValidatePath_OutsideWorkspace(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)

	// Try to access file outside workspace
	outsidePath := "/tmp/outside.txt"
	err := loader.validatePath(outsidePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace")
}

func TestWorkspaceLoader_ValidatePath_ValidPath(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	loader := NewWorkspaceLoader(ws.Path)

	// Valid path within workspace
	validPath := filepath.Join(ws.Path, "AGENTS.md")
	err := loader.validatePath(validPath)
	assert.NoError(t, err)
}
