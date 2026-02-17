package toolexecutor

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockPluginToolExecutor implements PluginToolExecutor for testing
type mockPluginToolExecutor struct {
	plugins map[string]PluginToolProvider
	getErr  error
}

func (m *mockPluginToolExecutor) GetPlugin(pluginID string) (interface{}, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, errors.New("plugin not found")
	}
	return plugin, nil
}

// mockPluginToolProvider implements PluginToolProvider for testing
type mockPluginToolProvider struct {
	executeFunc func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error)
}

func (m *mockPluginToolProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, name, params)
	}
	return map[string]interface{}{"result": "success"}, nil
}

func TestRegisterPluginTool_Success(t *testing.T) {
	te := New()

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{},
		},
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: []ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Test input",
				Required:    true,
			},
		},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Verify tool is registered
	registeredTool := te.GetTool("test_tool")
	if registeredTool == nil {
		t.Fatal("Tool not found after registration")
	}

	if registeredTool.Name != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", registeredTool.Name)
	}
}

func TestRegisterPluginTool_NameConflict(t *testing.T) {
	te := New()

	// Register a regular tool first
	regularTool := ToolDefinition{
		Name:        "conflicting_tool",
		Description: "Regular tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "regular", nil
		},
	}

	err := te.RegisterTool(regularTool)
	if err != nil {
		t.Fatalf("Failed to register regular tool: %v", err)
	}

	// Try to register plugin tool with same name
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{},
		},
	}

	pluginTool := ToolDefinition{
		Name:        "conflicting_tool",
		Description: "Plugin tool",
		Parameters:  []ToolParameter{},
	}

	err = te.RegisterPluginTool("test-plugin", pluginTool, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Verify plugin tool is registered with prefixed name
	prefixedTool := te.GetTool("test-plugin_conflicting_tool")
	if prefixedTool == nil {
		t.Fatal("Prefixed plugin tool not found after registration")
	}

	// Verify original tool still exists
	originalTool := te.GetTool("conflicting_tool")
	if originalTool == nil {
		t.Fatal("Original tool was overwritten")
	}
}

func TestPluginToolExecution_Success(t *testing.T) {
	te := New()

	expectedResult := map[string]interface{}{
		"status": "completed",
		"data":   "test data",
	}

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					if name != "test_tool" {
						t.Errorf("Expected tool name 'test_tool', got '%s'", name)
					}
					return expectedResult, nil
				},
			},
		},
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	result := te.Execute(ctx, "test_tool", map[string]interface{}{}, nil)

	if !result.Success {
		t.Fatalf("Tool execution failed: %s", result.Error)
	}

	resultMap, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T", result.Output)
	}

	if resultMap["status"] != "completed" {
		t.Errorf("Expected status 'completed', got '%v'", resultMap["status"])
	}
}

func TestPluginToolExecution_PluginNotFound(t *testing.T) {
	te := New()

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{},
		getErr:  errors.New("plugin not found in registry"),
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("missing-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	result := te.Execute(ctx, "test_tool", map[string]interface{}{}, nil)

	if result.Success {
		t.Fatal("Expected tool execution to fail when plugin not found")
	}

	if result.Error == "" {
		t.Error("Expected error message when plugin not found")
	}
}

func TestPluginToolExecution_ExecutionError(t *testing.T) {
	te := New()

	expectedError := errors.New("plugin execution failed")

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					return nil, expectedError
				},
			},
		},
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	result := te.Execute(ctx, "test_tool", map[string]interface{}{}, nil)

	if result.Success {
		t.Fatal("Expected tool execution to fail")
	}

	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestPluginToolExecution_Timeout(t *testing.T) {
	te := New()

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					// Simulate long-running operation
					select {
					case <-time.After(2 * time.Second):
						return map[string]interface{}{"result": "completed"}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
			},
		},
	}

	toolDef := ToolDefinition{
		Name:        "slow_tool",
		Description: "A slow tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute with short timeout
	ctx := context.Background()
	execCtx := &ExecutionContext{
		Timeout: 100 * time.Millisecond,
	}

	result := te.Execute(ctx, "slow_tool", map[string]interface{}{}, execCtx)

	if result.Success {
		t.Fatal("Expected tool execution to timeout")
	}

	if result.Error == "" {
		t.Error("Expected timeout error message")
	}
}

func TestPluginToolExecution_ContextCancellation(t *testing.T) {
	te := New()

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					// Check if context is cancelled
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return map[string]interface{}{"result": "completed"}, nil
				},
			},
		},
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := te.Execute(ctx, "test_tool", map[string]interface{}{}, nil)

	if result.Success {
		t.Fatal("Expected tool execution to fail with cancelled context")
	}
}

func TestPluginToolExecution_ParameterPassing(t *testing.T) {
	te := New()

	receivedParams := make(map[string]interface{})

	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginToolProvider{
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					// Capture received parameters
					for k, v := range params {
						receivedParams[k] = v
					}
					return map[string]interface{}{"result": "ok"}, nil
				},
			},
		},
	}

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: []ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Test input",
				Required:    true,
			},
			{
				Name:        "count",
				Type:        "number",
				Description: "Test count",
				Required:    false,
			},
		},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute with parameters
	ctx := context.Background()
	params := map[string]interface{}{
		"input": "test value",
		"count": 42.0,
	}

	result := te.Execute(ctx, "test_tool", params, nil)

	if !result.Success {
		t.Fatalf("Tool execution failed: %s", result.Error)
	}

	// Verify parameters were passed correctly
	if receivedParams["input"] != "test value" {
		t.Errorf("Expected input 'test value', got '%v'", receivedParams["input"])
	}

	if receivedParams["count"] != 42.0 {
		t.Errorf("Expected count 42.0, got '%v'", receivedParams["count"])
	}
}
