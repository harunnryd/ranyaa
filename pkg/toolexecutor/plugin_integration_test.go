package toolexecutor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPluginRuntime implements PluginToolExecutor for testing
type mockPluginRuntime struct {
	plugins map[string]*mockPluginProvider
}

func newMockPluginRuntime() *mockPluginRuntime {
	return &mockPluginRuntime{
		plugins: make(map[string]*mockPluginProvider),
	}
}

func (m *mockPluginRuntime) GetPlugin(pluginID string) (interface{}, error) {
	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, errors.New("plugin not found")
	}
	return plugin, nil
}

func (m *mockPluginRuntime) addPlugin(pluginID string, permissions []string) *mockPluginProvider {
	plugin := &mockPluginProvider{
		id:          pluginID,
		permissions: permissions,
		tools:       make(map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)),
	}
	m.plugins[pluginID] = plugin
	return plugin
}

// mockPluginProvider implements both PluginToolProvider and PluginInfo interfaces
type mockPluginProvider struct {
	id          string
	permissions []string
	tools       map[string]func(context.Context, map[string]interface{}) (map[string]interface{}, error)
	execCount   int
	lastTool    string
	lastParams  map[string]interface{}
}

func (m *mockPluginProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	m.execCount++
	m.lastTool = name
	m.lastParams = params

	handler, exists := m.tools[name]
	if !exists {
		return nil, errors.New("tool not found in plugin")
	}

	return handler(ctx, params)
}

func (m *mockPluginProvider) GetID() string {
	return m.id
}

func (m *mockPluginProvider) GetPermissions() []string {
	return m.permissions
}

func (m *mockPluginProvider) addTool(name string, handler func(context.Context, map[string]interface{}) (map[string]interface{}, error)) {
	m.tools[name] = handler
}

// TestPluginToolDiscovery tests that plugin tools are discovered and registered correctly
func TestPluginToolDiscovery(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	// Add a plugin with a tool
	plugin := runtime.addPlugin("test-plugin", []string{"filesystem:read"})
	plugin.addTool("read_file", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "file content"}, nil
	})

	// Register the plugin tool
	toolDef := ToolDefinition{
		Name:        "read_file",
		Description: "Reads a file",
		Parameters: []ToolParameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "File path",
				Required:    true,
			},
		},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, runtime)
	require.NoError(t, err)

	// Verify tool is registered
	tool := te.GetTool("read_file")
	assert.NotNil(t, tool)
	assert.Equal(t, "read_file", tool.Name)
	assert.Equal(t, "test-plugin", tool.PluginID)
	assert.Contains(t, tool.Description, "test-plugin")
	assert.Equal(t, []string{"filesystem:read"}, tool.Permissions)

	// Verify tool count
	assert.Equal(t, 1, te.GetToolCount())
}

// TestPluginToolNameConflictResolution tests that tool name conflicts are resolved by prefixing
func TestPluginToolNameConflictResolution(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	// Register a regular tool first
	regularTool := ToolDefinition{
		Name:        "search",
		Description: "Regular search tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "regular result", nil
		},
	}
	err := te.RegisterTool(regularTool)
	require.NoError(t, err)

	// Add a plugin with a conflicting tool name
	plugin := runtime.addPlugin("search-plugin", []string{"database:read"})
	plugin.addTool("search", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"results": []string{"plugin result"}}, nil
	})

	// Register the plugin tool with conflicting name
	pluginTool := ToolDefinition{
		Name:        "search",
		Description: "Plugin search tool",
		Parameters:  []ToolParameter{},
	}

	err = te.RegisterPluginTool("search-plugin", pluginTool, runtime)
	require.NoError(t, err)

	// Verify both tools exist with different names
	regularToolResult := te.GetTool("search")
	assert.NotNil(t, regularToolResult)
	assert.Empty(t, regularToolResult.PluginID) // Regular tool has no plugin ID

	prefixedTool := te.GetTool("search-plugin_search")
	assert.NotNil(t, prefixedTool)
	assert.Equal(t, "search-plugin", prefixedTool.PluginID)
	assert.Contains(t, prefixedTool.Description, "search-plugin")

	// Verify tool count
	assert.Equal(t, 2, te.GetToolCount())

	// Execute both tools to verify they work independently
	regularResult := te.Execute(context.Background(), "search", map[string]interface{}{}, nil)
	assert.True(t, regularResult.Success)
	assert.Equal(t, "regular result", regularResult.Output)

	pluginResult := te.Execute(context.Background(), "search-plugin_search", map[string]interface{}{}, nil)
	assert.True(t, pluginResult.Success)
	assert.Equal(t, 1, plugin.execCount)
}

