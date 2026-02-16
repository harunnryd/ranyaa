package toolexecutor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAllowlistManager(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")

	am, err := NewAllowlistManager(filePath)

	require.NoError(t, err)
	assert.NotNil(t, am)
	assert.Equal(t, filePath, am.filePath)
	assert.Equal(t, 0, am.Count())
}

func TestNewAllowlistManager_DefaultPath(t *testing.T) {
	// Test with empty path (should use default)
	am, err := NewAllowlistManager("")

	require.NoError(t, err)
	assert.NotNil(t, am)
	assert.Contains(t, am.filePath, ".ranya/exec-approvals.json")
}

func TestAllowlistManager_Add(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		Args:    []string{"-la"},
		Reason:  "test",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)

	require.NoError(t, err)
	assert.Equal(t, 1, am.Count())
}

func TestAllowlistManager_Add_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		Args:    []string{"-la"},
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	// Add duplicate
	err = am.Add(entry)
	require.NoError(t, err)

	// Should still have only 1 entry
	assert.Equal(t, 1, am.Count())
}

func TestAllowlistManager_Add_InvalidEntry(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		// No command or pattern
		Reason:  "test",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "either command or pattern must be specified")
}

func TestAllowlistManager_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		Args:    []string{"-la"},
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	err = am.Remove("ls", []string{"-la"})

	require.NoError(t, err)
	assert.Equal(t, 0, am.Count())
}

func TestAllowlistManager_Remove_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	err = am.Remove("nonexistent", []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestAllowlistManager_IsAllowed_ExactMatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		Args:    []string{"-la"},
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	// Exact match
	assert.True(t, am.IsAllowed("ls", []string{"-la"}))

	// Different args
	assert.False(t, am.IsAllowed("ls", []string{"-l"}))

	// Different command
	assert.False(t, am.IsAllowed("pwd", []string{}))
}

func TestAllowlistManager_IsAllowed_CommandOnly(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		// No args - matches any args
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	// Should match with any args
	assert.True(t, am.IsAllowed("ls", []string{"-la"}))
	assert.True(t, am.IsAllowed("ls", []string{"-l"}))
	assert.True(t, am.IsAllowed("ls", []string{}))
}

func TestAllowlistManager_IsAllowed_Pattern(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Pattern: "git *",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	// Should match git commands
	assert.True(t, am.IsAllowed("git", []string{"status"}))
	assert.True(t, am.IsAllowed("git", []string{"commit"}))

	// Should not match other commands
	assert.False(t, am.IsAllowed("ls", []string{}))
}

func TestAllowlistManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")

	// Create and populate allowlist
	am1, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry1 := AllowlistEntry{
		Command: "ls",
		Args:    []string{"-la"},
		AddedAt: time.Now().Format(time.RFC3339),
	}
	entry2 := AllowlistEntry{
		Pattern: "git *",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am1.Add(entry1)
	require.NoError(t, err)
	err = am1.Add(entry2)
	require.NoError(t, err)

	// Save
	err = am1.Save()
	require.NoError(t, err)

	// Load into new manager
	am2, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	// Verify loaded correctly
	assert.Equal(t, 2, am2.Count())
	assert.True(t, am2.IsAllowed("ls", []string{"-la"}))
	assert.True(t, am2.IsAllowed("git", []string{"status"}))
}

func TestAllowlistManager_List(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry1 := AllowlistEntry{
		Command: "ls",
		AddedAt: time.Now().Format(time.RFC3339),
	}
	entry2 := AllowlistEntry{
		Command: "pwd",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry1)
	require.NoError(t, err)
	err = am.Add(entry2)
	require.NoError(t, err)

	entries := am.List()

	assert.Equal(t, 2, len(entries))
}

func TestAllowlistManager_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-allowlist.json")
	am, err := NewAllowlistManager(filePath)
	require.NoError(t, err)

	entry := AllowlistEntry{
		Command: "ls",
		AddedAt: time.Now().Format(time.RFC3339),
	}

	err = am.Add(entry)
	require.NoError(t, err)

	err = am.Clear()

	require.NoError(t, err)
	assert.Equal(t, 0, am.Count())
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		str     string
		want    bool
	}{
		{"*", "anything", true},
		{"git *", "git status", true},
		{"git *", "git commit", true},
		{"git *", "ls", false},
		{"ls -?", "ls -l", true},
		{"ls -?", "ls -la", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.str, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.str)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAllowlistManager_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(filePath, []byte("invalid json"), 0644)
	require.NoError(t, err)

	_, err = NewAllowlistManager(filePath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse allowlist")
}
