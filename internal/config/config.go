package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config represents the main Ranya configuration
type Config struct {
	// Telegram
	Telegram TelegramConfig `json:"telegram" mapstructure:"telegram"`

	// Agents
	Agents []AgentConfig `json:"agents" mapstructure:"agents"`

	// Models
	Models ModelsConfig `json:"models" mapstructure:"models"`

	// Channels
	Channels ChannelsConfig `json:"channels" mapstructure:"channels"`

	// Tools
	Tools ToolsConfig `json:"tools" mapstructure:"tools"`

	// Hooks
	Hooks HooksConfig `json:"hooks" mapstructure:"hooks"`

	// Logging
	Logging LoggingConfig `json:"logging" mapstructure:"logging"`

	// Data directory
	DataDir string `json:"data_dir" mapstructure:"data_dir"`

	// Workspace path
	WorkspacePath string `json:"workspace_path" mapstructure:"workspace_path"`

	// Gateway configuration
	Gateway GatewayConfig `json:"gateway" mapstructure:"gateway"`

	// Webhook configuration
	Webhook WebhookConfig `json:"webhook" mapstructure:"webhook"`

	// AI configuration
	AI AIConfig `json:"ai" mapstructure:"ai"`

	// Moderation configuration
	Moderation ModerationConfig `json:"moderation" mapstructure:"moderation"`
}

// ModerationConfig holds content moderation settings
type ModerationConfig struct {
	Enabled         bool     `json:"enabled" mapstructure:"enabled"`
	BlockedKeywords []string `json:"blocked_keywords" mapstructure:"blocked_keywords"`
	BlockedPatterns []string `json:"blocked_patterns" mapstructure:"blocked_patterns"`
}

// TelegramConfig holds Telegram bot configuration
type TelegramConfig struct {
	BotToken           string  `json:"bot_token" mapstructure:"bot_token"`
	DMPolicy           string  `json:"dm_policy" mapstructure:"dm_policy"` // pairing, allowlist, open, disabled
	Allowlist          []int64 `json:"allowlist" mapstructure:"allowlist"`
	DedupeTTLSeconds   int     `json:"dedupe_ttl_seconds" mapstructure:"dedupe_ttl_seconds"`
	StreamMode         string  `json:"stream_mode" mapstructure:"stream_mode"` // off, partial, block
	StreamMinInterval  int     `json:"stream_min_interval_ms" mapstructure:"stream_min_interval_ms"`
	StreamMinChars     int     `json:"stream_min_chars" mapstructure:"stream_min_chars"`
	APIEndpoint        string  `json:"api_endpoint,omitempty" mapstructure:"api_endpoint"`
	PairingPrompt      string  `json:"pairing_prompt,omitempty" mapstructure:"pairing_prompt"`
	PairingSuccessText string  `json:"pairing_success_text,omitempty" mapstructure:"pairing_success_text"`
}

// AgentConfig represents an agent configuration
type AgentConfig struct {
	ID                     string           `json:"id" mapstructure:"id"`
	Name                   string           `json:"name" mapstructure:"name"`
	Role                   string           `json:"role" mapstructure:"role"` // captain, executor, critic, general
	Model                  string           `json:"model" mapstructure:"model"`
	Temperature            float64          `json:"temperature" mapstructure:"temperature"`
	MaxTokens              int              `json:"max_tokens" mapstructure:"max_tokens"`
	SystemPrompt           string           `json:"system_prompt" mapstructure:"system_prompt"`
	Tools                  ToolPolicyConfig `json:"tools" mapstructure:"tools"`
	Workspace              string           `json:"workspace" mapstructure:"workspace"`
	Sandbox                SandboxConfig    `json:"sandbox" mapstructure:"sandbox"`
	MaxConcurrentSubAgents int              `json:"max_concurrent_sub_agents" mapstructure:"max_concurrent_sub_agents"`
	AllowedSubAgents       []string         `json:"allowed_sub_agents" mapstructure:"allowed_sub_agents"`
}

