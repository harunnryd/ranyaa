package orchestrator

import (
	"encoding/json"
	"fmt"
)

// DefaultAgentConfig returns a default agent configuration
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Temperature:            0.7,
		MaxTokens:              4096,
		MaxConcurrentSubAgents: 5,
		AllowedSubAgents:       []string{"*"},
		Tools: ToolPolicy{
			Allow: []string{"*"},
			Deny:  []string{},
		},
		Sandbox: SandboxConfig{
			Mode:  SandboxModeOff,
			Scope: SandboxScopeAgent,
		},
		Metadata: make(map[string]string),
	}
}

// NewAgentConfig creates a new agent configuration with required fields
func NewAgentConfig(id, name string, role AgentRole, model string) AgentConfig {
	config := DefaultAgentConfig()
	config.ID = id
	config.Name = name
	config.Role = role
	config.Model = model
	return config
}

// WithTemperature sets the temperature for the agent config
func (c AgentConfig) WithTemperature(temp float64) AgentConfig {
	c.Temperature = temp
	return c
}

// WithMaxTokens sets the max tokens for the agent config
func (c AgentConfig) WithMaxTokens(tokens int) AgentConfig {
	c.MaxTokens = tokens
	return c
}

// WithSystemPrompt sets the system prompt for the agent config
func (c AgentConfig) WithSystemPrompt(prompt string) AgentConfig {
	c.SystemPrompt = prompt
	return c
}

// WithTools sets the tool policy for the agent config
func (c AgentConfig) WithTools(allow, deny []string) AgentConfig {
	c.Tools = ToolPolicy{
		Allow: allow,
		Deny:  deny,
	}
	return c
}

// WithWorkspace sets the workspace for the agent config
func (c AgentConfig) WithWorkspace(workspace string) AgentConfig {
	c.Workspace = workspace
	return c
}

// WithSandbox sets the sandbox configuration for the agent config
func (c AgentConfig) WithSandbox(mode SandboxMode, scope SandboxScope) AgentConfig {
	c.Sandbox = SandboxConfig{
		Mode:  mode,
		Scope: scope,
	}
	return c
}

// WithMaxConcurrentSubAgents sets the max concurrent sub-agents for the agent config
func (c AgentConfig) WithMaxConcurrentSubAgents(max int) AgentConfig {
	c.MaxConcurrentSubAgents = max
	return c
}

// WithAllowedSubAgents sets the sub-agent allowlist for the agent config.
func (c AgentConfig) WithAllowedSubAgents(allowed []string) AgentConfig {
	if allowed == nil {
		c.AllowedSubAgents = nil
		return c
	}
	c.AllowedSubAgents = append([]string{}, allowed...)
	return c
}

// WithMetadata sets metadata for the agent config
func (c AgentConfig) WithMetadata(key, value string) AgentConfig {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
	return c
}

// ToJSON converts the agent config to JSON
func (c AgentConfig) ToJSON() (string, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal agent config: %w", err)
	}
	return string(data), nil
}

// FromJSON creates an agent config from JSON
func FromJSON(data string) (AgentConfig, error) {
	var config AgentConfig
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return AgentConfig{}, fmt.Errorf("failed to unmarshal agent config: %w", err)
	}
	return config, nil
}

// Clone creates a deep copy of the agent config
func (c AgentConfig) Clone() AgentConfig {
	clone := c

	// Deep copy slices
	if c.Tools.Allow != nil {
		clone.Tools.Allow = make([]string, len(c.Tools.Allow))
		copy(clone.Tools.Allow, c.Tools.Allow)
	}

	if c.Tools.Deny != nil {
		clone.Tools.Deny = make([]string, len(c.Tools.Deny))
		copy(clone.Tools.Deny, c.Tools.Deny)
	}
	if c.AllowedSubAgents != nil {
		clone.AllowedSubAgents = make([]string, len(c.AllowedSubAgents))
		copy(clone.AllowedSubAgents, c.AllowedSubAgents)
	}

	// Deep copy metadata
	if c.Metadata != nil {
		clone.Metadata = make(map[string]string, len(c.Metadata))
		for k, v := range c.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// IsToolAllowed checks if a tool is allowed by the agent's tool policy
func (c AgentConfig) IsToolAllowed(toolName string) bool {
	// Check deny list first (overrides allow list)
	for _, denied := range c.Tools.Deny {
		if denied == toolName || denied == "*" {
			return false
		}
	}

	// Check allow list
	for _, allowed := range c.Tools.Allow {
		if allowed == toolName || allowed == "*" {
			return true
		}
	}

	// If no explicit allow, deny by default
	return false
}

// IsSandboxEnabled checks if sandboxing is enabled for the agent
func (c AgentConfig) IsSandboxEnabled() bool {
	return c.Sandbox.Mode != SandboxModeOff
}
