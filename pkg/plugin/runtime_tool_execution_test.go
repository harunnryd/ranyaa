package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockPluginClient implements PluginClient for testing
type mockPluginClient struct {
	executeToolFunc func(ctx context.Context, name string, params map[string]any) (map[string]any, error)
}

func (m *mockPluginClient) Activate(ctx context.Context, api PluginAPI, config map[string]any) error {
	return nil
}

func (m *mockPluginClient) Deactivate(ctx context.Context) error {
	return nil
}

func (m *mockPluginClient) GetTools(ctx context.Context) ([]ToolDefinition, error) {
	return []ToolDefinition{}, nil
}

func (m *mockPluginClient) ExecuteTool(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	if m.executeToolFunc != nil {
		return m.executeToolFunc(ctx, name, params)
	}
	return map[string]any{"result": "success"}, nil
}

func (m *mockPluginClient) ExecuteHook(ctx context.Context, event HookEvent) error {
	return nil
}

func (m *mockPluginClient) ExecuteGatewayMethod(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	return nil, nil
}

func TestGetPluginForTool_Success(t *testing.T) {
	logger := zerolog.Nop()
	runtime := NewPluginRuntime(logger, PluginRuntimeConfig{})

	// Create a loaded plugin
	loadedPlugin := &LoadedPlugin{
		ID: "test-plugin",
		Manifest: PluginManifest{
			ID:      "test-plugin",
			Name:    "Test Plugin",
			Version: "1.0.0",
		},
		State:  StateEnabled,
		Client: &mockPluginClient{},
	}

	// Register the plugin
	err := runtime.registry.Register(loadedPlugin)
	if err != nil {
		t.Fatalf("Failed to register plugin: %v", err)
	}

	// Get plugin for tool execution
	adapter, err := runtime.GetPluginForTool("test-plugin")
	if err != nil {
		t.Fatalf("Failed to get plugin for tool: %v", err)
	}

	if adapter == nil {
		t.Fatal("Expected adapter, got nil")
	}

	if adapter.plugin.ID != "test-plugin" {
		t.Errorf("Expected plugin ID 'test-plugin', got '%s'", adapter.plugin.ID)
	}
}

