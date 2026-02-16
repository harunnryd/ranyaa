package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingProvider generates vector embeddings from text
type EmbeddingProvider interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// OpenAIProvider implements EmbeddingProvider for OpenAI
type OpenAIProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI embedding provider
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	dimension := 1536 // text-embedding-ada-002 default
	if model == "text-embedding-3-small" {
		dimension = 1536
	} else if model == "text-embedding-3-large" {
		dimension = 3072
	}

	return &OpenAIProvider{
		apiKey:    apiKey,
		model:     model,
		dimension: dimension,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *OpenAIProvider) Dimension() int {
	return p.dimension
}

func (p *OpenAIProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.GenerateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (p *OpenAIProvider) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := map[string]interface{}{
		"input": texts,
		"model": p.model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for i, data := range result.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}
