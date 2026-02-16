package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManager(t *testing.T) (*SessionManager, string) {
	tempDir := t.TempDir()
	sm, err := New(tempDir)
	require.NoError(t, err)
	return sm, tempDir
}

func TestSessionManager_CreateSession(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	err := sm.CreateSession("test-session")
	assert.NoError(t, err)

	// Creating again should succeed
	err = sm.CreateSession("test-session")
	assert.NoError(t, err)
}

func TestSessionManager_ValidateSessionKey(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	tests := []struct {
		name      string
		key       string
		shouldErr bool
	}{
		{"valid key", "test-session", false},
		{"empty key", "", true},
		{"path traversal", "../etc/passwd", true},
		{"forward slash", "test/session", true},
		{"backslash", "test\\session", true},
		{"null byte", "test\x00session", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.validateSessionKey(tt.key)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSessionManager_AppendMessage(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	msg := Message{
		Role:      "user",
		Content:   "Hello, world!",
		Timestamp: time.Now(),
	}

	err := sm.AppendMessage("test-session", msg)
	assert.NoError(t, err)

	// Verify file exists
	sessionPath := sm.getSessionPath("test-session")
	_, err = os.Stat(sessionPath)
	assert.NoError(t, err)
}

func TestSessionManager_LoadSession(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	// Append messages
	messages := []Message{
		{Role: "user", Content: "Message 1", Timestamp: time.Now()},
		{Role: "assistant", Content: "Message 2", Timestamp: time.Now()},
		{Role: "user", Content: "Message 3", Timestamp: time.Now()},
	}

	for _, msg := range messages {
		err := sm.AppendMessage("test-session", msg)
		require.NoError(t, err)
	}

	// Load session
	entries, err := sm.LoadSession("test-session")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(entries))

	// Verify content
	for i, entry := range entries {
		assert.Equal(t, "test-session", entry.SessionKey)
		assert.Equal(t, messages[i].Role, entry.Message.Role)
		assert.Equal(t, messages[i].Content, entry.Message.Content)
	}
}

func TestSessionManager_LoadNonExistentSession(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	entries, err := sm.LoadSession("non-existent")
	assert.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSessionManager_DeleteSession(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	// Create and append message
	msg := Message{
		Role:      "user",
		Content:   "Test",
		Timestamp: time.Now(),
	}
	err := sm.AppendMessage("test-session", msg)
	require.NoError(t, err)

	// Delete session
	err = sm.DeleteSession("test-session")
	assert.NoError(t, err)

	// Verify file is deleted
	sessionPath := sm.getSessionPath("test-session")
	_, err = os.Stat(sessionPath)
	assert.True(t, os.IsNotExist(err))
}

func TestSessionManager_ListSessions(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	// Create multiple sessions
	sessions := []string{"session1", "session2", "session3"}
	for _, key := range sessions {
		err := sm.CreateSession(key)
		require.NoError(t, err)
	}

	// List sessions
	list, err := sm.ListSessions()
	assert.NoError(t, err)
	assert.ElementsMatch(t, sessions, list)
}

func TestSessionManager_RepairSession(t *testing.T) {
	sm, tempDir := setupTestManager(t)
	defer sm.Close()

	// Create session with valid and invalid lines
	sessionPath := filepath.Join(tempDir, "test-session.jsonl")
	content := `{"sessionKey":"test-session","message":{"role":"user","content":"Valid 1","timestamp":"2024-01-01T00:00:00Z"}}
invalid json line
{"sessionKey":"test-session","message":{"role":"assistant","content":"Valid 2","timestamp":"2024-01-01T00:00:01Z"}}
{"invalid":"entry"}
{"sessionKey":"test-session","message":{"role":"user","content":"Valid 3","timestamp":"2024-01-01T00:00:02Z"}}
`
	err := os.WriteFile(sessionPath, []byte(content), 0600)
	require.NoError(t, err)

	// Repair session
	err = sm.RepairSession("test-session")
	assert.NoError(t, err)

	// Load and verify
	entries, err := sm.LoadSession("test-session")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(entries))
}

func TestSessionManager_GetSessionInfo(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	// Create session with messages
	for i := 0; i < 5; i++ {
		msg := Message{
			Role:      "user",
			Content:   "Test message",
			Timestamp: time.Now(),
		}
		err := sm.AppendMessage("test-session", msg)
		require.NoError(t, err)
	}

	// Get info
	info, err := sm.GetSessionInfo("test-session")
	assert.NoError(t, err)
	assert.Equal(t, "test-session", info["sessionKey"])
	assert.Equal(t, 5, info["messageCount"])
	assert.Greater(t, info["size"].(int64), int64(0))
}

func TestSessionManager_ConcurrentWrites(t *testing.T) {
	sm, _ := setupTestManager(t)
	defer sm.Close()

	const numGoroutines = 10
	const messagesPerGoroutine = 10

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := Message{
					Role:      "user",
					Content:   "Concurrent message",
					Timestamp: time.Now(),
				}
				err := sm.AppendMessage("concurrent-session", msg)
				assert.NoError(t, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all messages were written
	entries, err := sm.LoadSession("concurrent-session")
	assert.NoError(t, err)
	assert.Equal(t, numGoroutines*messagesPerGoroutine, len(entries))
}
