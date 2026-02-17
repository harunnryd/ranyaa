package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// testWorkspace represents a temporary workspace for testing
type testWorkspace struct {
	Path  string
	Files map[string]string
}

// createTempWorkspace creates a temporary workspace directory with files
func createTempWorkspace(t *testing.T, files map[string]string) *testWorkspace {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	ws := &testWorkspace{
		Path:  tmpDir,
		Files: files,
	}

	// Create files
	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)

		// Create parent directories
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	return ws
}

// cleanup removes the temporary workspace
func (ws *testWorkspace) cleanup(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(ws.Path); err != nil {
		t.Errorf("Failed to cleanup workspace: %v", err)
	}
}

// generateTestFiles generates a set of test files for various file types
func generateTestFiles() map[string]string {
	return map[string]string{
		"AGENTS.md":                "# Agent Instructions\n\nYou are an AI assistant.",
		"SOUL.md":                  "# Agent Soul\n\n## Identity\n- Name: Molty",
		"TOOLS.md":                 "# Available Tools\n\n- search\n- calculate",
		"MEMORY.md":                "# Memory\n\nRemember important facts.",
		"WEBHOOKS.md":              "# Webhooks\n\nWebhook configuration.",
		"CRON.md":                  "# Cron Jobs\n\nScheduled tasks.",
		"PLUGINS.md":               "# Plugins\n\nPlugin configuration.",
		"SKILL.md":                 "# Skills\n\nAgent skills.",
		"config/test.yaml":         "key: value\nport: 8080\n",
		"config/app.json":          `{"name": "test", "version": "1.0.0"}`,
		"plugins/test/plugin.json": `{"id": "test", "name": "Test Plugin", "version": "1.0.0"}`,
	}
}

// generateInvalidYAML generates invalid YAML content for testing
func generateInvalidYAML() string {
	return "invalid: yaml: content:\n  - missing\n    - indent"
}

// generateInvalidJSON generates invalid JSON content for testing
func generateInvalidJSON() string {
	return `{"invalid": json, "missing": "quotes"}`
}

// generateLargeFile generates a file content larger than the specified size
func generateLargeFile(size int) string {
	line := "This is a test line to make the file large.\n"
	result := ""
	for len(result) < size {
		result += line
	}
	return result
}