// TestPluginToolExecutionRouting tests that plugin tool calls are routed correctly
func TestPluginToolExecutionRouting(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	approvalManager := NewApprovalManager(&MockApprovalHandler{AutoApprove: true})
	te.SetApprovalManager(approvalManager)

	// Add a plugin with multiple tools
	plugin := runtime.addPlugin("data-plugin", []string{"database:read", "database:write"})

	plugin.addTool("query", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		query := params["query"].(string)
		return map[string]interface{}{
			"results": []string{"row1", "row2"},
			"query":   query,
		}, nil
	})

	plugin.addTool("insert", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"inserted": true,
			"id":       123,
		}, nil
	})

	// Register both tools
	queryTool := ToolDefinition{
		Name:        "query",
		Description: "Query database",
		Parameters: []ToolParameter{
			{Name: "query", Type: "string", Description: "SQL query", Required: true},
		},
	}
	err := te.RegisterPluginTool("data-plugin", queryTool, runtime)
	require.NoError(t, err)

	insertTool := ToolDefinition{
		Name:        "insert",
		Description: "Insert data",
		Parameters: []ToolParameter{
			{Name: "table", Type: "string", Description: "Table name", Required: true},
			{Name: "data", Type: "object", Description: "Data to insert", Required: true},
		},
	}
	err = te.RegisterPluginTool("data-plugin", insertTool, runtime)
	require.NoError(t, err)

	// Execute query tool
	queryResult := te.Execute(context.Background(), "query", map[string]interface{}{
		"query": "SELECT * FROM users",
	}, nil)

	assert.True(t, queryResult.Success)
	assert.Equal(t, 1, plugin.execCount)
	assert.Equal(t, "query", plugin.lastTool)
	assert.Equal(t, "SELECT * FROM users", plugin.lastParams["query"])

	resultMap := queryResult.Output.(map[string]interface{})
	assert.Equal(t, "SELECT * FROM users", resultMap["query"])
	assert.Len(t, resultMap["results"], 2)

	// Execute insert tool
	insertResult := te.Execute(context.Background(), "insert", map[string]interface{}{
		"table": "users",
		"data":  map[string]interface{}{"name": "John"},
	}, nil)

	assert.True(t, insertResult.Success)
	assert.Equal(t, 2, plugin.execCount)
	assert.Equal(t, "insert", plugin.lastTool)

	insertMap := insertResult.Output.(map[string]interface{})
	assert.True(t, insertMap["inserted"].(bool))
	assert.Equal(t, 123, insertMap["id"])
}

