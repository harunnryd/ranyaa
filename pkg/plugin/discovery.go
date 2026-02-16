package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// PluginDiscovery scans directories to find plugins
type PluginDiscovery struct {
	logger zerolog.Logger
}

// NewPluginDiscovery creates a new plugin discovery instance
func NewPluginDiscovery(logger zerolog.Logger) *PluginDiscovery {
	return &PluginDiscovery{
		logger: logger.With().Str("component", "plugin-discovery").Logger(),
	}
}

// DiscoverPlugins scans configured directories for plugins
// Returns all discovered plugins with their source type
func (d *PluginDiscovery) DiscoverPlugins(config PluginDiscoveryConfig) ([]DiscoveredPlugin, error) {
	var discovered []DiscoveredPlugin

	// Scan builtin directory
	if config.BuiltinDir != "" {
		plugins, err := d.scanDirectory(config.BuiltinDir, SourceBuiltin)
		if err != nil {
			d.logger.Warn().Err(err).Str("dir", config.BuiltinDir).Msg("Failed to scan builtin directory")
		} else {
			discovered = append(discovered, plugins...)
		}
	}

	// Scan workspace directory
	if config.WorkspaceDir != "" {
		plugins, err := d.scanDirectory(config.WorkspaceDir, SourceWorkspace)
		if err != nil {
			d.logger.Warn().Err(err).Str("dir", config.WorkspaceDir).Msg("Failed to scan workspace directory")
		} else {
			discovered = append(discovered, plugins...)
		}
	}

	// Scan extra directories
	for _, extraDir := range config.ExtraDirs {
		if extraDir == "" {
			continue
		}
		plugins, err := d.scanDirectory(extraDir, SourceExtra)
		if err != nil {
			d.logger.Warn().Err(err).Str("dir", extraDir).Msg("Failed to scan extra directory")
		} else {
			discovered = append(discovered, plugins...)
		}
	}

	d.logger.Info().Int("count", len(discovered)).Msg("Plugin discovery completed")
	return discovered, nil
}

// scanDirectory scans a single directory for plugins
func (d *PluginDiscovery) scanDirectory(dir string, source PluginSource) ([]DiscoveredPlugin, error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			d.logger.Debug().Str("dir", dir).Msg("Directory does not exist, skipping")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var discovered []DiscoveredPlugin

	// Check each subdirectory for plugin.json
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "plugin.json")

		// Check if plugin.json exists
		if _, err := os.Stat(manifestPath); err != nil {
			if os.IsNotExist(err) {
				d.logger.Debug().
					Str("dir", pluginDir).
					Msg("Directory does not contain plugin.json, skipping")
				continue
			}
			d.logger.Warn().
				Err(err).
				Str("dir", pluginDir).
				Msg("Failed to check for plugin.json")
			continue
		}

		// Plugin found
		plugin := DiscoveredPlugin{
			ID:           entry.Name(),
			Path:         pluginDir,
			Source:       source,
			ManifestPath: manifestPath,
		}

		discovered = append(discovered, plugin)
		d.logger.Debug().
			Str("id", plugin.ID).
			Str("path", plugin.Path).
			Str("source", string(source)).
			Msg("Discovered plugin")
	}

	return discovered, nil
}
