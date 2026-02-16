package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests that work with file system operations
// These tests don't require FTS5 support

func TestMemoryWriteIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)

	// Create manager without embedding provider to avoid FTS5 requirement
	manager, err := NewManager(Config{
		WorkspacePath:     tmpDir,
		DBPath:            dbPath,
		Logger:            logger,
		EmbeddingProvider: nil, // No embedding provider
	})

	// If FTS5 is not available, skip these tests
	if err != nil && contains(err.Error(), "fts5") {
		t.Skip("FTS5 not available, skipping integration tests")
	}
	require.NoError(t, err)
	defer manager.Close()

	t.Run("write and verify file", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "integration/test.md",
			Content: "# Integration Test\n\nThis is a test file.",
		}

		result, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		require.NoError(t, err)
		assert.True(t, result.Created)

		// Verify file exists and has correct content
		fullPath := filepath.Join(tmpDir, params.Path)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, params.Content, string(content))

		// Verify manager is marked dirty
		status := manager.Status()
		assert.True(t, status.IsDirty)
	})

	t.Run("update and verify file", func(t *testing.T) {
		// Create initial file
		params1 := MemoryWriteParams{
			Path:    "update-test.md",
			Content: "Initial content",
		}
		_, err := MemoryWrite(context.Background(), manager, tmpDir, params1)
		require.NoError(t, err)

		// Update file
		params2 := MemoryWriteParams{
			Path:    "update-test.md",
			Content: "Updated content with more text",
		}
		result, err := MemoryWrite(context.Background(), manager, tmpDir, params2)
		require.NoError(t, err)
		assert.False(t, result.Created)
		assert.Equal(t, len(params2.Content), result.BytesWritten)

		// Verify updated content
		fullPath := filepath.Join(tmpDir, params2.Path)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, params2.Content, string(content))
	})

	t.Run("nested directory creation", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "deep/nested/directory/file.md",
			Content: "Nested file content",
		}

		result, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		require.NoError(t, err)
		assert.True(t, result.Created)

		// Verify directory structure was created
		fullPath := filepath.Join(tmpDir, params.Path)
		_, err = os.Stat(fullPath)
		assert.NoError(t, err)
	})
}

func TestMemoryDeleteIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	manager, err := NewManager(Config{
		WorkspacePath:     tmpDir,
		DBPath:            dbPath,
		Logger:            logger,
		EmbeddingProvider: nil,
	})

	if err != nil && contains(err.Error(), "fts5") {
		t.Skip("FTS5 not available, skipping integration tests")
	}
	require.NoError(t, err)
	defer manager.Close()

	t.Run("delete and verify removal", func(t *testing.T) {
		// Create file first
		writeParams := MemoryWriteParams{
			Path:    "to-delete.md",
			Content: "This file will be deleted",
		}
		_, err := MemoryWrite(context.Background(), manager, tmpDir, writeParams)
		require.NoError(t, err)

		// Delete file
		deleteParams := MemoryDeleteParams{
			Path: "to-delete.md",
		}
		result, err := MemoryDelete(context.Background(), manager, tmpDir, deleteParams)
		require.NoError(t, err)
		assert.True(t, result.Deleted)

		// Verify file is gone
		fullPath := filepath.Join(tmpDir, writeParams.Path)
		_, err = os.Stat(fullPath)
		assert.True(t, os.IsNotExist(err))

		// Verify manager is marked dirty
		status := manager.Status()
		assert.True(t, status.IsDirty)
	})
}

func TestMemoryListIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	testFiles := map[string]string{
		"file1.md":            "Content 1",
		"file2.md":            "Content 2",
		"subdir/file3.md":     "Content 3",
		"subdir/file4.md":     "Content 4",
		"deep/nested/file.md": "Content 5",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	t.Run("list all markdown files", func(t *testing.T) {
		params := MemoryListParams{}
		result, err := MemoryList(context.Background(), tmpDir, params)
		require.NoError(t, err)
		assert.Equal(t, len(testFiles), result.Count)
		assert.Len(t, result.Files, len(testFiles))

		// Verify all files are present
		paths := make(map[string]bool)
		for _, file := range result.Files {
			paths[file.Path] = true
			assert.Greater(t, file.SizeBytes, int64(0))
			assert.False(t, file.ModifiedTime.IsZero())
		}

		for expectedPath := range testFiles {
			assert.True(t, paths[expectedPath], "Expected file %s not found", expectedPath)
		}
	})

	t.Run("list with glob pattern", func(t *testing.T) {
		params := MemoryListParams{
			Pattern: "subdir/*.md",
		}
		result, err := MemoryList(context.Background(), tmpDir, params)
		require.NoError(t, err)
		assert.Equal(t, 2, result.Count) // file3.md and file4.md
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
