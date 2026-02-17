package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Loader handles configuration loading
type Loader struct {
	configPath string
}

// NewLoader creates a new config loader
func NewLoader(configPath string) *Loader {
	return &Loader{
		configPath: configPath,
	}
}

// Load loads the configuration from file
func (l *Loader) Load() (*Config, error) {
	// Determine config path
	configPath := l.configPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, ".ranya", "ranya.json")
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		return DefaultConfig(), nil
	}

	// Setup viper
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("json")

	// Read environment variables
	v.SetEnvPrefix("RANYA")
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal into config struct
	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set data directory if not specified
	if cfg.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.DataDir = filepath.Join(home, ".ranya")
	}

	// Set logging file path if not specified
	if cfg.Logging.File == "" {
		cfg.Logging.File = filepath.Join(cfg.DataDir, "ranya.log")
	}

	// Set exec approvals path if not specified
	if cfg.Tools.ExecApprovals.AllowlistPath == "" {
		cfg.Tools.ExecApprovals.AllowlistPath = filepath.Join(cfg.DataDir, "exec-approvals.json")
	}

	return cfg, nil
}

// Save saves the configuration to file
func (l *Loader) Save(cfg *Config) error {
	// Determine config path
	configPath := l.configPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, ".ranya", "ranya.json")
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Setup viper
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("json")

	// Set all config values (use canonical fields only)
	v.Set("telegram", cfg.Telegram)
	v.Set("agents", cfg.Agents)
	v.Set("models", cfg.Models)
	v.Set("channels", cfg.Channels)
	v.Set("tools", cfg.Tools)
	v.Set("hooks", cfg.Hooks)
	v.Set("logging", cfg.Logging)
	v.Set("data_dir", cfg.DataDir)
	v.Set("workspace_path", cfg.WorkspacePath)
	v.Set("gateway", cfg.Gateway)
	v.Set("webhook", cfg.Webhook)
	v.Set("ai", cfg.AI)

	// Write config file
	if err := v.WriteConfig(); err != nil {
		// If file doesn't exist, create it
		if os.IsNotExist(err) {
			if err := v.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to write config file: %w", err)
		}
	}

	return nil
}

// GetConfigPath returns the config file path
func (l *Loader) GetConfigPath() string {
	if l.configPath != "" {
		return l.configPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ranya", "ranya.json")
}

// Load is a convenience function that creates a loader and loads the config
func Load(configPath string) (*Config, error) {
	loader := NewLoader(configPath)
	return loader.Load()
}
