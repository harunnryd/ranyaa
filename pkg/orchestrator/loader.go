package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigLoader loads agent configurations from files
type ConfigLoader struct {
	logger Logger
}

// NewConfigLoader creates a new ConfigLoader instance
func NewConfigLoader(logger Logger) *ConfigLoader {
	return &ConfigLoader{
		logger: logger,
	}
}

// MainConfig represents the main configuration file structure
type MainConfig struct {
	Agents []AgentConfig `json:"agents" yaml:"agents"`
}

// LoadFromFile loads agent configurations from a JSON or YAML file
func (cl *ConfigLoader) LoadFromFile(path string) ([]AgentConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("config file path is required")
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Determine file format by extension
	ext := filepath.Ext(path)
	var mainConfig MainConfig

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &mainConfig); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &mainConfig); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .json, .yaml, .yml)", ext)
	}

	if cl.logger != nil {
		cl.logger.Info("Loaded agent configurations from file",
			"path", path,
			"count", len(mainConfig.Agents))
	}

	return mainConfig.Agents, nil
}

// LoadAndValidate loads agent configurations from a file and validates them
func (cl *ConfigLoader) LoadAndValidate(path string) ([]AgentConfig, error) {
	// Load configs
	configs, err := cl.LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	// Validate each config
	if err := cl.ValidateConfigs(configs); err != nil {
		return nil, err
	}

	if cl.logger != nil {
		cl.logger.Info("Validated agent configurations",
			"count", len(configs))
	}

	return configs, nil
}

// ValidateConfigs validates a list of agent configurations
func (cl *ConfigLoader) ValidateConfigs(configs []AgentConfig) error {
	if len(configs) == 0 {
		return fmt.Errorf("no agent configurations found")
	}

	// Track agent IDs to check for duplicates
	seenIDs := make(map[string]bool)

	for i, config := range configs {
		// Validate individual config
		if err := config.Validate(); err != nil {
			return fmt.Errorf("agent config at index %d is invalid: %w", i, err)
		}

		// Check for duplicate IDs
		if seenIDs[config.ID] {
			return fmt.Errorf("duplicate agent ID found: %s", config.ID)
		}
		seenIDs[config.ID] = true
	}

	return nil
}

// LoadFromJSON loads agent configurations from a JSON string
func (cl *ConfigLoader) LoadFromJSON(jsonStr string) ([]AgentConfig, error) {
	if jsonStr == "" {
		return nil, fmt.Errorf("JSON string is required")
	}

	var mainConfig MainConfig
	if err := json.Unmarshal([]byte(jsonStr), &mainConfig); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if cl.logger != nil {
		cl.logger.Debug("Loaded agent configurations from JSON",
			"count", len(mainConfig.Agents))
	}

	return mainConfig.Agents, nil
}

// LoadFromYAML loads agent configurations from a YAML string
func (cl *ConfigLoader) LoadFromYAML(yamlStr string) ([]AgentConfig, error) {
	if yamlStr == "" {
		return nil, fmt.Errorf("YAML string is required")
	}

	var mainConfig MainConfig
	if err := yaml.Unmarshal([]byte(yamlStr), &mainConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if cl.logger != nil {
		cl.logger.Debug("Loaded agent configurations from YAML",
			"count", len(mainConfig.Agents))
	}

	return mainConfig.Agents, nil
}
