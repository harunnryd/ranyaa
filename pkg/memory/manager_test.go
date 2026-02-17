package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestManager(t *testing.T) (*Manager, string, func()) {
	// Create temp workspace
	workspace, err := os.MkdirTemp("", "memory-test-*")
	require.NoError(t, err)

	// Create temp db
	dbPath := filepath.Join(workspace, "test.db")

	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)

	// Use mock embedding provider
	embeddingProvider := NewMockEmbeddingProvider(384)

	m, err := NewManager(Config{
		WorkspacePath:     workspace,
		DBPath:            dbPath,
		Logger:            logger,
		EmbeddingProvider: embeddingProvider,
	})
	require.NoError(t, err)

	cleanup := func() {
		m.Close()
		os.RemoveAll(workspace)
	}

	return m, workspace, cleanup
}

func TestNewManager(t *testing.T) {
	m, _, cleanup := createTestManager(t)
	defer cleanup()

	assert.NotNil(t, m)
	assert.NotNil(t, m.db)
	assert.NotNil(t, m.watcher)
}

func TestNewManager_InvalidConfig(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "empty workspace",
			config: Config{
				WorkspacePath: "",
				DBPath:        "/tmp/test.db",
				Logger:        logger,
			},
		},
		{
			name: "empty db path",
			config: Config{
				WorkspacePath: "/tmp",
				DBPath:        "",
				Logger:        logger,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewManager(tt.config)
			assert.Error(t, err)
			assert.Nil(t, m)
		})
	}
}

func TestSync_EmptyWorkspace(t *testing.T) {
	m, _, cleanup := createTestManager(t)
	defer cleanup()

	err := m.Sync()
	require.NoError(t, err)

	status := m.Status()
	assert.Equal(t, 0, status.TotalFiles)
	assert.Equal(t, 0, status.TotalChunks)
	assert.False(t, status.IsDirty)
}

func TestSync_SingleFile(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create a markdown file
	content := "# Test Document\n\nThis is a test document with some content."
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	err = m.Sync()
	require.NoError(t, err)

	status := m.Status()
	assert.Equal(t, 1, status.TotalFiles)
	assert.Greater(t, status.TotalChunks, 0)
}

func TestSync_MultipleFiles(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create multiple markdown files
	files := map[string]string{
		"doc1.md": "# Document 1\n\nContent for document 1.",
		"doc2.md": "# Document 2\n\nContent for document 2.",
		"doc3.md": "# Document 3\n\nContent for document 3.",
	}

	for name, content := range files {
		err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644)
		require.NoError(t, err)
	}

	err := m.Sync()
	require.NoError(t, err)

	status := m.Status()
	assert.Equal(t, 3, status.TotalFiles)
	assert.Greater(t, status.TotalChunks, 0)
}

func TestSync_IgnoresNonMarkdown(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create markdown and non-markdown files
	files := map[string]string{
		"doc.md":   "# Markdown\n\nThis should be indexed.",
		"doc.txt":  "This should be ignored.",
		"doc.html": "<h1>This should be ignored</h1>",
	}

	for name, content := range files {
		err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644)
		require.NoError(t, err)
	}

	err := m.Sync()
	require.NoError(t, err)

	status := m.Status()
	assert.Equal(t, 1, status.TotalFiles) // Only .md file
}

func TestSync_Idempotent(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create a file
	content := "# Test\n\nContent"
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	// First sync
	err = m.Sync()
	require.NoError(t, err)

	status1 := m.Status()

	// Second sync without changes
	m.MarkDirty() // Force sync
	err = m.Sync()
	require.NoError(t, err)

	status2 := m.Status()

	// Should have same counts
	assert.Equal(t, status1.TotalFiles, status2.TotalFiles)
	assert.Equal(t, status1.TotalChunks, status2.TotalChunks)
}

func TestSync_DeletedFiles(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create files
	files := []string{"doc1.md", "doc2.md", "doc3.md"}
	for _, name := range files {
		content := "# " + name + "\n\nContent"
		err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644)
		require.NoError(t, err)
	}

	// First sync
	err := m.Sync()
	require.NoError(t, err)

	status1 := m.Status()
	assert.Equal(t, 3, status1.TotalFiles)

	// Delete one file
	err = os.Remove(filepath.Join(workspace, "doc2.md"))
	require.NoError(t, err)

	// Second sync
	m.MarkDirty()
	err = m.Sync()
	require.NoError(t, err)

	status2 := m.Status()
	assert.Equal(t, 2, status2.TotalFiles)
}

