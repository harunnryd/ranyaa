package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates configuration values
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateAPIKey validates an API key format
func (v *Validator) ValidateAPIKey(key string, provider string) error {
	if key == "" {
		return fmt.Errorf("%s API key cannot be empty", provider)
	}

	switch provider {
	case "anthropic":
		if !strings.HasPrefix(key, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format (should start with sk-ant-)")
		}
	case "openai":
		if !strings.HasPrefix(key, "sk-") {
			return fmt.Errorf("invalid OpenAI API key format (should start with sk-)")
		}
	}

	return nil
}

// ValidateTelegramToken validates a Telegram bot token
func (v *Validator) ValidateTelegramToken(token string) error {
	if token == "" {
		return fmt.Errorf("telegram bot token cannot be empty")
	}

	// Telegram bot tokens have format: <bot_id>:<token>
	// Example: 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
	pattern := regexp.MustCompile(`^\d+:[A-Za-z0-9_-]+$`)
	if !pattern.MatchString(token) {
		return fmt.Errorf("invalid Telegram bot token format")
	}

	return nil
}

// ValidateModel validates a model name
func (v *Validator) ValidateModel(model string) error {
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	// Check if it's a known model
	knownModels := []string{
		"claude-opus-4",
		"claude-sonnet-4",
		"claude-haiku-4",
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
	}

	for _, known := range knownModels {
		if model == known {
			return nil
		}
	}

	// Allow custom models (just warn)
	return nil
}

// ValidateAgentRole validates an agent role
func (v *Validator) ValidateAgentRole(role string) error {
	if role == "" {
		return nil // Role is optional
	}

	validRoles := []string{"captain", "executor", "critic", "general"}
	for _, valid := range validRoles {
		if role == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid agent role: %s (must be one of: %s)", role, strings.Join(validRoles, ", "))
}

// ValidateTemperature validates temperature value
func (v *Validator) ValidateTemperature(temp float64) error {
	if temp < 0 || temp > 1 {
		return fmt.Errorf("temperature must be between 0 and 1, got %f", temp)
	}
	return nil
}

// ValidateMaxTokens validates max tokens value
func (v *Validator) ValidateMaxTokens(tokens int) error {
	if tokens <= 0 {
		return fmt.Errorf("max tokens must be positive, got %d", tokens)
	}
	if tokens > 200000 {
		return fmt.Errorf("max tokens too large (max 200000), got %d", tokens)
	}
	return nil
}

// ValidateLogLevel validates log level
func (v *Validator) ValidateLogLevel(level string) error {
	validLevels := []string{"debug", "info", "warn", "error"}
	for _, valid := range validLevels {
		if level == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid log level: %s (must be one of: %s)", level, strings.Join(validLevels, ", "))
}

// ValidateDMPolicy validates Telegram DM policy
func (v *Validator) ValidateDMPolicy(policy string) error {
	if policy == "" {
		return nil // Use default
	}

	validPolicies := []string{"pairing", "allowlist", "open", "disabled"}
	for _, valid := range validPolicies {
		if policy == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid DM policy: %s (must be one of: %s)", policy, strings.Join(validPolicies, ", "))
}

// ValidateTelegramStreamMode validates Telegram streaming mode.
func (v *Validator) ValidateTelegramStreamMode(mode string) error {
	if mode == "" {
		return nil // Use default
	}

	validModes := []string{"off", "partial", "block"}
	for _, valid := range validModes {
		if mode == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid telegram stream mode: %s (must be one of: %s)", mode, strings.Join(validModes, ", "))
}

// ValidateConfig performs comprehensive validation
func (v *Validator) ValidateConfig(cfg *Config) []error {
	var errors []error

	// Validate AI profiles (canonical source)
	if len(cfg.AI.Profiles) > 0 {
		for i, profile := range cfg.AI.Profiles {
			if profile.Provider != "" {
				if err := v.ValidateAPIKey(profile.APIKey, profile.Provider); err != nil {
					errors = append(errors, fmt.Errorf("AI profile %d (%s): %w", i, profile.ID, err))
				}
			}
		}
	}

	// Validate Telegram
	if cfg.Channels.Telegram.Enabled && cfg.Telegram.BotToken != "" {
		if err := v.ValidateTelegramToken(cfg.Telegram.BotToken); err != nil {
			errors = append(errors, err)
		}
	}
	if err := v.ValidateDMPolicy(cfg.Telegram.DMPolicy); err != nil {
		errors = append(errors, err)
	}
	if err := v.ValidateTelegramStreamMode(cfg.Telegram.StreamMode); err != nil {
		errors = append(errors, err)
	}
	if cfg.Telegram.DedupeTTLSeconds < 0 {
		errors = append(errors, fmt.Errorf("telegram dedupe_ttl_seconds must be >= 0"))
	}
	if cfg.Telegram.StreamMinInterval < 0 {
		errors = append(errors, fmt.Errorf("telegram stream_min_interval_ms must be >= 0"))
	}
	if cfg.Telegram.StreamMinChars < 0 {
		errors = append(errors, fmt.Errorf("telegram stream_min_chars must be >= 0"))
	}

	if cfg.Tools.Retry.MaxAttempts < 0 {
		errors = append(errors, fmt.Errorf("tools.retry.max_attempts must be >= 0"))
	}
	if cfg.Tools.Retry.InitialBackoffMs < 0 {
		errors = append(errors, fmt.Errorf("tools.retry.initial_backoff_ms must be >= 0"))
	}
	if cfg.Tools.Retry.MaxBackoffMs < 0 {
		errors = append(errors, fmt.Errorf("tools.retry.max_backoff_ms must be >= 0"))
	}

	if cfg.Hooks.Enabled {
		for i, hook := range cfg.Hooks.Entries {
			if !hook.Enabled {
				continue
			}
			if strings.TrimSpace(hook.Event) == "" {
				errors = append(errors, fmt.Errorf("hook %d: event is required", i))
			}
			if strings.TrimSpace(hook.Script) == "" {
				errors = append(errors, fmt.Errorf("hook %d: script is required", i))
			}
		}
	}

	// Validate agents
	for i, agent := range cfg.Agents {
		if err := v.ValidateModel(agent.Model); err != nil {
			errors = append(errors, fmt.Errorf("agent %d (%s): %w", i, agent.ID, err))
		}
		if err := v.ValidateAgentRole(agent.Role); err != nil {
			errors = append(errors, fmt.Errorf("agent %d (%s): %w", i, agent.ID, err))
		}
		if agent.Temperature != 0 {
			if err := v.ValidateTemperature(agent.Temperature); err != nil {
				errors = append(errors, fmt.Errorf("agent %d (%s): %w", i, agent.ID, err))
			}
		}
		if agent.MaxTokens != 0 {
			if err := v.ValidateMaxTokens(agent.MaxTokens); err != nil {
				errors = append(errors, fmt.Errorf("agent %d (%s): %w", i, agent.ID, err))
			}
		}
	}

	// Validate logging
	if err := v.ValidateLogLevel(cfg.Logging.Level); err != nil {
		errors = append(errors, err)
	}

	return errors
}
