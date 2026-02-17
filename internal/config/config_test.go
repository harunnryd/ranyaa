package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, "pairing", cfg.Telegram.DMPolicy)
	assert.Equal(t, "claude-sonnet-4", cfg.Models.Default)
	assert.True(t, cfg.Tools.ExecApprovals.Enabled)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Len(t, cfg.Agents, 1)
	assert.Equal(t, "default", cfg.Agents[0].ID)
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Channels.Telegram.Enabled = false // Disable Telegram for this test

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing API keys", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no AI credentials")
	})

	t.Run("missing agents", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Agents = []AgentConfig{}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent")
	})

	t.Run("agent missing ID", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Agents[0].ID = ""

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ID is required")
	})

	t.Run("agent missing model", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Agents[0].Model = ""

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model is required")
	})

	t.Run("invalid agent role", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Agents[0].Role = "invalid"

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
	})

	t.Run("telegram enabled without token", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Channels.Telegram.Enabled = true
		cfg.Telegram.BotToken = ""

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bot token")
	})

	t.Run("invalid DM policy", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AI.Profiles = []AIProfile{
			{
				ID:       "test-profile",
				Provider: "anthropic",
				APIKey:   "sk-ant-test123",
				Priority: 1,
			},
		}
		cfg.Channels.Telegram.Enabled = true
		cfg.Telegram.BotToken = "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
		cfg.Telegram.DMPolicy = "invalid"

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DM policy")
	})
}

func TestConfigString(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AI.Profiles = []AIProfile{
		{
			ID:       "test-profile",
			Provider: "anthropic",
			APIKey:   "sk-ant-test123",
			Priority: 1,
		},
	}

	str := cfg.String()
	assert.NotEmpty(t, str)
	assert.Contains(t, str, "profiles")
}