// ToolPolicyConfig defines tool access policies
type ToolPolicyConfig struct {
	Allow []string `json:"allow" mapstructure:"allow"`
	Deny  []string `json:"deny" mapstructure:"deny"`
}

// SandboxConfig defines sandbox settings
type SandboxConfig struct {
	Mode        string `json:"mode" mapstructure:"mode"`                 // off, all, tools
	Scope       string `json:"scope" mapstructure:"scope"`               // agent, session
	Runtime     string `json:"runtime" mapstructure:"runtime"`           // host, docker
	DockerImage string `json:"docker_image" mapstructure:"docker_image"` // docker image when runtime=docker
}

// ModelsConfig holds model configuration
type ModelsConfig struct {
	Default  string            `json:"default" mapstructure:"default"`
	Aliases  map[string]string `json:"aliases" mapstructure:"aliases"`
	Fallback []string          `json:"fallback" mapstructure:"fallback"`
}

// ChannelsConfig holds channel configuration
type ChannelsConfig struct {
	Telegram ChannelConfig `json:"telegram" mapstructure:"telegram"`
	Gateway  ChannelConfig `json:"gateway" mapstructure:"gateway"`
}

// ChannelConfig represents a channel configuration
type ChannelConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// ToolsConfig holds tool configuration
type ToolsConfig struct {
	ExecApprovals ExecApprovalsConfig `json:"exec_approvals" mapstructure:"exec_approvals"`
	Retry         ToolRetryConfig     `json:"retry" mapstructure:"retry"`
	MCP           MCPConfig           `json:"mcp" mapstructure:"mcp"`
}

// ExecApprovalsConfig holds exec approval settings
type ExecApprovalsConfig struct {
	Enabled       bool   `json:"enabled" mapstructure:"enabled"`
	AllowlistPath string `json:"allowlist_path" mapstructure:"allowlist_path"`
}

// ToolRetryConfig holds transient retry settings for tool execution.
type ToolRetryConfig struct {
	Enabled          bool `json:"enabled" mapstructure:"enabled"`
	MaxAttempts      int  `json:"max_attempts" mapstructure:"max_attempts"`
	InitialBackoffMs int  `json:"initial_backoff_ms" mapstructure:"initial_backoff_ms"`
	MaxBackoffMs     int  `json:"max_backoff_ms" mapstructure:"max_backoff_ms"`
}

// MCPConfig holds MCP server integration settings.
type MCPConfig struct {
	Enabled bool              `json:"enabled" mapstructure:"enabled"`
	Servers []MCPServerConfig `json:"servers" mapstructure:"servers"`
}

// MCPServerConfig defines a single MCP server command.
type MCPServerConfig struct {
	ID      string   `json:"id" mapstructure:"id"`
	Command string   `json:"command" mapstructure:"command"`
	Args    []string `json:"args" mapstructure:"args"`
}

// HooksConfig holds daemon hook execution settings.
type HooksConfig struct {
	Enabled bool         `json:"enabled" mapstructure:"enabled"`
	Entries []HookConfig `json:"entries" mapstructure:"entries"`
}

