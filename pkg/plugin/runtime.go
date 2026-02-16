package plugin

import (
	"fmt"

	"github.com/rs/zerolog"
)

// PluginRuntime orchestrates the plugin system
type PluginRuntime struct {
	logger             zerolog.Logger
	discovery          *PluginDiscovery
	manifestLoader     *ManifestLoader
	dependencyResolver *DependencyResolver
	loader             *PluginLoader
	registry           *PluginRegistry
	toolRegistry       *ToolRegistry
	hookRegistry       *HookRegistry
	config             PluginRuntimeConfig
}

// NewPluginRuntime creates a new plugin runtime
func NewPluginRuntime(logger zerolog.Logger, config PluginRuntimeConfig) *PluginRuntime {
	manifestLoader := NewManifestLoader(logger)
	toolRegistry := NewToolRegistry()
	hookRegistry := NewHookRegistry()
	pluginRegistry := NewPluginRegistry()

	return &PluginRuntime{
		logger:             logger.With().Str("component", "plugin-runtime").Logger(),
		discovery:          NewPluginDiscovery(logger),
		manifestLoader:     manifestLoader,
		dependencyResolver: NewDependencyResolver(logger),
		loader: NewPluginLoader(
			logger,
			manifestLoader,
			toolRegistry,
			hookRegistry,
			pluginRegistry,
		),
		registry:     pluginRegistry,
		toolRegistry: toolRegistry,
		hookRegistry: hookRegistry,
		config:       config,
	}
}

// Initialize initializes the plugin runtime and loads all plugins
func (r *PluginRuntime) Initialize() (*LoadResult, error) {
	r.logger.Info().Msg("Initializing plugin runtime")

	result := &LoadResult{
		Loaded:  []string{},
		Failed:  []string{},
		Skipped: []string{},
		Errors:  make(map[string]error),
	}

	// Discover plugins
	discovered, err := r.discovery.DiscoverPlugins(PluginDiscoveryConfig{
		BuiltinDir:   r.config.BuiltinDir,
		WorkspaceDir: r.config.WorkspaceDir,
		ExtraDirs:    r.config.ExtraDirs,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin discovery failed: %w", err)
	}

	if len(discovered) == 0 {
		r.logger.Info().Msg("No plugins discovered")
		return result, nil
	}

	// Load manifests
	manifests := make(map[string]*PluginManifest)
	for _, plugin := range discovered {
		manifest, err := r.manifestLoader.LoadManifest(plugin.ManifestPath)
		if err != nil {
			r.logger.Error().Err(err).Str("plugin", plugin.ID).Msg("Failed to load manifest")
			result.Failed = append(result.Failed, plugin.ID)
			result.Errors[plugin.ID] = err
			continue
		}
		manifests[plugin.ID] = manifest
	}

	// Build dependency graph
	graph := r.dependencyResolver.BuildDependencyGraph(discovered, manifests)

	// Detect cycles
	cycles := r.dependencyResolver.DetectCycles(graph)
	if len(cycles) > 0 {
		r.logger.Error().Interface("cycles", cycles).Msg("Dependency cycles detected")
		for _, cycle := range cycles {
			for _, pluginID := range cycle {
				result.Failed = append(result.Failed, pluginID)
				result.Errors[pluginID] = fmt.Errorf("plugin is part of dependency cycle: %v", cycle)
			}
		}
	}

	// Validate dependencies
	depErrors := r.dependencyResolver.ValidateDependencies(graph)
	for pluginID, err := range depErrors {
		r.logger.Error().Err(err).Str("plugin", pluginID).Msg("Dependency validation failed")
		result.Failed = append(result.Failed, pluginID)
		result.Errors[pluginID] = err
	}

	// Get load order
	loadOrder, err := r.dependencyResolver.TopologicalSort(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to determine load order: %w", err)
	}

	// Load plugins in order
	for _, pluginID := range loadOrder {
		// Skip if already failed
		if _, failed := result.Errors[pluginID]; failed {
			continue
		}

		// Find discovered plugin
		var discoveredPlugin *DiscoveredPlugin
		for _, dp := range discovered {
			if dp.ID == pluginID {
				discoveredPlugin = &dp
				break
			}
		}

		if discoveredPlugin == nil {
			continue
		}

		// Get plugin config
		config := r.config.PluginConfigs[pluginID]

		// Load plugin
		loaded, err := r.loader.LoadPlugin(*discoveredPlugin, config)
		if err != nil {
			r.logger.Error().Err(err).Str("plugin", pluginID).Msg("Failed to load plugin")
			result.Failed = append(result.Failed, pluginID)
			result.Errors[pluginID] = err
			continue
		}

		// Register plugin
		if err := r.registry.Register(loaded); err != nil {
			r.logger.Error().Err(err).Str("plugin", pluginID).Msg("Failed to register plugin")
			result.Failed = append(result.Failed, pluginID)
			result.Errors[pluginID] = err
			continue
		}

		result.Loaded = append(result.Loaded, pluginID)
		r.logger.Info().Str("plugin", pluginID).Msg("Plugin loaded and registered")
	}

	r.logger.Info().
		Int("loaded", len(result.Loaded)).
		Int("failed", len(result.Failed)).
		Int("skipped", len(result.Skipped)).
		Msg("Plugin runtime initialization complete")

	return result, nil
}

// GetPlugin retrieves a loaded plugin
func (r *PluginRuntime) GetPlugin(pluginID string) (*LoadedPlugin, error) {
	record, exists := r.registry.Get(pluginID)
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}
	return record.Plugin, nil
}

