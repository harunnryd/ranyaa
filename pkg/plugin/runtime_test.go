package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

func TestPluginRuntime_ListPlugins(t *testing.T) {
	logger := zerolog.Nop()

	tests := []struct {
		name          string
		setupPlugins  func(*PluginRuntime)
		expectedCount int
		expectedTools map[string]int // pluginID -> tool count
	}{
		{
			name: "empty runtime",
			setupPlugins: func(r *PluginRuntime) {
				// No plugins
			},
			expectedCount: 0,
			expectedTools: map[string]int{},
		},
		{
			name: "single plugin with tools",
			setupPlugins: func(r *PluginRuntime) {
				// Create a loaded plugin
				plugin := &LoadedPlugin{
					ID: "test-plugin",
					Manifest: PluginManifest{
						ID:      "test-plugin",
						Name:    "Test Plugin",
						Version: "1.0.0",
					},
					State: StateEnabled,
				}

				// Register plugin
				_ = r.registry.Register(plugin)

				// Register tools
				_ = r.toolRegistry.Register("test-plugin", ToolDefinition{
					Name:        "test_tool_1",
					Description: "Test tool 1",
					Parameters:  map[string]any{},
				})
				_ = r.toolRegistry.Register("test-plugin", ToolDefinition{
					Name:        "test_tool_2",
					Description: "Test tool 2",
					Parameters:  map[string]any{},
				})
			},
			expectedCount: 1,
			expectedTools: map[string]int{
				"test-plugin": 2,
			},
		},
		{
			name: "multiple plugins with different tool counts",
			setupPlugins: func(r *PluginRuntime) {
				// Plugin 1 with 2 tools
				plugin1 := &LoadedPlugin{
					ID: "plugin-1",
					Manifest: PluginManifest{
						ID:      "plugin-1",
						Name:    "Plugin 1",
						Version: "1.0.0",
					},
					State: StateEnabled,
				}
				_ = r.registry.Register(plugin1)
				_ = r.toolRegistry.Register("plugin-1", ToolDefinition{
					Name:        "plugin1_tool1",
					Description: "Plugin 1 Tool 1",
					Parameters:  map[string]any{},
				})
				_ = r.toolRegistry.Register("plugin-1", ToolDefinition{
					Name:        "plugin1_tool2",
					Description: "Plugin 1 Tool 2",
					Parameters:  map[string]any{},
				})

				// Plugin 2 with 1 tool
				plugin2 := &LoadedPlugin{
					ID: "plugin-2",
					Manifest: PluginManifest{
						ID:      "plugin-2",
						Name:    "Plugin 2",
						Version: "2.0.0",
					},
					State: StateEnabled,
				}
				_ = r.registry.Register(plugin2)
				_ = r.toolRegistry.Register("plugin-2", ToolDefinition{
					Name:        "plugin2_tool1",
					Description: "Plugin 2 Tool 1",
					Parameters:  map[string]any{},
				})

				// Plugin 3 with no tools
				plugin3 := &LoadedPlugin{
					ID: "plugin-3",
					Manifest: PluginManifest{
						ID:      "plugin-3",
						Name:    "Plugin 3",
						Version: "3.0.0",
					},
					State: StateDisabled,
				}
				_ = r.registry.Register(plugin3)
			},
			expectedCount: 3,
			expectedTools: map[string]int{
				"plugin-1": 2,
				"plugin-2": 1,
				"plugin-3": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := NewPluginRuntime(logger, PluginRuntimeConfig{})
			tt.setupPlugins(runtime)

			plugins := runtime.ListPlugins()

			if len(plugins) != tt.expectedCount {
				t.Errorf("expected %d plugins, got %d", tt.expectedCount, len(plugins))
			}

			for _, plugin := range plugins {
				expectedToolCount, exists := tt.expectedTools[plugin.ID]
				if !exists {
					t.Errorf("unexpected plugin: %s", plugin.ID)
					continue
				}

				if len(plugin.Tools) != expectedToolCount {
					t.Errorf("plugin %s: expected %d tools, got %d", plugin.ID, expectedToolCount, len(plugin.Tools))
				}

				// Verify plugin metadata is populated
				if plugin.ID == "" {
					t.Error("plugin ID should not be empty")
				}
				if plugin.Manifest.Name == "" {
					t.Error("plugin manifest name should not be empty")
				}
				if plugin.LoadedAt.IsZero() {
					t.Error("plugin LoadedAt should not be zero")
				}
			}
		})
	}
}

