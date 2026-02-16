package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAPIKey(t *testing.T) {
	v := NewValidator()

	t.Run("valid anthropic key", func(t *testing.T) {
		err := v.ValidateAPIKey("sk-ant-test123", "anthropic")
		assert.NoError(t, err)
	})

	t.Run("invalid anthropic key", func(t *testing.T) {
		err := v.ValidateAPIKey("invalid-key", "anthropic")
		assert.Error(t, err)
	})

	t.Run("valid openai key", func(t *testing.T) {
		err := v.ValidateAPIKey("sk-test123", "openai")
		assert.NoError(t, err)
	})

	t.Run("invalid openai key", func(t *testing.T) {
		err := v.ValidateAPIKey("invalid-key", "openai")
		assert.Error(t, err)
	})

	t.Run("empty key", func(t *testing.T) {
		err := v.ValidateAPIKey("", "anthropic")
		assert.Error(t, err)
	})
}

func TestValidateTelegramToken(t *testing.T) {
	v := NewValidator()

	t.Run("valid token", func(t *testing.T) {
		err := v.ValidateTelegramToken("123456789:ABCdefGHIjklMNOpqrsTUVwxyz")
		assert.NoError(t, err)
	})

	t.Run("invalid format", func(t *testing.T) {
		err := v.ValidateTelegramToken("invalid-token")
		assert.Error(t, err)
	})

	t.Run("empty token", func(t *testing.T) {
		err := v.ValidateTelegramToken("")
		assert.Error(t, err)
	})
}

func TestValidateModel(t *testing.T) {
	v := NewValidator()

	t.Run("known model", func(t *testing.T) {
		err := v.ValidateModel("claude-sonnet-4")
		assert.NoError(t, err)
	})

	t.Run("custom model", func(t *testing.T) {
		err := v.ValidateModel("custom-model")
		assert.NoError(t, err)
	})

	t.Run("empty model", func(t *testing.T) {
		err := v.ValidateModel("")
		assert.Error(t, err)
	})
}

func TestValidateAgentRole(t *testing.T) {
	v := NewValidator()

	t.Run("valid roles", func(t *testing.T) {
		roles := []string{"captain", "executor", "critic", "general"}
		for _, role := range roles {
			err := v.ValidateAgentRole(role)
			assert.NoError(t, err, "role %s should be valid", role)
		}
	})

	t.Run("empty role", func(t *testing.T) {
		err := v.ValidateAgentRole("")
		assert.NoError(t, err) // Empty is allowed
	})

	t.Run("invalid role", func(t *testing.T) {
		err := v.ValidateAgentRole("invalid")
		assert.Error(t, err)
	})
}

func TestValidateTemperature(t *testing.T) {
	v := NewValidator()

	t.Run("valid temperature", func(t *testing.T) {
		err := v.ValidateTemperature(0.7)
		assert.NoError(t, err)
	})

	t.Run("too low", func(t *testing.T) {
		err := v.ValidateTemperature(-0.1)
		assert.Error(t, err)
	})

	t.Run("too high", func(t *testing.T) {
		err := v.ValidateTemperature(1.1)
		assert.Error(t, err)
	})
}

func TestValidateMaxTokens(t *testing.T) {
	v := NewValidator()

	t.Run("valid tokens", func(t *testing.T) {
		err := v.ValidateMaxTokens(4096)
		assert.NoError(t, err)
	})

	t.Run("zero tokens", func(t *testing.T) {
		err := v.ValidateMaxTokens(0)
		assert.Error(t, err)
	})

	t.Run("negative tokens", func(t *testing.T) {
		err := v.ValidateMaxTokens(-100)
		assert.Error(t, err)
	})

	t.Run("too many tokens", func(t *testing.T) {
		err := v.ValidateMaxTokens(300000)
		assert.Error(t, err)
	})
}

func TestValidateLogLevel(t *testing.T) {
	v := NewValidator()

	t.Run("valid levels", func(t *testing.T) {
		levels := []string{"debug", "info", "warn", "error"}
		for _, level := range levels {
			err := v.ValidateLogLevel(level)
			assert.NoError(t, err, "level %s should be valid", level)
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		err := v.ValidateLogLevel("invalid")
		assert.Error(t, err)
	})
}

func TestValidateDMPolicy(t *testing.T) {
	v := NewValidator()

	t.Run("valid policies", func(t *testing.T) {
		policies := []string{"pairing", "allowlist", "open", "disabled"}
		for _, policy := range policies {
			err := v.ValidateDMPolicy(policy)
			assert.NoError(t, err, "policy %s should be valid", policy)
		}
	})

	t.Run("empty policy", func(t *testing.T) {
		err := v.ValidateDMPolicy("")
		assert.NoError(t, err) // Empty is allowed
	})

	t.Run("invalid policy", func(t *testing.T) {
		err := v.ValidateDMPolicy("invalid")
		assert.Error(t, err)
	})
}

func TestValidateConfig(t *testing.T) {
	v := NewValidator()

	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AnthropicAPIKey = "sk-ant-test123"

		errors := v.ValidateConfig(cfg)
		assert.Empty(t, errors)
	})

	t.Run("multiple errors", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AnthropicAPIKey = "invalid-key"
		cfg.Telegram.DMPolicy = "invalid"
		cfg.Logging.Level = "invalid"

		errors := v.ValidateConfig(cfg)
		assert.NotEmpty(t, errors)
		assert.GreaterOrEqual(t, len(errors), 3)
	})
}