// EnablePlugin enables a disabled plugin
func (r *PluginRuntime) EnablePlugin(pluginID string) error {
	record, exists := r.registry.Get(pluginID)
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if record.Plugin.State != StateDisabled {
		return fmt.Errorf("plugin %s is not disabled", pluginID)
	}

	if err := r.registry.UpdateState(pluginID, StateEnabled); err != nil {
		return err
	}

	r.logger.Info().Str("plugin", pluginID).Msg("Plugin enabled")
	return nil
}

// DisablePlugin disables an enabled plugin
func (r *PluginRuntime) DisablePlugin(pluginID string) error {
	record, exists := r.registry.Get(pluginID)
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if record.Plugin.State != StateEnabled {
		return fmt.Errorf("plugin %s is not enabled", pluginID)
	}

	if err := r.registry.UpdateState(pluginID, StateDisabled); err != nil {
		return err
	}

	r.logger.Info().Str("plugin", pluginID).Msg("Plugin disabled")
	return nil
}

// UnloadPlugin unloads a plugin
func (r *PluginRuntime) UnloadPlugin(pluginID string) error {
	// Check for dependents
	// In a full implementation, we would check the dependency graph
	// For now, we just unload

	if err := r.loader.UnloadPlugin(pluginID); err != nil {
		return err
	}

	r.logger.Info().Str("plugin", pluginID).Msg("Plugin unloaded")
	return nil
}

// Shutdown shuts down the plugin runtime
func (r *PluginRuntime) Shutdown() error {
	r.logger.Info().Msg("Shutting down plugin runtime")

	// Get all plugins
	records := r.registry.GetAll()

	// Unload all plugins
	for _, record := range records {
		if err := r.loader.UnloadPlugin(record.Plugin.ID); err != nil {
			r.logger.Error().Err(err).Str("plugin", record.Plugin.ID).Msg("Failed to unload plugin")
		}
	}

	r.logger.Info().Msg("Plugin runtime shutdown complete")
	return nil
}

// GetToolRegistry returns the tool registry
func (r *PluginRuntime) GetToolRegistry() *ToolRegistry {
	return r.toolRegistry
}

// GetHookRegistry returns the hook registry
func (r *PluginRuntime) GetHookRegistry() *HookRegistry {
	return r.hookRegistry
}

// GetPluginRegistry returns the plugin registry
func (r *PluginRuntime) GetPluginRegistry() *PluginRegistry {
	return r.registry
}