// TestPluginToolExecutionErrors tests error handling in plugin tool execution
func TestPluginToolExecutionErrors(t *testing.T) {
	tests := []struct {
		name          string
		setupPlugin   func(*mockPluginRuntime) string
		toolName      string
		params        map[string]interface{}
		expectedError string
	}{
		{
			name: "plugin not found",
			setupPlugin: func(runtime *mockPluginRuntime) string {
				// Register tool but plugin doesn't exist
				return "nonexistent-plugin"
			},
			toolName:      "missing_tool",
			params:        map[string]interface{}{},
			expectedError: "plugin not found",
		},
		{
			name: "tool execution failure",
			setupPlugin: func(runtime *mockPluginRuntime) string {
				plugin := runtime.addPlugin("failing-plugin", []string{})
				plugin.addTool("fail_tool", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
					return nil, errors.New("tool execution failed")
				})
				return "failing-plugin"
			},
			toolName:      "fail_tool",
			params:        map[string]interface{}{},
			expectedError: "tool execution failed",
		},
		{
			name: "context timeout",
			setupPlugin: func(runtime *mockPluginRuntime) string {
				plugin := runtime.addPlugin("slow-plugin", []string{})
				plugin.addTool("slow_tool", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
					time.Sleep(200 * time.Millisecond)
					return map[string]interface{}{"done": true}, nil
				})
				return "slow-plugin"
			},
			toolName:      "slow_tool",
			params:        map[string]interface{}{},
			expectedError: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := New()
			runtime := newMockPluginRuntime()
			pluginID := tt.setupPlugin(runtime)

			// Register the tool (even if plugin doesn't exist, to test runtime errors)
			toolDef := ToolDefinition{
				Name:        tt.toolName,
				Description: "Test tool",
				Parameters:  []ToolParameter{},
			}
			// Only register if plugin exists in runtime
			if _, err := runtime.GetPlugin(pluginID); err == nil {
				err := te.RegisterPluginTool(pluginID, toolDef, runtime)
				require.NoError(t, err)
			} else {
				// For plugin not found test, still register the tool
				// This will fail at execution time when trying to get the plugin
				err := te.RegisterPluginTool(pluginID, toolDef, runtime)
				require.NoError(t, err)
			}

			// Execute with timeout for slow test
			ctx := context.Background()
			execCtx := (*ExecutionContext)(nil)
			if tt.name == "context timeout" {
				execCtx = &ExecutionContext{
					Timeout: 50 * time.Millisecond,
				}
			}

			result := te.Execute(ctx, tt.toolName, tt.params, execCtx)

			assert.False(t, result.Success)
			assert.Contains(t, result.Error, tt.expectedError)
		})
	}
}

// TestPluginToolApprovalWorkflow tests that plugin tools with sensitive permissions require approval
func TestPluginToolApprovalWorkflow(t *testing.T) {
	tests := []struct {
		name            string
		permissions     []string
		expectsApproval bool
	}{
		{
			name:            "filesystem write requires approval",
			permissions:     []string{"filesystem:write"},
			expectsApproval: true,
		},
		{
			name:            "process spawn requires approval",
			permissions:     []string{"process:spawn"},
			expectsApproval: true,
		},
		{
			name:            "database write requires approval",
			permissions:     []string{"database:write"},
			expectsApproval: true,
		},
		{
			name:            "network http requires approval",
			permissions:     []string{"network:http"},
			expectsApproval: true,
		},
		{
			name:            "network websocket requires approval",
			permissions:     []string{"network:websocket"},
			expectsApproval: true,
		},
		{
			name:            "read-only permissions no approval",
			permissions:     []string{"filesystem:read", "database:read"},
			expectsApproval: false,
		},
		{
			name:            "mixed permissions requires approval",
			permissions:     []string{"filesystem:read", "filesystem:write"},
			expectsApproval: true,
		},
		{
			name:            "no permissions no approval",
			permissions:     []string{},
			expectsApproval: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := New()
			runtime := newMockPluginRuntime()

			// Add plugin with specified permissions
			plugin := runtime.addPlugin("test-plugin", tt.permissions)
			plugin.addTool("test_tool", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
				return map[string]interface{}{"success": true}, nil
			})

			// Register the plugin tool
			toolDef := ToolDefinition{
				Name:        "test_tool",
				Description: "Test tool",
				Parameters:  []ToolParameter{},
			}
			err := te.RegisterPluginTool("test-plugin", toolDef, runtime)
			require.NoError(t, err)

			// Get the registered tool
			tool := te.GetTool("test_tool")
			require.NotNil(t, tool)

			// Check if approval is required
			requiresApproval := te.requiresApproval(tool)
			assert.Equal(t, tt.expectsApproval, requiresApproval,
				"Expected approval requirement to be %v for permissions %v",
				tt.expectsApproval, tt.permissions)
		})
	}
}

// TestPluginToolContextPropagation tests that context is properly propagated to plugin tools
func TestPluginToolContextPropagation(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	contextReceived := false
	plugin := runtime.addPlugin("context-plugin", []string{})
	plugin.addTool("context_tool", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		// Verify context is not nil
		if ctx != nil {
			contextReceived = true
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return map[string]interface{}{"received": true}, nil
		}
	})

	toolDef := ToolDefinition{
		Name:        "context_tool",
		Description: "Tool that checks context",
		Parameters:  []ToolParameter{},
	}
	err := te.RegisterPluginTool("context-plugin", toolDef, runtime)
	require.NoError(t, err)

	// Execute with valid context
	result := te.Execute(context.Background(), "context_tool", map[string]interface{}{}, nil)
	assert.True(t, result.Success)
	assert.True(t, contextReceived)

	// Execute with cancelled context
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result = te.Execute(cancelledCtx, "context_tool", map[string]interface{}{}, nil)
	assert.False(t, result.Success)
	// The error could be either "context" or "timeout" depending on timing
	assert.True(t, result.Error != "", "Expected an error message")
}

