package memory

import (
	"context"
)

// MockEmbeddingProvider for testing (generates deterministic embeddings)
type MockEmbeddingProvider struct {
	dimension int
}

func NewMockEmbeddingProvider(dimension int) *MockEmbeddingProvider {
	return &MockEmbeddingProvider{dimension: dimension}
}

func (p *MockEmbeddingProvider) Dimension() int {
	return p.dimension
}

func (p *MockEmbeddingProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Generate deterministic embedding based on text hash
	embedding := make([]float32, p.dimension)
	hash := 0
	for _, c := range text {
		hash = hash*31 + int(c)
	}

	for i := 0; i < p.dimension; i++ {
		embedding[i] = float32((hash+i)%100) / 100.0
	}

	return embedding, nil
}

func (p *MockEmbeddingProvider) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := p.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}
