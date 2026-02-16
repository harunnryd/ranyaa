package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureMemoryDirectory(t *testing.T) {
	t.Run("create new directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		memoryPath, err := EnsureMemoryDirectory(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, "memory"), memoryPath)

		// Verify directory exists
		info, err := os.Stat(memoryPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("directory already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		memoryPath := filepath.Join(tmpDir, "memory")

		// Create directory first
		err := os.MkdirAll(memoryPath, 0755)
		require.NoError(t, err)

		// Should succeed without error
		result, err := EnsureMemoryDirectory(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, memoryPath, result)
	})

	t.Run("path exists but is not directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		memoryPath := filepath.Join(tmpDir, "memory")

		// Create file instead of directory
		err := os.WriteFile(memoryPath, []byte("test"), 0644)
		require.NoError(t, err)

		// Should fail
		_, err = EnsureMemoryDirectory(tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestValidateMemoryPath(t *testing.T) {
	t.Run("valid relative path", func(t *testing.T) {
		err := ValidateMemoryPath("notes/test.md")
		assert.NoError(t, err)
	})

	t.Run("empty path", func(t *testing.T) {
		err := ValidateMemoryPath("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		err := ValidateMemoryPath("/absolute/path.md")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be relative")
	})

	t.Run("parent directory reference rejected", func(t *testing.T) {
		err := ValidateMemoryPath("../escape.md")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parent directories")
	})
}

func TestGetMemoryFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid path", func(t *testing.T) {
		fullPath, err := GetMemoryFilePath(tmpDir, "notes/test.md")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, "notes/test.md"), fullPath)
	})

	t.Run("invalid path rejected", func(t *testing.T) {
		_, err := GetMemoryFilePath(tmpDir, "../escape.md")
		assert.Error(t, err)
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := GetMemoryFilePath(tmpDir, "notes/../../escape.md")
		assert.Error(t, err)
		// The error could be either "invalid components" or "escapes base directory"
		// depending on which check catches it first
		assert.True(t,
			contains(err.Error(), "invalid components") ||
				contains(err.Error(), "escapes base directory"),
			"Expected path validation error, got: %v", err)
	})
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("file exists", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.md")
		err := os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)

		exists, err := FileExists(testFile)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("file does not exist", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "nonexistent.md")

		exists, err := FileExists(testFile)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}
