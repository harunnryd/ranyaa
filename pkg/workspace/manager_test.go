package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceManager_Create(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	defer manager.Close()
}

func TestWorkspaceManager_Init(t *testing.T) {
	files := GenerateTestFiles()
	ws := CreateTempWorkspace(t, files)
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Listen for initialized event
	var initPayload InitializedPayload
	var wg sync.WaitGroup
	wg.Add(1)

	manager.On(EventInitialized, func(payload interface{}) {
		initPayload = payload.(InitializedPayload)
		wg.Done()
	})

	// Initialize
	err = manager.Init()
	require.NoError(t, err)

	// Wait for event
	wg.Wait()

	// Check that files were loaded
	assert.Greater(t, initPayload.FileCount, 0)
	assert.Greater(t, manager.cache.Size(), 0)
}

func TestWorkspaceManager_GetFile(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{
		"AGENTS.md": "# Agent Instructions",
	})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Get file by path
	filePath := filepath.Join(ws.Path, "AGENTS.md")
	file, ok := manager.GetFile(filePath)
	assert.True(t, ok)
	assert.NotNil(t, file)
	assert.Equal(t, FileTypeAgents, file.Type)
}

func TestWorkspaceManager_GetFileByName(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{
		"AGENTS.md": "# Agent Instructions",
	})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Get file by name
	file, ok := manager.GetFileByName("AGENTS.md")
	assert.True(t, ok)
	assert.NotNil(t, file)
	assert.Equal(t, FileTypeAgents, file.Type)
}

func TestWorkspaceManager_GetFileContent(t *testing.T) {
	content := "# Agent Instructions\n\nYou are an AI assistant."
	ws := CreateTempWorkspace(t, map[string]string{
		"AGENTS.md": content,
	})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Get file content
	retrieved, ok := manager.GetFileContent("AGENTS.md")
	assert.True(t, ok)
	assert.Equal(t, content, retrieved)
}

func TestWorkspaceManager_GetAllFiles(t *testing.T) {
	files := map[string]string{
		"AGENTS.md": "# Agents",
		"SOUL.md":   "# Soul",
		"TOOLS.md":  "# Tools",
	}
	ws := CreateTempWorkspace(t, files)
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Get all files
	allFiles := manager.GetAllFiles()
	assert.Len(t, allFiles, 3)
}

func TestWorkspaceManager_GetFilesByType(t *testing.T) {
	files := map[string]string{
		"AGENTS.md":        "# Agents",
		"SOUL.md":          "# Soul",
		"config/test.yaml": "key: value",
		"config/app.yaml":  "app: test",
	}
	ws := CreateTempWorkspace(t, files)
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Get config files
	configFiles := manager.GetFilesByType(FileTypeConfig)
	assert.Len(t, configFiles, 2)

	// Get agent files
	agentFiles := manager.GetFilesByType(FileTypeAgents)
	assert.Len(t, agentFiles, 1)
}

func TestWorkspaceManager_FileAdded(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Listen for file added event
	var addedFile FileEventPayload
	var wg sync.WaitGroup
	wg.Add(1)

	manager.On(EventFileAdded, func(payload interface{}) {
		addedFile = payload.(FileEventPayload)
		wg.Done()
	})

	err = manager.Init()
	require.NoError(t, err)

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Add a new file
	testFile := filepath.Join(ws.Path, "test.md")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Wait for event (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, "test.md", addedFile.RelativePath)
		assert.Equal(t, FileTypeOther, addedFile.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file added event")
	}
}

func TestWorkspaceManager_FileChanged(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{
		"test.md": "initial content",
	})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Listen for file changed event
	var changedFile FileChangedPayload
	var wg sync.WaitGroup
	wg.Add(1)

	manager.On(EventFileChanged, func(payload interface{}) {
		changedFile = payload.(FileChangedPayload)
		wg.Done()
	})

	err = manager.Init()
	require.NoError(t, err)

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	testFile := filepath.Join(ws.Path, "test.md")
	err = os.WriteFile(testFile, []byte("updated content"), 0644)
	require.NoError(t, err)

	// Wait for event (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, "test.md", changedFile.RelativePath)
		assert.True(t, changedFile.HasChanges)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file changed event")
	}
}

func TestWorkspaceManager_FileDeleted(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{
		"test.md": "content",
	})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Listen for file deleted event
	var deletedFile FileEventPayload
	var wg sync.WaitGroup
	wg.Add(1)

	manager.On(EventFileDeleted, func(payload interface{}) {
		deletedFile = payload.(FileEventPayload)
		wg.Done()
	})

	err = manager.Init()
	require.NoError(t, err)

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Delete the file
	testFile := filepath.Join(ws.Path, "test.md")
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for event (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, "test.md", deletedFile.RelativePath)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file deleted event")
	}
}

func TestWorkspaceManager_CriticalFileIdentification(t *testing.T) {
	files := map[string]string{
		"AGENTS.md":   "# Agents",
		"SOUL.md":     "# Soul",
		"MEMORY.md":   "# Memory",
		"config.yaml": "key: value",
	}
	ws := CreateTempWorkspace(t, files)
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	err = manager.Init()
	require.NoError(t, err)

	// Give it time to load
	time.Sleep(100 * time.Millisecond)

	// Check critical files
	agentsFile, _ := manager.GetFileByName("AGENTS.md")
	assert.True(t, agentsFile.IsCritical)

	soulFile, _ := manager.GetFileByName("SOUL.md")
	assert.True(t, soulFile.IsCritical)

	configFile, _ := manager.GetFileByName("config.yaml")
	assert.True(t, configFile.IsCritical)

	// Memory is not critical
	memoryFile, _ := manager.GetFileByName("MEMORY.md")
	assert.False(t, memoryFile.IsCritical)
}

func TestWorkspaceManager_Close(t *testing.T) {
	ws := CreateTempWorkspace(t, map[string]string{})
	defer ws.Cleanup(t)

	config := WorkspaceConfig{
		WorkspacePath: ws.Path,
	}

	manager, err := NewWorkspaceManager(config)
	require.NoError(t, err)

	err = manager.Init()
	require.NoError(t, err)

	// Close manager
	err = manager.Close()
	assert.NoError(t, err)

	// Cache should be empty
	assert.Equal(t, 0, manager.cache.Size())

	// Should not be able to init again
	err = manager.Init()
	assert.Error(t, err)
}
