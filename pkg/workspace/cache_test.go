package workspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceCache_SetAndGet(t *testing.T) {
	cache := NewWorkspaceCache()

	file := &WorkspaceFile{
		Path:         "/workspace/AGENTS.md",
		RelativePath: "AGENTS.md",
		Content:      "test content",
		Hash:         "abc123",
		LoadedAt:     time.Now(),
		Type:         FileTypeAgents,
		Size:         12,
		IsCritical:   true,
	}

	// Set file
	cache.Set(file.Path, file)

	// Get file
	retrieved, ok := cache.Get(file.Path)
	assert.True(t, ok)
	assert.Equal(t, file, retrieved)
}

func TestWorkspaceCache_GetNonExistent(t *testing.T) {
	cache := NewWorkspaceCache()

	retrieved, ok := cache.Get("/nonexistent")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestWorkspaceCache_Has(t *testing.T) {
	cache := NewWorkspaceCache()

	file := &WorkspaceFile{
		Path: "/workspace/AGENTS.md",
		Type: FileTypeAgents,
	}

	assert.False(t, cache.Has(file.Path))

	cache.Set(file.Path, file)
	assert.True(t, cache.Has(file.Path))
}

func TestWorkspaceCache_Delete(t *testing.T) {
	cache := NewWorkspaceCache()

	file := &WorkspaceFile{
		Path: "/workspace/AGENTS.md",
		Type: FileTypeAgents,
	}

	cache.Set(file.Path, file)
	assert.True(t, cache.Has(file.Path))

	deleted := cache.Delete(file.Path)
	assert.True(t, deleted)
	assert.False(t, cache.Has(file.Path))

	// Delete non-existent
	deleted = cache.Delete(file.Path)
	assert.False(t, deleted)
}

func TestWorkspaceCache_GetAll(t *testing.T) {
	cache := NewWorkspaceCache()

	files := []*WorkspaceFile{
		{Path: "/workspace/AGENTS.md", Type: FileTypeAgents},
		{Path: "/workspace/SOUL.md", Type: FileTypeSoul},
		{Path: "/workspace/config.yaml", Type: FileTypeConfig},
	}

	for _, file := range files {
		cache.Set(file.Path, file)
	}

	// Get all without filter
	all := cache.GetAll(nil)
	assert.Len(t, all, 3)

	// Get all with filter
	filtered := cache.GetAll(func(file *WorkspaceFile) bool {
		return file.Type == FileTypeAgents || file.Type == FileTypeSoul
	})
	assert.Len(t, filtered, 2)
}

func TestWorkspaceCache_GetByType(t *testing.T) {
	cache := NewWorkspaceCache()

	files := []*WorkspaceFile{
		{Path: "/workspace/AGENTS.md", Type: FileTypeAgents},
		{Path: "/workspace/SOUL.md", Type: FileTypeSoul},
		{Path: "/workspace/config1.yaml", Type: FileTypeConfig},
		{Path: "/workspace/config2.yaml", Type: FileTypeConfig},
	}

	for _, file := range files {
		cache.Set(file.Path, file)
	}

	// Get by type
	configs := cache.GetByType(FileTypeConfig)
	assert.Len(t, configs, 2)

	agents := cache.GetByType(FileTypeAgents)
	assert.Len(t, agents, 1)

	memory := cache.GetByType(FileTypeMemory)
	assert.Len(t, memory, 0)
}

func TestWorkspaceCache_Clear(t *testing.T) {
	cache := NewWorkspaceCache()

	files := []*WorkspaceFile{
		{Path: "/workspace/AGENTS.md", Type: FileTypeAgents},
		{Path: "/workspace/SOUL.md", Type: FileTypeSoul},
	}

	for _, file := range files {
		cache.Set(file.Path, file)
	}

	assert.Equal(t, 2, cache.Size())

	cache.Clear()
	assert.Equal(t, 0, cache.Size())
	assert.False(t, cache.Has("/workspace/AGENTS.md"))
}

func TestWorkspaceCache_Size(t *testing.T) {
	cache := NewWorkspaceCache()
	assert.Equal(t, 0, cache.Size())

	cache.Set("/workspace/AGENTS.md", &WorkspaceFile{Path: "/workspace/AGENTS.md"})
	assert.Equal(t, 1, cache.Size())

	cache.Set("/workspace/SOUL.md", &WorkspaceFile{Path: "/workspace/SOUL.md"})
	assert.Equal(t, 2, cache.Size())

	cache.Delete("/workspace/AGENTS.md")
	assert.Equal(t, 1, cache.Size())
}

func TestWorkspaceCache_ConcurrentAccess(t *testing.T) {
	cache := NewWorkspaceCache()

	// Test concurrent writes and reads
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			file := &WorkspaceFile{
				Path: "/workspace/file" + string(rune(i)),
				Type: FileTypeOther,
			}
			cache.Set(file.Path, file)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetAll(nil)
			cache.Size()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Should not panic and should have some files
	assert.True(t, cache.Size() > 0)
}
