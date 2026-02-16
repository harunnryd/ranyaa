package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Wizard provides an interactive configuration wizard
type Wizard struct {
	reader *bufio.Reader
}

// NewWizard creates a new configuration wizard
func NewWizard() *Wizard {
	return &Wizard{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run runs the interactive configuration wizard
func (w *Wizard) Run() (*Config, error) {
	fmt.Println("=== Ranya Configuration Wizard ===")
	fmt.Println()

	cfg := DefaultConfig()
	validator := NewValidator()

	// API Keys
	fmt.Println("API Keys (at least one is required):")
	fmt.Println()

	// Anthropic API Key
	for {
		fmt.Print("Anthropic API Key (press Enter to skip): ")
		key, err := w.readLine()
		if err != nil {
			return nil, err
		}

		if key == "" {
			break
		}

		if err := validator.ValidateAPIKey(key, "anthropic"); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		cfg.AnthropicAPIKey = key
		break
	}

	// OpenAI API Key
	for {
		fmt.Print("OpenAI API Key (press Enter to skip): ")
		key, err := w.readLine()
		if err != nil {
			return nil, err
		}

		if key == "" {
			break
		}

		if err := validator.ValidateAPIKey(key, "openai"); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		cfg.OpenAIAPIKey = key
		break
	}

	// Check if at least one API key is provided
	if cfg.AnthropicAPIKey == "" && cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("at least one API key is required")
	}

	fmt.Println()

	// Telegram Configuration
	fmt.Println("Telegram Configuration:")
	fmt.Println()

	fmt.Print("Enable Telegram integration? (y/n) [y]: ")
	enable, err := w.readLine()
	if err != nil {
		return nil, err
	}

	if enable == "" || strings.ToLower(enable) == "y" {
		cfg.Channels.Telegram.Enabled = true

		// Bot Token
		for {
			fmt.Print("Telegram Bot Token: ")
			token, err := w.readLine()
			if err != nil {
				return nil, err
			}

			if token == "" {
				fmt.Println("Error: Bot token is required when Telegram is enabled")
				continue
			}

			if err := validator.ValidateTelegramToken(token); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			cfg.Telegram.BotToken = token
			break
		}

		// DM Policy
		fmt.Println()
		fmt.Println("DM Policy options:")
		fmt.Println("  pairing   - Only paired users can DM (default)")
		fmt.Println("  allowlist - Only users in allowlist can DM")
		fmt.Println("  open      - Anyone can DM")
		fmt.Println("  disabled  - No DMs allowed")
		fmt.Print("DM Policy [pairing]: ")
		policy, err := w.readLine()
		if err != nil {
			return nil, err
		}

		if policy == "" {
			policy = "pairing"
		}

		if err := validator.ValidateDMPolicy(policy); err != nil {
			fmt.Printf("Warning: %v, using default (pairing)\n", err)
			policy = "pairing"
		}

		cfg.Telegram.DMPolicy = policy
	} else {
		cfg.Channels.Telegram.Enabled = false
	}

	fmt.Println()

	// Default Model
	fmt.Println("Default Model:")
	fmt.Print("Model name [claude-sonnet-4]: ")
	model, err := w.readLine()
	if err != nil {
		return nil, err
	}

	if model != "" {
		cfg.Models.Default = model
	}

	fmt.Println()

	// Log Level
	fmt.Println("Logging:")
	fmt.Print("Log level (debug/info/warn/error) [info]: ")
	level, err := w.readLine()
	if err != nil {
		return nil, err
	}

	if level != "" {
		if err := validator.ValidateLogLevel(level); err != nil {
			fmt.Printf("Warning: %v, using default (info)\n", err)
		} else {
			cfg.Logging.Level = level
		}
	}

	fmt.Println()
	fmt.Println("Configuration complete!")

	return cfg, nil
}

func (w *Wizard) readLine() (string, error) {
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
