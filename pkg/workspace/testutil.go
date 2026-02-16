package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWorkspace represents a temporary workspace for testing
type TestWorkspace struct {
	Path  string
	Files map[string]string
}

// CreateTempWorkspace creates a temporary workspace directory with files
func CreateTempWorkspace(t *testing.T, files map[string]string) *TestWorkspace {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	ws := &TestWorkspace{
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

// Cleanup removes the temporary workspace
func (ws *TestWorkspace) Cleanup(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(ws.Path); err != nil {
		t.Errorf("Failed to cleanup workspace: %v", err)
	}
}

// AddFile adds a new file to the workspace
func (ws *TestWorkspace) AddFile(t *testing.T, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(ws.Path, relPath)

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", fullPath, err)
	}

	ws.Files[relPath] = content
}

// UpdateFile updates an existing file in the workspace
func (ws *TestWorkspace) UpdateFile(t *testing.T, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(ws.Path, relPath)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to update file %s: %v", fullPath, err)
	}

	ws.Files[relPath] = content
}

// DeleteFile removes a file from the workspace
func (ws *TestWorkspace) DeleteFile(t *testing.T, relPath string) {
	t.Helper()

	fullPath := filepath.Join(ws.Path, relPath)
	if err := os.Remove(fullPath); err != nil {
		t.Fatalf("Failed to delete file %s: %v", fullPath, err)
	}

	delete(ws.Files, relPath)
}

// GenerateTestFiles generates a set of test files for various file types
func GenerateTestFiles() map[string]string {
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

// GenerateInvalidYAML generates invalid YAML content for testing
func GenerateInvalidYAML() string {
	return "invalid: yaml: content:\n  - missing\n    - indent"
}

// GenerateInvalidJSON generates invalid JSON content for testing
func GenerateInvalidJSON() string {
	return `{"invalid": json, "missing": "quotes"}`
}

// GenerateLargeFile generates a file content larger than the specified size
func GenerateLargeFile(size int) string {
	// Use a more efficient approach with strings.Builder
	var builder strings.Builder
	builder.Grow(size)

	line := "This is a test line to make the file large.\n"
	for builder.Len() < size {
		builder.WriteString(line)
	}
	return builder.String()
}

// ExpectedFileType returns the expected file type for a given path
func ExpectedFileType(path string) WorkspaceFileType {
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	dir := filepath.Dir(path)

	switch base {
	case "AGENTS.md":
		return FileTypeAgents
	case "SOUL.md":
		return FileTypeSoul
	case "TOOLS.md":
		return FileTypeTools
	case "MEMORY.md":
		return FileTypeMemory
	case "WEBHOOKS.md":
		return FileTypeWebhooks
	case "CRON.md":
		return FileTypeCron
	case "PLUGINS.md":
		return FileTypePlugins
	case "SKILL.md":
		return FileTypeSkill
	case "plugin.json":
		return FileTypePluginManifest
	}

	switch ext {
	case ".yaml", ".yml":
		return FileTypeConfig
	case ".json":
		if base != "plugin.json" {
			return FileTypeConfig
		}
	case ".go":
		if filepath.Base(dir) == "plugins" || strings.Contains(dir, "plugins") {
			return FileTypePluginCode
		}
	}

	return FileTypeOther
}

// IsCriticalFile returns whether a file type is critical
func IsCriticalFile(fileType WorkspaceFileType) bool {
	switch fileType {
	case FileTypeAgents, FileTypeSoul, FileTypeTools, FileTypeWebhooks,
		FileTypeCron, FileTypePlugins, FileTypeConfig:
		return true
	default:
		return false
	}
}

// WaitForCondition waits for a condition to be true or times out
func WaitForCondition(t *testing.T, condition func() bool, timeout, interval time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// AssertEventually asserts that a condition becomes true within a timeout
func AssertEventually(t *testing.T, condition func() bool, timeout, interval time.Duration, msg string) {
	t.Helper()

	if !WaitForCondition(t, condition, timeout, interval) {
		t.Fatalf("Condition not met within %v: %s", timeout, msg)
	}
}
