package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// VertexAIFetcher retrieves Google Vertex AI quota data.
type VertexAIFetcher struct{}

// NewVertexAIFetcher creates a Vertex AI quota fetcher.
func NewVertexAIFetcher() *VertexAIFetcher {
	return &VertexAIFetcher{}
}

func (f *VertexAIFetcher) Name() string {
	return "vertex_ai"
}

func (f *VertexAIFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeVertexAI
}

func (f *VertexAIFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeAPIKey
}

func (f *VertexAIFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *VertexAIFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	// Vertex AI quotas are managed through Google Cloud Console and have no public API.
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage creates fallback quota data.
func (f *VertexAIFetcher) createDefaultUsage(provider *ai.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeVertexAI,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available - check Google Cloud Console",
		LastErrorAt:  &now,
	}
}