// HookConfig defines a lifecycle hook script.
type HookConfig struct {
	ID        string `json:"id" mapstructure:"id"`
	Event     string `json:"event" mapstructure:"event"` // daemon:startup, command:new, agent:bootstrap
	Script    string `json:"script" mapstructure:"script"`
	TimeoutMs int    `json:"timeout_ms,omitempty" mapstructure:"timeout_ms"`
	Enabled   bool   `json:"enabled" mapstructure:"enabled"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level     string `json:"level" mapstructure:"level"`
	File      string `json:"file" mapstructure:"file"`
	MaxSize   int    `json:"max_size" mapstructure:"max_size"` // MB
	MaxAge    int    `json:"max_age" mapstructure:"max_age"`   // days
	Compress  bool   `json:"compress" mapstructure:"compress"`
	Redaction bool   `json:"redaction" mapstructure:"redaction"`
}

// GatewayConfig holds gateway server configuration
type GatewayConfig struct {
	Port         int    `json:"port" mapstructure:"port"`
	Host         string `json:"host" mapstructure:"host"`
	SharedSecret string `json:"shared_secret" mapstructure:"shared_secret"`
	TickInterval int    `json:"tick_interval_ms" mapstructure:"tick_interval_ms"`
}

// WebhookConfig holds webhook server configuration
type WebhookConfig struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
	Port    int    `json:"port" mapstructure:"port"`
	Host    string `json:"host" mapstructure:"host"`
	Timeout int    `json:"timeout" mapstructure:"timeout"` // seconds
}

// AIConfig holds AI provider configuration
type AIConfig struct {
	Profiles []AIProfile `json:"profiles" mapstructure:"profiles"`
}

// AIProfile represents an AI provider profile
type AIProfile struct {
	ID       string `json:"id" mapstructure:"id"`
	Provider string `json:"provider" mapstructure:"provider"` // anthropic, openai, gemini
	APIKey   string `json:"api_key" mapstructure:"api_key"`
	Priority int    `json:"priority" mapstructure:"priority"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Telegram: TelegramConfig{
			DMPolicy:           "pairing",
			DedupeTTLSeconds:   300,
			StreamMode:         "partial",
			StreamMinInterval:  2000,
			StreamMinChars:     200,
			PairingPrompt:      "⚠️ Please pair this device first. Send /pair to begin.",
			PairingSuccessText: "✅ Device paired. You can now send messages.",
		},
		Models: ModelsConfig{
			Default: "claude-sonnet-4",
			Aliases: map[string]string{
				"opus":   "claude-opus-4",
				"sonnet": "claude-sonnet-4",
				"gpt4":   "gpt-4-turbo",
			},
			Fallback: []string{"claude-sonnet-4", "gpt-4-turbo"},
		},
		Channels: ChannelsConfig{
			Telegram: ChannelConfig{Enabled: true},
			Gateway:  ChannelConfig{Enabled: false},
		},
		Tools: ToolsConfig{
			ExecApprovals: ExecApprovalsConfig{
				Enabled: true,
			},
			Retry: ToolRetryConfig{
				Enabled:          true,
				MaxAttempts:      3,
				InitialBackoffMs: 250,
				MaxBackoffMs:     2000,
			},
			MCP: MCPConfig{
				Enabled: false,
				Servers: []MCPServerConfig{},
			},
		},
		Hooks: HooksConfig{
			Enabled: false,
			Entries: []HookConfig{},
		},
		Logging: LoggingConfig{
			Level:     "info",
			MaxSize:   100,
			MaxAge:    7,
			Compress:  true,
			Redaction: true,
		},
		Gateway: GatewayConfig{
			Port:         8080,
			Host:         "0.0.0.0",
			SharedSecret: "",
			TickInterval: 30000,
		},
		Webhook: WebhookConfig{
			Enabled: false,
			Port:    3000,
			Host:    "0.0.0.0",
			Timeout: 30,
		},
		AI: AIConfig{
			Profiles: []AIProfile{},
		},
		DataDir:       "",
		WorkspacePath: "",
		Agents: []AgentConfig{
			{
				ID:          "default",
				Name:        "Default Agent",
				Role:        "general",
				Model:       "claude-sonnet-4",
				Temperature: 0.7,
				MaxTokens:   4096,
				Tools: ToolPolicyConfig{
					Allow: []string{"*"},
					Deny:  []string{},
				},
				Sandbox: SandboxConfig{
					Mode:        "off",
					Scope:       "agent",
					Runtime:     "host",
					DockerImage: "",
				},
				MaxConcurrentSubAgents: 5,
				AllowedSubAgents:       []string{"*"},
			},
		},
		Moderation: ModerationConfig{
			Enabled:         false,
			BlockedKeywords: []string{},
			BlockedPatterns: []string{},
		},
	}
}

