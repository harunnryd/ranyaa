package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog"
)

// PluginLoader loads and manages plugin processes
type PluginLoader struct {
	logger         zerolog.Logger
	manifestLoader *ManifestLoader
	toolRegistry   *ToolRegistry
	hookRegistry   *HookRegistry
	pluginRegistry *PluginRegistry
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(
	logger zerolog.Logger,
	manifestLoader *ManifestLoader,
	toolRegistry *ToolRegistry,
	hookRegistry *HookRegistry,
	pluginRegistry *PluginRegistry,
) *PluginLoader {
	return &PluginLoader{
		logger:         logger.With().Str("component", "plugin-loader").Logger(),
		manifestLoader: manifestLoader,
		toolRegistry:   toolRegistry,
		hookRegistry:   hookRegistry,
		pluginRegistry: pluginRegistry,
	}
}

// LoadPlugin loads a plugin from a discovered plugin
func (l *PluginLoader) LoadPlugin(discovered DiscoveredPlugin, config map[string]any) (*LoadedPlugin, error) {
	// Load manifest
	manifest, err := l.manifestLoader.LoadManifest(discovered.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	// Create sandbox context
	sandbox := NewSandboxContext(manifest.Permissions)

	// Get plugin executable path
	pluginPath := filepath.Join(discovered.Path, manifest.Main)

	// Check if plugin executable exists
	if _, err := os.Stat(pluginPath); err != nil {
		return nil, fmt.Errorf("plugin executable not found: %s", pluginPath)
	}

	// Create plugin client
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          PluginMap,
		Cmd:              exec.Command(pluginPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolNetRPC},
	})

	// Connect to plugin
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Dispense plugin
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	pluginClient, ok := raw.(PluginClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("unexpected plugin type")
	}

	// Create loaded plugin
	loaded := &LoadedPlugin{
		ID:       manifest.ID,
		Manifest: *manifest,
		State:    StateLoading,
		Sandbox:  sandbox,
		Client:   pluginClient,
		Config:   config,
	}

	// Create plugin API
	api := NewPluginAPI(
		manifest.ID,
		config,
		sandbox,
		l.toolRegistry,
		l.hookRegistry,
		l.logger,
	)

	// Activate plugin
	if err := pluginClient.Activate(nil, api, config); err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to activate plugin: %w", err)
	}

	loaded.State = StateEnabled

	l.logger.Info().
		Str("id", manifest.ID).
		Str("version", manifest.Version).
		Msg("Plugin loaded successfully")

	return loaded, nil
}

// UnloadPlugin unloads a plugin
func (l *PluginLoader) UnloadPlugin(pluginID string) error {
	record, exists := l.pluginRegistry.Get(pluginID)
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	// Deactivate plugin
	if record.Plugin.Client != nil {
		if err := record.Plugin.Client.Deactivate(nil); err != nil {
			l.logger.Warn().Err(err).Str("plugin", pluginID).Msg("Failed to deactivate plugin")
		}
	}

	// Remove registered tools
	removed := l.toolRegistry.UnregisterByPlugin(pluginID)
	l.logger.Debug().Str("plugin", pluginID).Strs("tools", removed).Msg("Removed tools")

	// Remove registered hooks
	removedHooks := l.hookRegistry.UnregisterByPlugin(pluginID)
	l.logger.Debug().Str("plugin", pluginID).Strs("hooks", removedHooks).Msg("Removed hooks")

	// Remove from registry
	if err := l.pluginRegistry.Remove(pluginID); err != nil {
		return fmt.Errorf("failed to remove plugin from registry: %w", err)
	}

	l.logger.Info().Str("id", pluginID).Msg("Plugin unloaded successfully")
	return nil
}

// ReloadPlugin reloads a plugin
func (l *PluginLoader) ReloadPlugin(pluginID string, discovered DiscoveredPlugin, config map[string]any) error {
	// Unload existing plugin
	if err := l.UnloadPlugin(pluginID); err != nil {
		return fmt.Errorf("failed to unload plugin: %w", err)
	}

	// Load plugin again
	loaded, err := l.LoadPlugin(discovered, config)
	if err != nil {
		return fmt.Errorf("failed to reload plugin: %w", err)
	}

	// Register in registry
	if err := l.pluginRegistry.Register(loaded); err != nil {
		return fmt.Errorf("failed to register reloaded plugin: %w", err)
	}

	// Record reload
	if err := l.pluginRegistry.RecordReload(pluginID); err != nil {
		l.logger.Warn().Err(err).Msg("Failed to record reload")
	}

	l.logger.Info().Str("id", pluginID).Msg("Plugin reloaded successfully")
	return nil
}
