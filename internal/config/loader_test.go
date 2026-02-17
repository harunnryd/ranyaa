package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/path/to/config.json")
	assert.NotNil(t, loader)
	assert.Equal(t, "/path/to/config.json", loader.configPath)
}

func TestLoaderLoad(t *testing.T) {
	t.Run("load default config when file doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "nonexistent.json")

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "pairing", cfg.Telegram.DMPolicy)
	})

	t.Run("load config from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		// Create a test config file
		testConfig := `{
			"ai": {
				"profiles": [{
					"id": "test-profile",
					"provider": "anthropic",
					"api_key": "sk-ant-test123",
					"priority": 1
				}]
			},
			"telegram": {
				"bot_token": "test-token",
				"dm_policy": "open"
			}
		}`
		err := os.WriteFile(configPath, []byte(testConfig), 0644)
		require.NoError(t, err)

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.AI.Profiles, 1)
		assert.Equal(t, "test-profile", cfg.AI.Profiles[0].ID)
		assert.Equal(t, "test-token", cfg.Telegram.BotToken)
		assert.Equal(t, "open", cfg.Telegram.DMPolicy)
	})

	t.Run("set default paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		testConfig := `{
			"ai": {
				"profiles": [{
					"id": "test-profile",
					"provider": "anthropic",
					"api_key": "sk-ant-test123",
					"priority": 1
				}]
			}
		}`
		err := os.WriteFile(configPath, []byte(testConfig), 0644)
		require.NoError(t, err)

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.NotEmpty(t, cfg.DataDir)
		assert.NotEmpty(t, cfg.Logging.File)
		assert.NotEmpty(t, cfg.Tools.ExecApprovals.AllowlistPath)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.json")

		err := os.WriteFile(configPath, []byte("invalid json"), 0644)
		require.NoError(t, err)

		loader := NewLoader(configPath)
		_, err = loader.Load()

		assert.Error(t, err)
	})
}

func TestLoaderSave(t *testing.T) {
	t.Run("save config to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Telegram.BotToken = "test-token"

		loader := NewLoader(configPath)
		err := loader.Save(cfg)

		require.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(configPath)
		assert.NoError(t, err)

		// Load and verify
		loader2 := NewLoader(configPath)
		loadedCfg, err := loader2.Load()
		require.NoError(t, err)
		assert.Len(t, loadedCfg.AI.Profiles, 1)
		assert.Equal(t, "test-profile", loadedCfg.AI.Profiles[0].ID)
		assert.Equal(t, "test-token", loadedCfg.Telegram.BotToken)
	})

	t.Run("create directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "subdir", "config.json")

		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}

		loader := NewLoader(configPath)
		err := loader.Save(cfg)

		require.NoError(t, err)

		// Verify directory was created
		_, err = os.Stat(filepath.Dir(configPath))
		assert.NoError(t, err)
	})
}

func TestLoaderGetConfigPath(t *testing.T) {
	t.Run("custom path", func(t *testing.T) {
		loader := NewLoader("/custom/path/config.json")
		path := loader.GetConfigPath()
		assert.Equal(t, "/custom/path/config.json", path)
	})

	t.Run("default path", func(t *testing.T) {
		loader := NewLoader("")
		path := loader.GetConfigPath()
		assert.NotEmpty(t, path)
		assert.Contains(t, path, ".ranya")
	})
}