func TestToolRegistry_GetByPlugin(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools for different plugins
	_ = registry.Register("plugin-1", ToolDefinition{
		Name:        "tool1",
		Description: "Tool 1",
		Parameters:  map[string]any{},
	})
	_ = registry.Register("plugin-1", ToolDefinition{
		Name:        "tool2",
		Description: "Tool 2",
		Parameters:  map[string]any{},
	})
	_ = registry.Register("plugin-2", ToolDefinition{
		Name:        "tool3",
		Description: "Tool 3",
		Parameters:  map[string]any{},
	})

	tests := []struct {
		name          string
		pluginID      string
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "plugin with multiple tools",
			pluginID:      "plugin-1",
			expectedCount: 2,
			expectedNames: []string{"tool1", "tool2"},
		},
		{
			name:          "plugin with single tool",
			pluginID:      "plugin-2",
			expectedCount: 1,
			expectedNames: []string{"tool3"},
		},
		{
			name:          "plugin with no tools",
			pluginID:      "plugin-3",
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := registry.GetByPlugin(tt.pluginID)

			if len(tools) != tt.expectedCount {
				t.Errorf("expected %d tools, got %d", tt.expectedCount, len(tools))
			}

			// Verify tool names
			toolNames := make(map[string]bool)
			for _, tool := range tools {
				toolNames[tool.Name] = true
			}

			for _, expectedName := range tt.expectedNames {
				if !toolNames[expectedName] {
					t.Errorf("expected tool %s not found", expectedName)
				}
			}
		})
	}
}

type fakePluginClient struct {
	pingErr   error
	pingCount int
}

func (f *fakePluginClient) Activate(context.Context, PluginAPI, map[string]any) error { return nil }
func (f *fakePluginClient) Deactivate(context.Context) error                          { return nil }
func (f *fakePluginClient) GetTools(context.Context) ([]ToolDefinition, error)        { return nil, nil }
func (f *fakePluginClient) ExecuteTool(context.Context, string, map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}
func (f *fakePluginClient) ExecuteHook(context.Context, HookEvent) error { return nil }
func (f *fakePluginClient) ExecuteGatewayMethod(context.Context, string, map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}
func (f *fakePluginClient) Ping(context.Context) error {
	f.pingCount++
	return f.pingErr
}

func TestPluginRuntime_CheckHealth(t *testing.T) {
	runtime := NewPluginRuntime(zerolog.Nop(), PluginRuntimeConfig{})
	client := &fakePluginClient{pingErr: errors.New("boom")}

	plugin := &LoadedPlugin{
		ID:       "test-plugin",
		State:    StateEnabled,
		Client:   client,
		Manifest: PluginManifest{ID: "test-plugin", Name: "Test Plugin", Version: "1.0.0"},
	}

	if err := runtime.registry.Register(plugin); err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}

	if err := runtime.CheckHealth(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	record, ok := runtime.registry.Get("test-plugin")
	if !ok {
		t.Fatalf("plugin record missing")
	}
	if client.pingCount != 1 {
		t.Fatalf("expected ping count 1, got %d", client.pingCount)
	}
	if record.LastError == nil {
		t.Fatalf("expected health check error to be recorded")
	}
}

func TestToolRegistry_GetAll(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools
	_ = registry.Register("plugin-1", ToolDefinition{
		Name:        "tool1",
		Description: "Tool 1",
		Parameters:  map[string]any{},
	})
	_ = registry.Register("plugin-2", ToolDefinition{
		Name:        "tool2",
		Description: "Tool 2",
		Parameters:  map[string]any{},
	})

	tools := registry.GetAll()

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify tools have correct plugin IDs
	pluginIDs := make(map[string]bool)
	for _, tool := range tools {
		pluginIDs[tool.PluginID] = true
	}

	if !pluginIDs["plugin-1"] || !pluginIDs["plugin-2"] {
		t.Error("expected both plugin-1 and plugin-2 in results")
	}
}

// Mock plugin implementation for testing GetTools
type mockPlugin struct {
	tools []ToolDefinition
}

func (m *mockPlugin) Activate(ctx context.Context, api PluginAPI, config map[string]any) error {
	return nil
}

func (m *mockPlugin) Deactivate(ctx context.Context) error {
	return nil
}

func (m *mockPlugin) GetTools(ctx context.Context) ([]ToolDefinition, error) {
	return m.tools, nil
}

func (m *mockPlugin) ExecuteTool(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	return nil, nil
}

func (m *mockPlugin) ExecuteHook(ctx context.Context, event HookEvent) error {
	return nil
}

func (m *mockPlugin) ExecuteGatewayMethod(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	return nil, nil
}

func TestPlugin_GetTools(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		tools         []ToolDefinition
		expectedCount int
	}{
		{
			name:          "no tools",
			tools:         []ToolDefinition{},
			expectedCount: 0,
		},
		{
			name: "single tool",
			tools: []ToolDefinition{
				{
					Name:        "test_tool",
					Description: "Test tool",
					Parameters:  map[string]any{},
				},
			},
			expectedCount: 1,
		},
		{
			name: "multiple tools",
			tools: []ToolDefinition{
				{
					Name:        "tool1",
					Description: "Tool 1",
					Parameters:  map[string]any{},
				},
				{
					Name:        "tool2",
					Description: "Tool 2",
					Parameters:  map[string]any{},
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &mockPlugin{tools: tt.tools}

			tools, err := plugin.GetTools(ctx)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(tools) != tt.expectedCount {
				t.Errorf("expected %d tools, got %d", tt.expectedCount, len(tools))
			}
		})
	}
}