func TestSearch_EmptyQuery(t *testing.T) {
	m, _, cleanup := createTestManager(t)
	defer cleanup()

	results, err := m.Search("", nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_KeywordMatch(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create a file with searchable content
	content := `# Test Document

This document contains information about golang programming.
Golang is a great language for building scalable systems.
`
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	err = m.Sync()
	require.NoError(t, err)

	// Search for "golang"
	results, err := m.Search("golang", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Check result structure
	if len(results) > 0 {
		r := results[0]
		assert.NotEmpty(t, r.ChunkID)
		assert.NotEmpty(t, r.FilePath)
		assert.NotEmpty(t, r.Content)
		assert.Greater(t, r.Score, 0.0)
	}
}

func TestSearch_VectorSearch(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create files with similar content
	files := map[string]string{
		"doc1.md": "# Machine Learning\n\nMachine learning is a subset of artificial intelligence.",
		"doc2.md": "# Deep Learning\n\nDeep learning uses neural networks with multiple layers.",
		"doc3.md": "# Cooking\n\nCooking is the art of preparing food.",
	}

	for name, content := range files {
		err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644)
		require.NoError(t, err)
	}

	err := m.Sync()
	require.NoError(t, err)

	// Search for AI-related content
	results, err := m.Search("artificial intelligence", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Should have vector scores (mock provider generates deterministic embeddings)
	if len(results) > 0 {
		assert.NotNil(t, results[0].VectorScore, "Should have vector score")
	}
}

func TestSearch_HybridSearch(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	content := `# Golang Programming

Golang is a programming language created by Google.
It is designed for building scalable and concurrent systems.
`
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	err = m.Sync()
	require.NoError(t, err)

	// Search should use both vector and keyword
	results, err := m.Search("golang concurrent", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Should have both vector and keyword scores
	if len(results) > 0 {
		r := results[0]
		assert.NotNil(t, r.VectorScore)
		assert.NotNil(t, r.KeywordScore)
	}
}

func TestSearch_Limit(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create multiple files
	for i := 0; i < 10; i++ {
		content := "# Document\n\nThis document mentions golang multiple times. Golang golang golang."
		filename := filepath.Join(workspace, "doc"+string(rune('0'+i))+".md")
		err := os.WriteFile(filename, []byte(content), 0644)
		require.NoError(t, err)
	}

	err := m.Sync()
	require.NoError(t, err)

	// Search with limit
	results, err := m.Search("golang", &SearchOptions{Limit: 3})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 3)
}

func TestFileWatcher_AutoSync(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Initial sync
	err := m.Sync()
	require.NoError(t, err)

	status1 := m.Status()
	assert.False(t, status1.IsDirty)

	// Create a new file
	content := "# New Document\n\nThis is new content."
	err = os.WriteFile(filepath.Join(workspace, "new.md"), []byte(content), 0644)
	require.NoError(t, err)

	// Wait for file watcher to detect change
	time.Sleep(1 * time.Second)

	status2 := m.Status()
	assert.True(t, status2.IsDirty, "Index should be marked dirty after file change")
}

func TestEmbeddingCache(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Create a file
	content := "# Test\n\nSome content for testing embedding cache."
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	// First sync - should generate embeddings
	err = m.Sync()
	require.NoError(t, err)

	status1 := m.Status()
	assert.NotNil(t, status1.EmbeddingCacheHitRate)

	// Delete and recreate same file
	os.Remove(filepath.Join(workspace, "test.md"))
	m.MarkDirty()
	_ = m.Sync()

	err = os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	// Second sync - should use cached embeddings
	m.MarkDirty()
	err = m.Sync()
	require.NoError(t, err)

	status2 := m.Status()
	assert.NotNil(t, status2.EmbeddingCacheHitRate)
	// Cache hit rate should be higher
	if status1.EmbeddingCacheHitRate != nil && status2.EmbeddingCacheHitRate != nil {
		assert.GreaterOrEqual(t, *status2.EmbeddingCacheHitRate, *status1.EmbeddingCacheHitRate)
	}
}

func TestStatus(t *testing.T) {
	m, workspace, cleanup := createTestManager(t)
	defer cleanup()

	// Initial status
	status := m.Status()
	assert.Equal(t, 0, status.TotalFiles)
	assert.Equal(t, 0, status.TotalChunks)
	assert.True(t, status.IsDirty) // Starts dirty

	// Add file and sync
	content := "# Test\n\nContent"
	err := os.WriteFile(filepath.Join(workspace, "test.md"), []byte(content), 0644)
	require.NoError(t, err)

	err = m.Sync()
	require.NoError(t, err)

	// Updated status
	status = m.Status()
	assert.Equal(t, 1, status.TotalFiles)
	assert.Greater(t, status.TotalChunks, 0)
	assert.False(t, status.IsDirty)
	assert.False(t, status.IsSyncing)
	assert.NotNil(t, status.LastSyncTime)
}
