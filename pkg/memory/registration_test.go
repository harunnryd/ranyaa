package memory

import (
	"context"
	"testing"

	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockToolExecutor for testing
type MockToolExecutor struct {
	registeredTools []toolexecutor.ToolDefinition
}

func (m *MockToolExecutor) RegisterTool(def toolexecutor.ToolDefinition) error {
	m.registeredTools = append(m.registeredTools, def)
	return nil
}

func TestRegisterMemoryTools(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock manager (we don't need a real one for registration tests)
	manager := &Manager{}

	t.Run("register all tools", func(t *testing.T) {
		executor := &MockToolExecutor{}

		err := RegisterMemoryTools(executor, manager, tmpDir)
		require.NoError(t, err)

		// Verify all 4 tools were registered
		assert.Len(t, executor.registeredTools, 4)

		// Verify tool names
		toolNames := make(map[string]bool)
		for _, tool := range executor.registeredTools {
			toolNames[tool.Name] = true
		}

		assert.True(t, toolNames["memory_search"])
		assert.True(t, toolNames["memory_write"])
		assert.True(t, toolNames["memory_delete"])
		assert.True(t, toolNames["memory_list"])
	})

	t.Run("verify tool parameters", func(t *testing.T) {
		executor := &MockToolExecutor{}

		err := RegisterMemoryTools(executor, manager, tmpDir)
		require.NoError(t, err)

		// Find memory_search tool
		var searchTool *toolexecutor.ToolDefinition
		for i, tool := range executor.registeredTools {
			if tool.Name == "memory_search" {
				searchTool = &executor.registeredTools[i]
				break
			}
		}

		require.NotNil(t, searchTool)
		assert.NotEmpty(t, searchTool.Description)
		assert.Greater(t, len(searchTool.Parameters), 0)

		// Verify query parameter exists and is required
		var queryParam *toolexecutor.ToolParameter
		for i, param := range searchTool.Parameters {
			if param.Name == "query" {
				queryParam = &searchTool.Parameters[i]
				break
			}
		}

		require.NotNil(t, queryParam)
		assert.True(t, queryParam.Required)
		assert.Equal(t, "string", queryParam.Type)
	})

	t.Run("verify tool handlers", func(t *testing.T) {
		executor := &MockToolExecutor{}

		err := RegisterMemoryTools(executor, manager, tmpDir)
		require.NoError(t, err)

		// Verify all tools have handlers
		for _, tool := range executor.registeredTools {
			assert.NotNil(t, tool.Handler, "Tool %s should have a handler", tool.Name)
		}
	})
}

func TestMemoryToolHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &Manager{}
	executor := &MockToolExecutor{}

	err := RegisterMemoryTools(executor, manager, tmpDir)
	require.NoError(t, err)

	t.Run("memory_list handler", func(t *testing.T) {
		// Find memory_list tool
		var listTool *toolexecutor.ToolDefinition
		for i, tool := range executor.registeredTools {
			if tool.Name == "memory_list" {
				listTool = &executor.registeredTools[i]
				break
			}
		}

		require.NotNil(t, listTool)
		require.NotNil(t, listTool.Handler)

		// Test handler with empty params
		params := map[string]interface{}{}
		result, err := listTool.Handler(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify result is MemoryListResult
		listResult, ok := result.(*MemoryListResult)
		assert.True(t, ok)
		assert.NotNil(t, listResult)
	})
}
