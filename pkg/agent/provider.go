package agent

import (
	"context"
	"fmt"
)

// LLMProvider is an interface for LLM API providers
type LLMProvider interface {
	// Call makes an LLM API call
	Call(ctx context.Context, request LLMRequest) (*LLMResponse, error)

	// Provider returns the provider name
	Provider() string
}

// LLMRequest contains the request parameters for LLM call
type LLMRequest struct {
	Model        string
	Messages     []AgentMessage
	Tools        []interface{}
	Temperature  float64
	MaxTokens    int
	SystemPrompt string
}

// LLMResponse contains the response from LLM
type LLMResponse struct {
	Content   string
	ToolCalls []ToolCall
	Usage     *TokenUsage
}

// ProviderFactory creates LLM providers
type ProviderFactory struct{}

// NewProvider creates a new LLM provider based on auth profile
func (f *ProviderFactory) NewProvider(profile AuthProfile) (LLMProvider, error) {
	switch profile.Provider {
	case "anthropic":
		return NewAnthropicProvider(profile.APIKey), nil
	case "openai":
		return NewOpenAIProvider(profile.APIKey), nil
	case "gemini":
		return NewGeminiProvider(profile.APIKey), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", profile.Provider)
	}
}