// TestPluginToolParameterValidation tests that plugin tool parameters are validated
func TestPluginToolParameterValidation(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	plugin := runtime.addPlugin("validation-plugin", []string{})
	plugin.addTool("validate_tool", func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"validated": true}, nil
	})

	toolDef := ToolDefinition{
		Name:        "validate_tool",
		Description: "Tool with required parameters",
		Parameters: []ToolParameter{
			{
				Name:        "required_string",
				Type:        "string",
				Description: "Required string parameter",
				Required:    true,
			},
			{
				Name:        "required_number",
				Type:        "number",
				Description: "Required number parameter",
				Required:    true,
			},
			{
				Name:        "optional_bool",
				Type:        "boolean",
				Description: "Optional boolean parameter",
				Required:    false,
			},
		},
	}
	err := te.RegisterPluginTool("validation-plugin", toolDef, runtime)
	require.NoError(t, err)

	tests := []struct {
		name       string
		params     map[string]interface{}
		shouldFail bool
	}{
		{
			name: "valid parameters",
			params: map[string]interface{}{
				"required_string": "test",
				"required_number": 42.0,
				"optional_bool":   true,
			},
			shouldFail: false,
		},
		{
			name: "missing required parameter",
			params: map[string]interface{}{
				"required_string": "test",
			},
			shouldFail: true,
		},
		{
			name: "wrong parameter type",
			params: map[string]interface{}{
				"required_string": "test",
				"required_number": "not a number",
			},
			shouldFail: true,
		},
		{
			name: "optional parameter omitted",
			params: map[string]interface{}{
				"required_string": "test",
				"required_number": 42.0,
			},
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := te.Execute(context.Background(), "validate_tool", tt.params, nil)

			if tt.shouldFail {
				assert.False(t, result.Success)
				assert.Contains(t, result.Error, "validation")
			} else {
				assert.True(t, result.Success)
			}
		})
	}
}

// TestMultiplePluginToolRegistration tests registering tools from multiple plugins
func TestMultiplePluginToolRegistration(t *testing.T) {
	te := New()
	runtime := newMockPluginRuntime()

	approvalManager := NewApprovalManager(&MockApprovalHandler{AutoApprove: true})
	te.SetApprovalManager(approvalManager)

	// Register tools from multiple plugins
	plugins := []struct {
		id          string
		toolName    string
		permissions []string
	}{
		{"plugin-a", "tool_a", []string{"filesystem:read"}},
		{"plugin-b", "tool_b", []string{"database:read"}},
		{"plugin-c", "tool_c", []string{"network:http"}},
	}

	for _, p := range plugins {
		plugin := runtime.addPlugin(p.id, p.permissions)
		plugin.addTool(p.toolName, func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"plugin": p.id}, nil
		})

		toolDef := ToolDefinition{
			Name:        p.toolName,
			Description: "Tool from " + p.id,
			Parameters:  []ToolParameter{},
		}
		err := te.RegisterPluginTool(p.id, toolDef, runtime)
		require.NoError(t, err)
	}

	// Verify all tools are registered
	assert.Equal(t, 3, te.GetToolCount())

	// Verify each tool has correct plugin ID
	for _, p := range plugins {
		tool := te.GetTool(p.toolName)
		assert.NotNil(t, tool)
		assert.Equal(t, p.id, tool.PluginID)
		assert.Equal(t, p.permissions, tool.Permissions)
	}

	// Execute each tool and verify routing
	for _, p := range plugins {
		result := te.Execute(context.Background(), p.toolName, map[string]interface{}{}, nil)
		require.True(t, result.Success, "tool %s failed: %s", p.toolName, result.Error)

		resultMap := result.Output.(map[string]interface{})
		assert.Equal(t, p.id, resultMap["plugin"])
	}
}
