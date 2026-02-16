package orchestrator

import (
	"errors"
	"fmt"
)

// AgentRole defines the role of an agent in the orchestration
type AgentRole string

const (
	RoleCaptain  AgentRole = "captain"  // Coordinator and decision-maker
	RoleExecutor AgentRole = "executor" // Task execution specialist
	RoleCritic   AgentRole = "critic"   // Quality assurance and review
	RoleGeneral  AgentRole = "general"  // General purpose agent
)

// AgentConfig defines the configuration for an agent
type AgentConfig struct {
	ID                     string            `json:"id" yaml:"id"`
	Name                   string            `json:"name" yaml:"name"`
	Role                   AgentRole         `json:"role" yaml:"role"`
	Model                  string            `json:"model" yaml:"model"`
	Temperature            float64           `json:"temperature" yaml:"temperature"`
	MaxTokens              int               `json:"max_tokens" yaml:"max_tokens"`
	SystemPrompt           string            `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Tools                  ToolPolicy        `json:"tools" yaml:"tools"`
	Workspace              string            `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Sandbox                SandboxConfig     `json:"sandbox" yaml:"sandbox"`
	MaxConcurrentSubAgents int               `json:"max_concurrent_sub_agents" yaml:"max_concurrent_sub_agents"`
	Metadata               map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ToolPolicy defines which tools an agent can use
type ToolPolicy struct {
	Allow []string `json:"allow" yaml:"allow"` // List of allowed tools (* for all)
	Deny  []string `json:"deny" yaml:"deny"`   // List of denied tools (overrides allow)
}

// SandboxConfig defines sandbox settings for an agent
type SandboxConfig struct {
	Mode  SandboxMode  `json:"mode" yaml:"mode"`   // off, all, tools
	Scope SandboxScope `json:"scope" yaml:"scope"` // agent, session
}

// SandboxMode defines how sandboxing is applied
type SandboxMode string

const (
	SandboxModeOff   SandboxMode = "off"   // No sandboxing
	SandboxModeAll   SandboxMode = "all"   // All operations sandboxed
	SandboxModeTools SandboxMode = "tools" // Only tool execution sandboxed
)

// SandboxScope defines the scope of sandboxing
type SandboxScope string

const (
	SandboxScopeAgent   SandboxScope = "agent"   // Per-agent sandbox
	SandboxScopeSession SandboxScope = "session" // Per-session sandbox
)

// Validate validates the agent configuration
func (c AgentConfig) Validate() error {
	if c.ID == "" {
		return errors.New("agent ID is required")
	}

	if c.Name == "" {
		return errors.New("agent name is required")
	}

	if c.Model == "" {
		return errors.New("agent model is required")
	}

	if c.Role != RoleCaptain && c.Role != RoleExecutor && c.Role != RoleCritic && c.Role != RoleGeneral {
		return fmt.Errorf("invalid agent role: %s", c.Role)
	}

	if c.Temperature < 0 || c.Temperature > 1 {
		return fmt.Errorf("temperature must be between 0 and 1, got: %f", c.Temperature)
	}

	if c.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be positive, got: %d", c.MaxTokens)
	}

	if c.MaxConcurrentSubAgents < 0 {
		return fmt.Errorf("max_concurrent_sub_agents must be non-negative, got: %d", c.MaxConcurrentSubAgents)
	}

	return nil
}

// JoinStrategy defines how to wait for parallel agent execution
type JoinStrategy string

const (
	JoinAll   JoinStrategy = "all"   // Wait for all agents to complete
	JoinFirst JoinStrategy = "first" // Wait for first agent to complete
	JoinAny   JoinStrategy = "any"   // Wait for any agent to complete
)

// OnFailStrategy defines how to handle failures in parallel execution
type OnFailStrategy string

const (
	OnFailAbort    OnFailStrategy = "abort"    // Abort all on first failure
	OnFailContinue OnFailStrategy = "continue" // Continue despite failures
)

// AgentContext defines the context passed between agents
type AgentContext struct {
	ParentSessionKey string                 `json:"parent_session_key"`
	SharedState      map[string]interface{} `json:"shared_state"`
	Files            []string               `json:"files"`
	Instructions     string                 `json:"instructions"`
	TraceID          string                 `json:"trace_id"`
	RunID            string                 `json:"run_id"`
}

// AgentResult represents the result of an agent execution
type AgentResult struct {
	InstanceID string                 `json:"instance_id"`
	AgentID    string                 `json:"agent_id"`
	Success    bool                   `json:"success"`
	Output     interface{}            `json:"output"`
	Error      string                 `json:"error,omitempty"`
	Duration   int64                  `json:"duration_ms"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// SpawnRequest represents a request to spawn a sub-agent
type SpawnRequest struct {
	AgentID string       `json:"agent_id"`
	Context AgentContext `json:"context"`
	Timeout int          `json:"timeout_seconds"` // Default 300s
}

// ParallelRequest represents a request for parallel agent execution
type ParallelRequest struct {
	Requests     []SpawnRequest `json:"requests"`
	JoinStrategy JoinStrategy   `json:"join_strategy"`
	OnFail       OnFailStrategy `json:"on_fail"`
}