func TestGetPluginForTool_NotFound(t *testing.T) {
	logger := zerolog.Nop()
	runtime := NewPluginRuntime(logger, PluginRuntimeConfig{})

	// Try to get non-existent plugin
	_, err := runtime.GetPluginForTool("non-existent")
	if err == nil {
		t.Fatal("Expected error for non-existent plugin")
	}

	expectedMsg := "plugin non-existent not found in registry"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestGetPluginForTool_NotEnabled(t *testing.T) {
	logger := zerolog.Nop()
	runtime := NewPluginRuntime(logger, PluginRuntimeConfig{})

	// Create a disabled plugin
	loadedPlugin := &LoadedPlugin{
		ID: "disabled-plugin",
		Manifest: PluginManifest{
			ID:      "disabled-plugin",
			Name:    "Disabled Plugin",
			Version: "1.0.0",
		},
		State:  StateDisabled,
		Client: &mockPluginClient{},
	}

	// Register the plugin
	err := runtime.registry.Register(loadedPlugin)
	if err != nil {
		t.Fatalf("Failed to register plugin: %v", err)
	}

	// Try to get disabled plugin
	_, err = runtime.GetPluginForTool("disabled-plugin")
	if err == nil {
		t.Fatal("Expected error for disabled plugin")
	}

	if err.Error() != "plugin disabled-plugin is not enabled (current state: disabled)" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}

func TestGetPluginForTool_NoClient(t *testing.T) {
	logger := zerolog.Nop()
	runtime := NewPluginRuntime(logger, PluginRuntimeConfig{})

	// Create a plugin without client
	loadedPlugin := &LoadedPlugin{
		ID: "no-client-plugin",
		Manifest: PluginManifest{
			ID:      "no-client-plugin",
			Name:    "No Client Plugin",
			Version: "1.0.0",
		},
		State:  StateEnabled,
		Client: nil, // No client
	}

	// Register the plugin
	err := runtime.registry.Register(loadedPlugin)
	if err != nil {
		t.Fatalf("Failed to register plugin: %v", err)
	}

	// Try to get plugin without client
	_, err = runtime.GetPluginForTool("no-client-plugin")
	if err == nil {
		t.Fatal("Expected error for plugin without client")
	}

	expectedMsg := "plugin no-client-plugin has no active client connection"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestPluginToolAdapter_ExecuteTool_Success(t *testing.T) {
	logger := zerolog.Nop()

	expectedResult := map[string]any{
		"status": "completed",
		"data":   "test data",
	}

	mockClient := &mockPluginClient{
		executeToolFunc: func(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
			if name != "test_tool" {
				t.Errorf("Expected tool name 'test_tool', got '%s'", name)
			}
			return expectedResult, nil
		},
	}

	loadedPlugin := &LoadedPlugin{
		ID:     "test-plugin",
		Client: mockClient,
	}

	adapter := &pluginToolAdapter{
		plugin: loadedPlugin,
		logger: logger,
	}

	ctx := context.Background()
	params := map[string]interface{}{
		"input": "test",
	}

	result, err := adapter.ExecuteTool(ctx, "test_tool", params)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	if result["status"] != "completed" {
		t.Errorf("Expected status 'completed', got '%v'", result["status"])
	}

	if result["data"] != "test data" {
		t.Errorf("Expected data 'test data', got '%v'", result["data"])
	}
}

func TestPluginToolAdapter_ExecuteTool_Error(t *testing.T) {
	logger := zerolog.Nop()

	expectedError := errors.New("tool execution failed")

	mockClient := &mockPluginClient{
		executeToolFunc: func(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
			return nil, expectedError
		},
	}

	loadedPlugin := &LoadedPlugin{
		ID:     "test-plugin",
		Client: mockClient,
	}

	adapter := &pluginToolAdapter{
		plugin: loadedPlugin,
		logger: logger,
	}

	ctx := context.Background()
	params := map[string]interface{}{}

	_, err := adapter.ExecuteTool(ctx, "test_tool", params)
	if err == nil {
		t.Fatal("Expected error from ExecuteTool")
	}

	expectedMsg := "plugin test-plugin tool test_tool execution failed: tool execution failed"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestPluginToolAdapter_ExecuteTool_ContextCancelled(t *testing.T) {
	logger := zerolog.Nop()

	mockClient := &mockPluginClient{
		executeToolFunc: func(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
			return map[string]any{"result": "ok"}, nil
		},
	}

	loadedPlugin := &LoadedPlugin{
		ID:     "test-plugin",
		Client: mockClient,
	}

	adapter := &pluginToolAdapter{
		plugin: loadedPlugin,
		logger: logger,
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	params := map[string]interface{}{}

	_, err := adapter.ExecuteTool(ctx, "test_tool", params)
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}

	expectedMsg := "context error before execution: context canceled"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestPluginToolAdapter_ExecuteTool_ContextTimeout(t *testing.T) {
	logger := zerolog.Nop()

	mockClient := &mockPluginClient{
		executeToolFunc: func(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
			// Simulate slow operation
			select {
			case <-time.After(200 * time.Millisecond):
				return map[string]any{"result": "ok"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	loadedPlugin := &LoadedPlugin{
		ID:     "test-plugin",
		Client: mockClient,
	}

	adapter := &pluginToolAdapter{
		plugin: loadedPlugin,
		logger: logger,
	}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	params := map[string]interface{}{}

	_, err := adapter.ExecuteTool(ctx, "test_tool", params)
	if err == nil {
		t.Fatal("Expected timeout error")
	}

	// Error should contain timeout information
	if err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestPluginToolAdapter_ExecuteTool_ParameterPassing(t *testing.T) {
	logger := zerolog.Nop()

	var receivedParams map[string]any

	mockClient := &mockPluginClient{
		executeToolFunc: func(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
			receivedParams = params
			return map[string]any{"result": "ok"}, nil
		},
	}

	loadedPlugin := &LoadedPlugin{
		ID:     "test-plugin",
		Client: mockClient,
	}

	adapter := &pluginToolAdapter{
		plugin: loadedPlugin,
		logger: logger,
	}

	ctx := context.Background()
	params := map[string]interface{}{
		"input":  "test value",
		"count":  42,
		"enable": true,
	}

	_, err := adapter.ExecuteTool(ctx, "test_tool", params)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	// Verify parameters were passed correctly
	if receivedParams["input"] != "test value" {
		t.Errorf("Expected input 'test value', got '%v'", receivedParams["input"])
	}

	if receivedParams["count"] != 42 {
		t.Errorf("Expected count 42, got '%v'", receivedParams["count"])
	}

	if receivedParams["enable"] != true {
		t.Errorf("Expected enable true, got '%v'", receivedParams["enable"])
	}
}
