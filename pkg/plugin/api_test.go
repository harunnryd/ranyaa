package plugin

import (
	"context"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginAPI_RegisterTool(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()
	sandbox := NewSandboxContext([]Permission{})

	api := NewPluginAPI("test-plugin", nil, sandbox, toolRegistry, hookRegistry, logger)

	t.Run("registers valid tool", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "test-tool",
			Description: "A test tool",
			Parameters:  map[string]any{"type": "object"},
		}

		err := api.RegisterTool(context.Background(), def)
		require.NoError(t, err)

		tool, exists := toolRegistry.Get("test-tool")
		assert.True(t, exists)
		assert.Equal(t, "test-plugin", tool.PluginID)
	})

	t.Run("rejects tool with empty name", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "",
			Description: "A test tool",
			Parameters:  map[string]any{},
		}

		err := api.RegisterTool(context.Background(), def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name cannot be empty")
	})

	t.Run("rejects tool with empty description", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "test-tool-2",
			Description: "",
			Parameters:  map[string]any{},
		}

		err := api.RegisterTool(context.Background(), def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "description cannot be empty")
	})

	t.Run("rejects tool with nil parameters", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "test-tool-3",
			Description: "A test tool",
			Parameters:  nil,
		}

		err := api.RegisterTool(context.Background(), def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parameters cannot be nil")
	})
}

func TestPluginAPI_RegisterHook(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()
	sandbox := NewSandboxContext([]Permission{})

	api := NewPluginAPI("test-plugin", nil, sandbox, toolRegistry, hookRegistry, logger)

	t.Run("registers valid hook", func(t *testing.T) {
		def := HookDefinition{
			Event: "test-event",
		}

		err := api.RegisterHook(context.Background(), def)
		require.NoError(t, err)

		hooks := hookRegistry.GetHooks("test-event")
		assert.Len(t, hooks, 1)
		assert.Equal(t, "test-plugin", hooks[0].PluginID)
	})

	t.Run("rejects hook with empty event", func(t *testing.T) {
		def := HookDefinition{
			Event: "",
		}

		err := api.RegisterHook(context.Background(), def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be empty")
	})
}

func TestPluginAPI_RegisterGatewayMethod(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()

	t.Run("requires gateway:register permission", func(t *testing.T) {
		sandbox := NewSandboxContext([]Permission{})
		api := NewPluginAPI("test-plugin", nil, sandbox, toolRegistry, hookRegistry, logger)

		def := GatewayMethodDefinition{
			Name: "test-method",
		}

		err := api.RegisterGatewayMethod(context.Background(), def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("registers with permission", func(t *testing.T) {
		sandbox := NewSandboxContext([]Permission{PermissionGatewayRegister})
		api := NewPluginAPI("test-plugin", nil, sandbox, toolRegistry, hookRegistry, logger)

		def := GatewayMethodDefinition{
			Name: "test-method",
		}

		err := api.RegisterGatewayMethod(context.Background(), def)
		assert.NoError(t, err)
	})
}

func TestPluginAPI_GetConfig(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()
	sandbox := NewSandboxContext([]Permission{})

	config := map[string]any{
		"key": "value",
	}

	api := NewPluginAPI("test-plugin", config, sandbox, toolRegistry, hookRegistry, logger)

	result, err := api.GetConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, config, result)
}

func TestPluginAPI_GetPluginID(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()
	sandbox := NewSandboxContext([]Permission{})

	api := NewPluginAPI("test-plugin", nil, sandbox, toolRegistry, hookRegistry, logger)

	id, err := api.GetPluginID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", id)
}

func TestToolRegistry(t *testing.T) {
	registry := NewToolRegistry()

	t.Run("registers and retrieves tool", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "tool1",
			Description: "Tool 1",
			Parameters:  map[string]any{},
		}

		err := registry.Register("plugin1", def)
		require.NoError(t, err)

		tool, exists := registry.Get("tool1")
		assert.True(t, exists)
		assert.Equal(t, "plugin1", tool.PluginID)
	})

	t.Run("rejects duplicate tool name", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "tool2",
			Description: "Tool 2",
			Parameters:  map[string]any{},
		}

		err := registry.Register("plugin1", def)
		require.NoError(t, err)

		err = registry.Register("plugin2", def)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("unregisters tool", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "tool3",
			Description: "Tool 3",
			Parameters:  map[string]any{},
		}

		err := registry.Register("plugin1", def)
		require.NoError(t, err)

		registry.Unregister("tool3")

		_, exists := registry.Get("tool3")
		assert.False(t, exists)
	})

	t.Run("unregisters by plugin", func(t *testing.T) {
		def1 := ToolDefinition{Name: "tool4", Description: "Tool 4", Parameters: map[string]any{}}
		def2 := ToolDefinition{Name: "tool5", Description: "Tool 5", Parameters: map[string]any{}}

		registry.Register("plugin2", def1)
		registry.Register("plugin2", def2)

		removed := registry.UnregisterByPlugin("plugin2")
		assert.Len(t, removed, 2)

		_, exists := registry.Get("tool4")
		assert.False(t, exists)
	})
}

func TestHookRegistry(t *testing.T) {
	registry := NewHookRegistry()

	t.Run("registers and retrieves hooks", func(t *testing.T) {
		def := HookDefinition{Event: "event1"}

		err := registry.Register("plugin1", def)
		require.NoError(t, err)

		hooks := registry.GetHooks("event1")
		assert.Len(t, hooks, 1)
		assert.Equal(t, "plugin1", hooks[0].PluginID)
	})

	t.Run("allows multiple hooks for same event", func(t *testing.T) {
		def := HookDefinition{Event: "event2"}

		registry.Register("plugin1", def)
		registry.Register("plugin2", def)

		hooks := registry.GetHooks("event2")
		assert.Len(t, hooks, 2)
	})

	t.Run("unregisters by plugin", func(t *testing.T) {
		def := HookDefinition{Event: "event3"}

		registry.Register("plugin3", def)
		registry.Register("plugin3", def)

		removed := registry.UnregisterByPlugin("plugin3")
		assert.Len(t, removed, 2)

		hooks := registry.GetHooks("event3")
		assert.Empty(t, hooks)
	})
}
