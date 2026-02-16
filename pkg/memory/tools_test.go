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

func TestMemorySearch(t *testing.T) {
	// Create temp workspace and database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	err := os.WriteFile(testFile, []byte("This is a test document about Go programming."), 0644)
	require.NoError(t, err)

	// Create manager
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	manager, err := NewManager(Config{
		WorkspacePath:     tmpDir,
		DBPath:            dbPath,
		Logger:            logger,
		EmbeddingProvider: NewMockEmbeddingProvider(128),
	})
	require.NoError(t, err)
	defer manager.Close()

	// Sync to index the file
	err = manager.Sync()
	require.NoError(t, err)

	t.Run("successful search", func(t *testing.T) {
		params := MemorySearchParams{
			Query: "Go programming",
			Limit: 10,
		}

		result, err := MemorySearch(context.Background(), manager, params)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Go programming", result.Query)
		assert.Greater(t, result.Count, 0)
	})

	t.Run("empty query", func(t *testing.T) {
		params := MemorySearchParams{
			Query: "",
		}

		_, err := MemorySearch(context.Background(), manager, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "query is required")
	})

	t.Run("with custom weights", func(t *testing.T) {
		params := MemorySearchParams{
			Query:         "test",
			Limit:         5,
			VectorWeight:  0.8,
			KeywordWeight: 0.2,
		}

		result, err := MemorySearch(context.Background(), manager, params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("with min score", func(t *testing.T) {
		params := MemorySearchParams{
			Query:    "test",
			MinScore: 0.5,
		}

		result, err := MemorySearch(context.Background(), manager, params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("no results", func(t *testing.T) {
		// Create a separate manager with empty database for this test
		emptyDir := t.TempDir()
		emptyDBPath := filepath.Join(emptyDir, "empty.db")

		emptyManager, err := NewManager(Config{
			WorkspacePath:     emptyDir,
			DBPath:            emptyDBPath,
			Logger:            logger,
			EmbeddingProvider: NewMockEmbeddingProvider(128),
		})
		require.NoError(t, err)
		defer emptyManager.Close()

		params := MemorySearchParams{
			Query: "nonexistent query xyz123",
		}

		result, err := MemorySearch(context.Background(), emptyManager, params)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Count)
	})
}

func TestMemoryWrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	manager, err := NewManager(Config{
		WorkspacePath: tmpDir,
		DBPath:        dbPath,
		Logger:        logger,
	})
	require.NoError(t, err)
	defer manager.Close()

	t.Run("create new file", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "notes/test.md",
			Content: "This is a test note.",
		}

		result, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		require.NoError(t, err)
		assert.Equal(t, "notes/test.md", result.Path)
		assert.True(t, result.Created)
		assert.Equal(t, len(params.Content), result.BytesWritten)

		// Verify file exists
		fullPath := filepath.Join(tmpDir, "notes/test.md")
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, params.Content, string(content))
	})

	t.Run("update existing file", func(t *testing.T) {
		// Create initial file
		params1 := MemoryWriteParams{
			Path:    "update.md",
			Content: "Initial content",
		}
		_, err := MemoryWrite(context.Background(), manager, tmpDir, params1)
		require.NoError(t, err)

		// Update file
		params2 := MemoryWriteParams{
			Path:    "update.md",
			Content: "Updated content",
		}
		result, err := MemoryWrite(context.Background(), manager, tmpDir, params2)
		require.NoError(t, err)
		assert.False(t, result.Created)
		assert.Equal(t, len(params2.Content), result.BytesWritten)
	})

	t.Run("empty path", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "",
			Content: "test",
		}

		_, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path is required")
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "/absolute/path.md",
			Content: "test",
		}

		_, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must be relative")
	})

	t.Run("non-markdown extension rejected", func(t *testing.T) {
		params := MemoryWriteParams{
			Path:    "test.txt",
			Content: "test",
		}

		_, err := MemoryWrite(context.Background(), manager, tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must end with .md")
	})
}

func TestMemoryDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	manager, err := NewManager(Config{
		WorkspacePath: tmpDir,
		DBPath:        dbPath,
		Logger:        logger,
	})
	require.NoError(t, err)
	defer manager.Close()

	t.Run("delete existing file", func(t *testing.T) {
		// Create file first
		testFile := filepath.Join(tmpDir, "delete-me.md")
		err := os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)

		params := MemoryDeleteParams{
			Path: "delete-me.md",
		}

		result, err := MemoryDelete(context.Background(), manager, tmpDir, params)
		require.NoError(t, err)
		assert.True(t, result.Deleted)
		assert.Equal(t, "delete-me.md", result.Path)

		// Verify file is deleted
		_, err = os.Stat(testFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete non-existent file", func(t *testing.T) {
		params := MemoryDeleteParams{
			Path: "does-not-exist.md",
		}

		result, err := MemoryDelete(context.Background(), manager, tmpDir, params)
		require.NoError(t, err)
		assert.False(t, result.Deleted)
	})

	t.Run("empty path", func(t *testing.T) {
		params := MemoryDeleteParams{
			Path: "",
		}

		_, err := MemoryDelete(context.Background(), manager, tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path is required")
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		params := MemoryDeleteParams{
			Path: "/absolute/path.md",
		}

		_, err := MemoryDelete(context.Background(), manager, tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must be relative")
	})
}

func TestMemoryList(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"file1.md",
		"file2.md",
		"subdir/file3.md",
		"other.txt", // Should be ignored
	}

	for _, f := range files {
		fullPath := filepath.Join(tmpDir, f)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	t.Run("list all files", func(t *testing.T) {
		params := MemoryListParams{}

		result, err := MemoryList(context.Background(), tmpDir, params)
		require.NoError(t, err)
		assert.Equal(t, 3, result.Count) // Only .md files
		assert.Len(t, result.Files, 3)

		// Verify file info
		for _, file := range result.Files {
			assert.NotEmpty(t, file.Path)
			assert.Greater(t, file.SizeBytes, int64(0))
			assert.False(t, file.ModifiedTime.IsZero())
		}
	})

	t.Run("list with pattern", func(t *testing.T) {
		params := MemoryListParams{
			Pattern: "file*.md",
		}

		result, err := MemoryList(context.Background(), tmpDir, params)
		require.NoError(t, err)
		assert.Equal(t, 2, result.Count) // file1.md and file2.md
	})

	t.Run("invalid pattern", func(t *testing.T) {
		params := MemoryListParams{
			Pattern: "[invalid",
		}

		_, err := MemoryList(context.Background(), tmpDir, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})
}
