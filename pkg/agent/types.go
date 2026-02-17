package agent

import (
	"time"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// AgentRunParams contains input parameters for agent execution
type AgentRunParams struct {
	Prompt        string                     `json:"prompt"`
	SessionKey    string                     `json:"session_key"`
	Config        AgentConfig                `json:"config"`
	CWD           string                     `json:"cwd,omitempty"`
	AgentID       string                     `json:"agent_id,omitempty"`
	ToolPolicy    *toolexecutor.ToolPolicy   `json:"tool_policy,omitempty"`
	SandboxPolicy map[string]interface{}     `json:"sandbox_policy,omitempty"`
}

// AgentConfig configures agent behavior
type AgentConfig struct {
	Model        string   `json:"model"`
	Temperature  float64  `json:"temperature,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	UseMemory    bool     `json:"use_memory,omitempty"`
	Streaming    bool     `json:"streaming,omitempty"`
	MaxRetries   int      `json:"max_retries,omitempty"`
}

// AgentResult contains output from agent execution
type AgentResult struct {
	Response   string      `json:"response"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	Usage      *TokenUsage `json:"usage,omitempty"`
	SessionKey string      `json:"session_key"`
	Aborted    bool        `json:"aborted,omitempty"`
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

// TokenUsage tracks token consumption
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AuthProfile represents authentication credentials for LLM providers
type AuthProfile struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"` // "anthropic", "openai", "gemini"
	APIKey        string `json:"api_key"`
	CooldownUntil *int64 `json:"cooldown_until,omitempty"`
	FailureCount  int    `json:"failure_count"`
	Priority      int    `json:"priority"`
}

// AgentMessage represents a message in the conversation
type AgentMessage struct {
	Role       string                 `json:"role"`
	Content    string                 `json:"content"`
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// StreamChunk represents a streaming response chunk
type StreamChunk struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// DefaultConfig returns default agent configuration
func DefaultConfig() AgentConfig {
	return AgentConfig{
		Model:       "claude-3-5-sonnet-20241022",
		Temperature: 0.7,
		MaxTokens:   4096,
		MaxRetries:  3,
	}
}

// IsRetryableError checks if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Network errors
	if contains(errMsg, "ECONNRESET") || contains(errMsg, "ETIMEDOUT") {
		return true
	}

	// Rate limits
	if contains(errMsg, "429") || contains(errMsg, "rate limit") {
		return true
	}

	// Server errors
	if contains(errMsg, "500") || contains(errMsg, "502") || contains(errMsg, "503") || contains(errMsg, "504") {
		return true
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// EstimateTokens provides a rough token count estimation
func EstimateTokens(messages []AgentMessage) int {
	totalChars := 0
	for _, msg := range messages {
		totalChars += len(msg.Content)
	}
	// Rough estimation: 1 token ≈ 4 characters
	return (totalChars + 3) / 4
}

// Sleep helper for retries
func Sleep(d time.Duration) {
	time.Sleep(d)
}
