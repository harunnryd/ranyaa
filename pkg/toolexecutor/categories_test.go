package toolexecutor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidCategory(t *testing.T) {
	tests := []struct {
		name     string
		category string
		want     bool
	}{
		{"valid read", "read", true},
		{"valid write", "write", true},
		{"valid shell", "shell", true},
		{"valid web", "web", true},
		{"valid spec", "spec", true},
		{"valid general", "general", true},
		{"invalid category", "invalid", false},
		{"empty category", "", false},
		{"case insensitive", "READ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidCategory(tt.category)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToolRegistry_Register(t *testing.T) {
	t.Run("register valid tool", func(t *testing.T) {
		registry := NewToolRegistry()

		err := registry.Register("read_file", "Read a file", CategoryRead)
		require.NoError(t, err)

		tool, err := registry.Get("read_file")
		require.NoError(t, err)
		assert.Equal(t, "read_file", tool.Name)
		assert.Equal(t, CategoryRead, tool.Category)
	})

	t.Run("register tool without category defaults to general", func(t *testing.T) {
		registry := NewToolRegistry()

		err := registry.Register("custom_tool", "Custom tool", "")
		require.NoError(t, err)

		tool, err := registry.Get("custom_tool")
		require.NoError(t, err)
		assert.Equal(t, CategoryGeneral, tool.Category)
	})

	t.Run("register tool without name fails", func(t *testing.T) {
		registry := NewToolRegistry()

		err := registry.Register("", "No name", CategoryRead)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tool name is required")
	})

	t.Run("register tool with invalid category fails", func(t *testing.T) {
		registry := NewToolRegistry()

		err := registry.Register("bad_tool", "Bad category", "invalid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid category")
	})
}

func TestToolRegistry_FilterByCategory(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools in different categories
	require.NoError(t, registry.Register("read_file", "Read file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write file", CategoryWrite))
	require.NoError(t, registry.Register("exec_command", "Execute command", CategoryShell))
	require.NoError(t, registry.Register("http_get", "HTTP GET", CategoryWeb))
	require.NoError(t, registry.Register("read_dir", "Read directory", CategoryRead))

	t.Run("filter by read category", func(t *testing.T) {
		filtered := registry.FilterByCategory(CategoryRead)
		assert.Len(t, filtered, 2)
		names := []string{filtered[0].Name, filtered[1].Name}
		assert.Contains(t, names, "read_file")
		assert.Contains(t, names, "read_dir")
	})

	t.Run("filter by write category", func(t *testing.T) {
		filtered := registry.FilterByCategory(CategoryWrite)
		assert.Len(t, filtered, 1)
		assert.Equal(t, "write_file", filtered[0].Name)
	})

	t.Run("filter by non-existent category", func(t *testing.T) {
		filtered := registry.FilterByCategory(CategorySpec)
		assert.Len(t, filtered, 0)
	})
}

func TestToolRegistry_FilterByCategories(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools
	require.NoError(t, registry.Register("read_file", "Read file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write file", CategoryWrite))
	require.NoError(t, registry.Register("exec_command", "Execute command", CategoryShell))
	require.NoError(t, registry.Register("http_get", "HTTP GET", CategoryWeb))

	t.Run("filter by multiple categories", func(t *testing.T) {
		filtered := registry.FilterByCategories([]ToolCategory{CategoryRead, CategoryWrite})
		assert.Len(t, filtered, 2)
		names := []string{filtered[0].Name, filtered[1].Name}
		assert.Contains(t, names, "read_file")
		assert.Contains(t, names, "write_file")
	})

	t.Run("filter by empty categories", func(t *testing.T) {
		filtered := registry.FilterByCategories([]ToolCategory{})
		assert.Len(t, filtered, 0)
	})
}

func TestCategoryMatcher_MatchesCategory(t *testing.T) {
	registry := NewToolRegistry()
	require.NoError(t, registry.Register("read_file", "Read file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write file", CategoryWrite))

	matcher := NewCategoryMatcher(registry)

	t.Run("matches correct category", func(t *testing.T) {
		assert.True(t, matcher.MatchesCategory("read_file", CategoryRead))
		assert.True(t, matcher.MatchesCategory("write_file", CategoryWrite))
	})

	t.Run("does not match wrong category", func(t *testing.T) {
		assert.False(t, matcher.MatchesCategory("read_file", CategoryWrite))
		assert.False(t, matcher.MatchesCategory("write_file", CategoryRead))
	})

	t.Run("unknown tool returns false", func(t *testing.T) {
		assert.False(t, matcher.MatchesCategory("unknown_tool", CategoryRead))
	})
}

func TestCategoryMatcher_ApplyCategoryPolicy(t *testing.T) {
	registry := NewToolRegistry()
	require.NoError(t, registry.Register("read_file", "Read file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write file", CategoryWrite))
	require.NoError(t, registry.Register("exec_command", "Execute command", CategoryShell))

	matcher := NewCategoryMatcher(registry)

	t.Run("allow specific categories", func(t *testing.T) {
		allowed := matcher.ApplyCategoryPolicy("read_file", []ToolCategory{CategoryRead}, []ToolCategory{})
		assert.True(t, allowed)

		denied := matcher.ApplyCategoryPolicy("write_file", []ToolCategory{CategoryRead}, []ToolCategory{})
		assert.False(t, denied)
	})

	t.Run("deny overrides allow", func(t *testing.T) {
		// Allow read and write, but deny write
		allowed := matcher.ApplyCategoryPolicy("write_file",
			[]ToolCategory{CategoryRead, CategoryWrite},
			[]ToolCategory{CategoryWrite})
		assert.False(t, allowed)
	})

	t.Run("empty allow list allows all except denied", func(t *testing.T) {
		// No allow list, deny shell
		readAllowed := matcher.ApplyCategoryPolicy("read_file", []ToolCategory{}, []ToolCategory{CategoryShell})
		assert.True(t, readAllowed)

		shellDenied := matcher.ApplyCategoryPolicy("exec_command", []ToolCategory{}, []ToolCategory{CategoryShell})
		assert.False(t, shellDenied)
	})

	t.Run("unknown tool is denied", func(t *testing.T) {
		allowed := matcher.ApplyCategoryPolicy("unknown_tool", []ToolCategory{CategoryRead}, []ToolCategory{})
		assert.False(t, allowed)
	})
}

func TestAllCategories(t *testing.T) {
	categories := AllCategories()
	assert.Len(t, categories, 6)
	assert.Contains(t, categories, CategoryRead)
	assert.Contains(t, categories, CategoryWrite)
	assert.Contains(t, categories, CategoryShell)
	assert.Contains(t, categories, CategoryWeb)
	assert.Contains(t, categories, CategorySpec)
	assert.Contains(t, categories, CategoryGeneral)
}
