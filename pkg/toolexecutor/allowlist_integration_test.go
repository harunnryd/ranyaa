package toolexecutor

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllowlistManager_Persistence tests that allowlist persists across restarts
func TestAllowlistManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "persistence-test.json")

	// Create first manager and add entries
	am1, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entries := []AllowlistEntry{
		{Command: "ls", Args: []string{"-la"}, AddedAt: time.Now().Format(time.RFC3339)},
		{Command: "pwd", AddedAt: time.Now().Format(time.RFC3339)},
		{Pattern: "git *", AddedAt: time.Now().Format(time.RFC3339)},
	}

	for _, entry := range entries {
		err = am1.Add(entry)
		require.NoError(t, err)
	}

	// Save
	err = am1.Save()
	require.NoError(t, err)

	// Create second manager (simulates restart)
	am2, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	// Verify all entries loaded
	assert.Equal(t, 3, am2.Count())
	assert.True(t, am2.IsAllowed("ls", []string{"-la"}))
	assert.True(t, am2.IsAllowed("pwd", []string{}))
	assert.True(t, am2.IsAllowed("git", []string{"status"}))
}

// TestAllowlistManager_ConcurrentAccess tests concurrent read/write operations
func TestAllowlistManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "concurrent-test.json")

	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	numGoroutines := 10
	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			entry := AllowlistEntry{
				Command: "test",
				Args:    []string{string(rune(index))},
				AddedAt: time.Now().Format(time.RFC3339),
			}

			err := am.Add(entry)
			assert.NoError(t, err)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_ = am.List()
			_ = am.Count()
			_ = am.IsAllowed("test", []string{"0"})
		}()
	}

	wg.Wait()

	// Verify all entries added
	assert.Equal(t, numGoroutines, am.Count())
}

// TestAllowlistManager_LargeAllowlist tests performance with large allowlist
func TestAllowlistManager_LargeAllowlist(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large-test.json")

	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	// Add 1000 entries
	numEntries := 1000
	for i := 0; i < numEntries; i++ {
		entry := AllowlistEntry{
			Command: "test",
			Args:    []string{string(rune(i))},
			AddedAt: time.Now().Format(time.RFC3339),
		}

		err = am.Add(entry)
		require.NoError(t, err)
	}

	// Save
	start := time.Now()
	err = am.Save()
	require.NoError(t, err)
	saveDuration := time.Since(start)

	// Load
	am2, err := NewAllowlistManager(filePath)
	require.NoError(t, err)
	loadDuration := time.Since(start) - saveDuration

	// Verify
	assert.Equal(t, numEntries, am2.Count())

	// Performance check (should be fast)
	assert.Less(t, saveDuration, 1*time.Second, "Save took too long")
	assert.Less(t, loadDuration, 1*time.Second, "Load took too long")
}

// TestAllowlistManager_FilePermissions tests file permission handling
func TestAllowlistManager_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "permissions-test.json")

	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	// Save
	err = am.Save()
	require.NoError(t, err)

	// Check file permissions
	info, err := os.Stat(filePath)
	require.NoError(t, err)

	// Should be readable and writable by owner
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}
