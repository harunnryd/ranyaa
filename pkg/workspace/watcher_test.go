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

func TestWorkspaceWatcher_Create(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 50 * time.Millisecond,
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)
	require.NotNil(t, watcher)

	defer watcher.Stop()
}

func TestWorkspaceWatcher_StartStop(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 50 * time.Millisecond,
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)

	err = watcher.Start()
	require.NoError(t, err)

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	err = watcher.Stop()
	assert.NoError(t, err)
}

func TestWorkspaceWatcher_FileAdded(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{})
	defer ws.cleanup(t)

	var addedPath string
	var wg sync.WaitGroup
	wg.Add(1)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 50 * time.Millisecond,
		OnFileAdded: func(path string) error {
			addedPath = path
			wg.Done()
			return nil
		},
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)

	err = watcher.Start()
	require.NoError(t, err)
	defer watcher.Stop()

	// Create a new file
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
		assert.Equal(t, testFile, addedPath)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file added event")
	}
}

func TestWorkspaceWatcher_FileChanged(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"test.md": "initial content",
	})
	defer ws.cleanup(t)

	var changedPath string
	var wg sync.WaitGroup
	wg.Add(1)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 50 * time.Millisecond,
		OnFileChanged: func(path string) error {
			changedPath = path
			wg.Done()
			return nil
		},
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)

	err = watcher.Start()
	require.NoError(t, err)
	defer watcher.Stop()

	// Give watcher time to initialize
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
		assert.Equal(t, testFile, changedPath)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file changed event")
	}
}

func TestWorkspaceWatcher_FileDeleted(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"test.md": "content",
	})
	defer ws.cleanup(t)

	var deletedPath string
	var wg sync.WaitGroup
	wg.Add(1)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 50 * time.Millisecond,
		OnFileDeleted: func(path string) error {
			deletedPath = path
			wg.Done()
			return nil
		},
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)

	err = watcher.Start()
	require.NoError(t, err)
	defer watcher.Stop()

	// Give watcher time to initialize
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
		assert.Equal(t, testFile, deletedPath)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file deleted event")
	}
}

func TestWorkspaceWatcher_ShouldIgnore(t *testing.T) {
	watcher := &WorkspaceWatcher{
		workspacePath: "/workspace",
	}

	tests := []struct {
		path   string
		ignore bool
	}{
		{"/workspace/AGENTS.md", false},
		{"/workspace/.git/config", true},
		{"/workspace/.env", true},
		{"/workspace/.env.local", true},
		{"/workspace/node_modules/package/index.js", true},
		{"/workspace/.hidden", true},
		{"/workspace/.workspace/config.yaml", true}, // All dotfiles ignored
		{"/workspace/config.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := watcher.shouldIgnore(tt.path)
			assert.Equal(t, tt.ignore, result)
		})
	}
}

func TestWorkspaceWatcher_Debouncing(t *testing.T) {
	ws := createTempWorkspace(t, map[string]string{
		"test.md": "initial",
	})
	defer ws.cleanup(t)

	changeCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	config := WorkspaceWatcherConfig{
		WorkspacePath:      ws.Path,
		StabilityThreshold: 100 * time.Millisecond,
		OnFileChanged: func(path string) error {
			mu.Lock()
			changeCount++
			if changeCount == 1 {
				wg.Done()
			}
			mu.Unlock()
			return nil
		},
	}

	watcher, err := NewWorkspaceWatcher(config)
	require.NoError(t, err)

	err = watcher.Start()
	require.NoError(t, err)
	defer watcher.Stop()

	// Give watcher time to initialize
	time.Sleep(100 * time.Millisecond)

	testFile := filepath.Join(ws.Path, "test.md")

	// Make multiple rapid changes
	for i := 0; i < 5; i++ {
		err = os.WriteFile(testFile, []byte("content"+string(rune(i))), 0644)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounced event
	done := make(chan struct{})
	go func() {
		wg.Wait()
		// Wait a bit more to see if more events come
		time.Sleep(200 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		// Should have received only 1 event due to debouncing
		assert.Equal(t, 1, changeCount)
		mu.Unlock()
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for debounced event")
	}
}
