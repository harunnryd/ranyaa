package agent

import (
	"context"
	"fmt"
)

// GeminiProvider implements LLMProvider for Google Gemini
type GeminiProvider struct {
	apiKey string
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{
		apiKey: apiKey,
	}
}

// Provider returns the provider name
func (p *GeminiProvider) Provider() string {
	return "gemini"
}

// Call makes an API call to Google Gemini
func (p *GeminiProvider) Call(ctx context.Context, request LLMRequest) (*LLMResponse, error) {
	// Gemini integration is not available yet in this provider.
	return nil, fmt.Errorf("gemini provider not yet implemented - use anthropic or openai")
}
