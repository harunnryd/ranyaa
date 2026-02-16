package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewArchiver(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 30*time.Minute)
	assert.NotNil(t, archiver)
	assert.Equal(t, manager, archiver.manager)
	assert.Equal(t, 30*time.Minute, archiver.idleTimeout)
}

func TestNewArchiver_DefaultTimeout(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 0)
	assert.Equal(t, DefaultIdleTimeout, archiver.idleTimeout)
}

func TestArchiverStartStop(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 30*time.Minute)

	// Test start
	err = archiver.Start()
	assert.NoError(t, err)
	assert.True(t, archiver.IsRunning())

	// Test start again (should fail)
	err = archiver.Start()
	assert.Error(t, err)

	// Test stop
	err = archiver.Stop()
	assert.NoError(t, err)
	assert.False(t, archiver.IsRunning())

	// Test stop again (should fail)
	err = archiver.Stop()
	assert.Error(t, err)
}

func TestArchiveSession(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 30*time.Minute)

	// Create a session
	sessionKey := "test-session"
	err = manager.CreateSession(sessionKey)
	require.NoError(t, err)

	// Add some messages
	err = manager.AppendMessage(sessionKey, Message{
		Role:      "user",
		Content:   "Hello",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Archive the session
	err = archiver.ArchiveNow(sessionKey)
	assert.NoError(t, err)

	// Check that original session is deleted
	sessions, err := manager.ListSessions()
	require.NoError(t, err)
	assert.NotContains(t, sessions, sessionKey)

	// Check that archived session exists
	archivedKey := "archived_" + sessionKey
	assert.Contains(t, sessions, archivedKey)

	// Verify archived session has the messages
	entries, err := manager.LoadSession(archivedKey)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "Hello", entries[0].Message.Content)
}

func TestArchiveIdleSessions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	// Use very short timeout for testing
	archiver := NewArchiver(manager, 100*time.Millisecond)

	// Create a session
	sessionKey := "idle-session"
	err = manager.CreateSession(sessionKey)
	require.NoError(t, err)

	// Add a message
	err = manager.AppendMessage(sessionKey, Message{
		Role:      "user",
		Content:   "Test",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Make the session file old by modifying its timestamp
	sessionPath := filepath.Join(tempDir, sessionKey+".jsonl")
	oldTime := time.Now().Add(-1 * time.Hour)
	err = os.Chtimes(sessionPath, oldTime, oldTime)
	require.NoError(t, err)

	// Run archiver
	err = archiver.archiveIdleSessions()
	assert.NoError(t, err)

	// Check that session was archived
	sessions, err := manager.ListSessions()
	require.NoError(t, err)
	assert.NotContains(t, sessions, sessionKey)
	assert.Contains(t, sessions, "archived_"+sessionKey)
}

func TestIsArchivedSession(t *testing.T) {
	assert.True(t, isArchivedSession("archived_test"))
	assert.True(t, isArchivedSession("archived_session-123"))
	assert.False(t, isArchivedSession("test"))
	assert.False(t, isArchivedSession("session-123"))
	assert.False(t, isArchivedSession("archived"))
}

func TestGetArchivedSessions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 30*time.Minute)

	// Create regular and archived sessions
	err = manager.CreateSession("session1")
	require.NoError(t, err)
	err = manager.CreateSession("archived_session2")
	require.NoError(t, err)
	err = manager.CreateSession("archived_session3")
	require.NoError(t, err)

	// Get archived sessions
	archived, err := archiver.GetArchivedSessions()
	assert.NoError(t, err)
	assert.Len(t, archived, 2)
	assert.Contains(t, archived, "archived_session2")
	assert.Contains(t, archived, "archived_session3")
}

func TestSetIdleTimeout(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := New(tempDir)
	require.NoError(t, err)

	archiver := NewArchiver(manager, 30*time.Minute)
	assert.Equal(t, 30*time.Minute, archiver.GetIdleTimeout())

	archiver.SetIdleTimeout(1 * time.Hour)
	assert.Equal(t, 1*time.Hour, archiver.GetIdleTimeout())
}
