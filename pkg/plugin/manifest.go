package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/rs/zerolog"
	"github.com/xeipuuv/gojsonschema"
)

var (
	// pluginIDRegex validates plugin ID format (lowercase alphanumeric with hyphens)
	pluginIDRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

	// semverRegex validates semver version format
	semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// ManifestLoader loads and validates plugin manifests
type ManifestLoader struct {
	logger       zerolog.Logger
	schemaLoader gojsonschema.JSONLoader
}

// NewManifestLoader creates a new manifest loader
func NewManifestLoader(logger zerolog.Logger) *ManifestLoader {
	schemaLoader := gojsonschema.NewStringLoader(ManifestSchema)
	return &ManifestLoader{
		logger:       logger.With().Str("component", "manifest-loader").Logger(),
		schemaLoader: schemaLoader,
	}
}

// LoadManifest loads and validates a plugin manifest from a file
func (m *ManifestLoader) LoadManifest(path string) (*PluginManifest, error) {
	// Read manifest file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// Parse JSON
	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Validate against JSON schema
	if err := m.validateSchema(data); err != nil {
		return nil, fmt.Errorf("manifest schema validation failed: %w", err)
	}

	// Additional validation
	if err := m.validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	m.logger.Debug().
		Str("id", manifest.ID).
		Str("version", manifest.Version).
		Msg("Loaded manifest")

	return &manifest, nil
}

// validateSchema validates the manifest against the JSON schema
func (m *ManifestLoader) validateSchema(data []byte) error {
	documentLoader := gojsonschema.NewBytesLoader(data)
	result, err := gojsonschema.Validate(m.schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if !result.Valid() {
		// Collect all validation errors
		var errMsg string
		for i, err := range result.Errors() {
			if i > 0 {
				errMsg += "; "
			}
			errMsg += err.String()
		}
		return fmt.Errorf("schema validation errors: %s", errMsg)
	}

	return nil
}

// validateManifest performs additional validation beyond JSON schema
func (m *ManifestLoader) validateManifest(manifest *PluginManifest) error {
	// Validate plugin ID format
	if !pluginIDRegex.MatchString(manifest.ID) {
		return fmt.Errorf("invalid plugin ID format: %s (must be lowercase alphanumeric with hyphens)", manifest.ID)
	}

	// Validate version format
	if !semverRegex.MatchString(manifest.Version) {
		return fmt.Errorf("invalid version format: %s (must be semver: X.Y.Z)", manifest.Version)
	}

	// Validate main entry point is not empty
	if manifest.Main == "" {
		return fmt.Errorf("main entry point cannot be empty")
	}

	// Validate dependencies
	for i, dep := range manifest.Dependencies {
		if dep.PluginID == "" {
			return fmt.Errorf("dependency %d: pluginId cannot be empty", i)
		}
		// Version constraint is optional, but if provided, should be valid
		if dep.Version != "" {
			// Basic validation - detailed semver constraint validation happens in dependency resolver
			if len(dep.Version) == 0 {
				return fmt.Errorf("dependency %d: version constraint cannot be empty string", i)
			}
		}
	}

	// Validate permissions
	for i, perm := range manifest.Permissions {
		if !ValidPermissions[perm] {
			return fmt.Errorf("permission %d: unrecognized permission: %s", i, perm)
		}
	}

	return nil
}

// ParseManifest parses a manifest from JSON bytes (for testing)
func ParseManifest(data []byte) (*PluginManifest, error) {
	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}
	return &manifest, nil
}
