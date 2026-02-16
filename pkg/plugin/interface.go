package plugin

import (
	"context"
)

// Plugin is the interface that plugins must implement
// This is used by HashiCorp go-plugin for RPC communication
type Plugin interface {
	// Activate is called when the plugin is loaded
	// It receives the plugin API and configuration
	Activate(ctx context.Context, api PluginAPI, config map[string]any) error

	// Deactivate is called when the plugin is unloaded
	Deactivate(ctx context.Context) error

	// ExecuteTool executes a registered tool
	ExecuteTool(ctx context.Context, name string, params map[string]any) (map[string]any, error)

	// ExecuteHook executes a registered hook
	ExecuteHook(ctx context.Context, event HookEvent) error

	// ExecuteGatewayMethod executes a registered gateway method
	ExecuteGatewayMethod(ctx context.Context, name string, params map[string]any) (map[string]any, error)
}

// PluginAPI is the interface provided to plugins for registering extensions
type PluginAPI interface {
	// RegisterTool registers a new tool
	RegisterTool(ctx context.Context, definition ToolDefinition) error

	// RegisterHook registers a new hook
	RegisterHook(ctx context.Context, definition HookDefinition) error

	// RegisterChannel registers a new messaging channel
	RegisterChannel(ctx context.Context, definition ChannelDefinition) error

	// RegisterProvider registers a new service provider
	RegisterProvider(ctx context.Context, definition ProviderDefinition) error

	// RegisterGatewayMethod registers a new gateway method
	RegisterGatewayMethod(ctx context.Context, definition GatewayMethodDefinition) error

	// GetConfig returns the plugin's configuration
	GetConfig(ctx context.Context) (map[string]any, error)

	// GetPluginID returns the plugin's ID
	GetPluginID(ctx context.Context) (string, error)
}

// PluginClient is the client-side interface for communicating with a plugin
type PluginClient interface {
	Plugin
}
