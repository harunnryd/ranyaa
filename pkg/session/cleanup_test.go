package session

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCleanup(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 7*24*time.Hour)
	assert.NotNil(t, cleanup)
	assert.Equal(t, manager, cleanup.manager)
	assert.Equal(t, 7*24*time.Hour, cleanup.cleanupAge)
}

func TestNewCleanup_DefaultAge(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 0)
	assert.Equal(t, DefaultCleanupAge, cleanup.cleanupAge)
}

func TestCleanupStartStop(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 7*24*time.Hour)

	// Test start
	err = cleanup.Start()
	assert.NoError(t, err)
	assert.True(t, cleanup.IsRunning())

	// Give it a moment to run initial cleanup
	time.Sleep(100 * time.Millisecond)

	// Test start again (should fail)
	err = cleanup.Start()
	assert.Error(t, err)

	// Test stop
	err = cleanup.Stop()
	assert.NoError(t, err)
	assert.False(t, cleanup.IsRunning())

	// Test stop again (should fail)
	err = cleanup.Stop()
	assert.Error(t, err)
}

func TestCleanupOldSessions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	// Use very short cleanup age for testing
	cleanup := NewCleanup(manager, 100*time.Millisecond)

	// Create an archived session
	sessionKey := "archived_old-session"
	err = manager.CreateSession(sessionKey)
	require.NoError(t, err)

	// Add a message
	err = manager.AppendMessage(sessionKey, Message{
		Role:      "user",
		Content:   "Test",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Make the session file old
	sessionPath := filepath.Join(tempDir, sessionKey+".jsonl")
	oldTime := time.Now().Add(-1 * time.Hour)
	err = os.Chtimes(sessionPath, oldTime, oldTime)
	require.NoError(t, err)

	// Run cleanup
	err = cleanup.CleanupNow()
	assert.NoError(t, err)

	// Check that session was deleted
	sessions, err := manager.ListSessions()
	require.NoError(t, err)
	assert.NotContains(t, sessions, sessionKey)
}

func TestCleanupOnlyArchivedSessions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 100*time.Millisecond)

	// Create a regular session (should not be cleaned up)
	regularKey := "regular-session"
	err = manager.CreateSession(regularKey)
	require.NoError(t, err)

	// Create an archived session (should be cleaned up)
	archivedKey := "archived_old-session"
	err = manager.CreateSession(archivedKey)
	require.NoError(t, err)

	// Make both files old
	regularPath := filepath.Join(tempDir, regularKey+".jsonl")
	archivedPath := filepath.Join(tempDir, archivedKey+".jsonl")
	oldTime := time.Now().Add(-1 * time.Hour)

	err = os.Chtimes(regularPath, oldTime, oldTime)
	require.NoError(t, err)
	err = os.Chtimes(archivedPath, oldTime, oldTime)
	require.NoError(t, err)

	// Run cleanup
	err = cleanup.CleanupNow()
	assert.NoError(t, err)

	// Check results
	sessions, err := manager.ListSessions()
	require.NoError(t, err)

	// Regular session should still exist
	assert.Contains(t, sessions, regularKey)

	// Archived session should be deleted
	assert.NotContains(t, sessions, archivedKey)
}

func TestSetCleanupAge(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 7*24*time.Hour)
	assert.Equal(t, 7*24*time.Hour, cleanup.GetCleanupAge())

	cleanup.SetCleanupAge(14 * 24 * time.Hour)
	assert.Equal(t, 14*24*time.Hour, cleanup.GetCleanupAge())
}

func TestGetCleanupStats(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 7*24*time.Hour)

	// Create some sessions
	err = manager.CreateSession("session1")
	require.NoError(t, err)
	err = manager.CreateSession("archived_session2")
	require.NoError(t, err)
	err = manager.CreateSession("archived_session3")
	require.NoError(t, err)

	// Get stats
	stats, err := cleanup.GetCleanupStats()
	assert.NoError(t, err)
	assert.Equal(t, 3, stats["total_sessions"])
	assert.Equal(t, 2, stats["archived_sessions"])
	assert.False(t, stats["running"].(bool))
}

func TestCleanupPrunesLargeSessions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	cleanup := NewCleanup(manager, 7*24*time.Hour)
	cleanup.SetMaxEntries(500)

	sessionKey := "session-prune"
	err = manager.CreateSession(sessionKey)
	require.NoError(t, err)

	for i := 0; i < 1000; i++ {
		err = manager.AppendMessage(sessionKey, Message{
			Role:      "user",
			Content:   "msg-" + strconv.Itoa(i),
			Timestamp: time.Now(),
		})
		require.NoError(t, err)
	}

	err = cleanup.CleanupNow()
	require.NoError(t, err)

	entries, err := manager.LoadSession(sessionKey)
	require.NoError(t, err)
	require.Len(t, entries, 500)
	assert.Equal(t, "msg-500", entries[0].Message.Content)
	assert.Equal(t, "msg-999", entries[len(entries)-1].Message.Content)
}