// String returns a JSON representation of the config
func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Require at least one AI profile
	if len(c.AI.Profiles) == 0 {
		return fmt.Errorf("no AI credentials configured: at least one AI profile is required")
	}

	// Validate AI profiles
	for i, profile := range c.AI.Profiles {
		if profile.ID == "" {
			return fmt.Errorf("AI profile %d: ID is required", i)
		}
		if profile.Provider == "" {
			return fmt.Errorf("AI profile %s: provider is required", profile.ID)
		}
		if profile.APIKey == "" {
			return fmt.Errorf("AI profile %s: api_key is required", profile.ID)
		}
		validProviders := []string{"anthropic", "openai", "gemini"}
		valid := false
		for _, vp := range validProviders {
			if profile.Provider == vp {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("AI profile %s: invalid provider %s (must be: anthropic, openai, gemini)", profile.ID, profile.Provider)
		}
	}

	// Validate MCP servers
	if c.Tools.MCP.Enabled {
		if len(c.Tools.MCP.Servers) == 0 {
			return fmt.Errorf("mcp enabled but no servers configured")
		}
		seen := map[string]bool{}
		for i, server := range c.Tools.MCP.Servers {
			if strings.TrimSpace(server.ID) == "" {
				return fmt.Errorf("mcp server %d: id is required", i)
			}
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("mcp server %s: command is required", server.ID)
			}
			if seen[server.ID] {
				return fmt.Errorf("mcp server id %s is duplicated", server.ID)
			}
			seen[server.ID] = true
		}
	}

	// Validate agents
	if len(c.Agents) == 0 {
		return fmt.Errorf("at least one agent must be configured")
	}

	for i, agent := range c.Agents {
		if agent.ID == "" {
			return fmt.Errorf("agent %d: ID is required", i)
		}
		if agent.Model == "" {
			return fmt.Errorf("agent %s: model is required", agent.ID)
		}
		if agent.Role != "" && agent.Role != "captain" && agent.Role != "executor" && agent.Role != "critic" && agent.Role != "general" {
			return fmt.Errorf("agent %s: invalid role %s", agent.ID, agent.Role)
		}
		if agent.Sandbox.Runtime != "" && agent.Sandbox.Runtime != "host" && agent.Sandbox.Runtime != "docker" {
			return fmt.Errorf("agent %s: invalid sandbox runtime %s", agent.ID, agent.Sandbox.Runtime)
		}
		if agent.Sandbox.Runtime == "docker" && strings.TrimSpace(agent.Sandbox.DockerImage) == "" {
			return fmt.Errorf("agent %s: sandbox docker_image is required when runtime=docker", agent.ID)
		}
		for idx, subAgentID := range agent.AllowedSubAgents {
			if strings.TrimSpace(subAgentID) == "" {
				return fmt.Errorf("agent %s: allowed_sub_agents[%d] cannot be empty", agent.ID, idx)
			}
		}
	}

	// Validate Telegram if enabled
	if c.Channels.Telegram.Enabled {
		if c.Telegram.BotToken == "" {
			return fmt.Errorf("telegram bot token is required when Telegram channel is enabled")
		}
		if c.Telegram.DMPolicy != "" && c.Telegram.DMPolicy != "pairing" && c.Telegram.DMPolicy != "allowlist" && c.Telegram.DMPolicy != "open" && c.Telegram.DMPolicy != "disabled" {
			return fmt.Errorf("invalid telegram DM policy: %s", c.Telegram.DMPolicy)
		}
	}

	if c.Gateway.TickInterval <= 0 {
		return fmt.Errorf("gateway tick_interval_ms must be greater than zero")
	}

	return nil
}
