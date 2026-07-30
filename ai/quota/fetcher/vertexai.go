package fetcher

import (
	"context"
	"fmt"

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
	return unreadableUsage(provider, quota.ProviderTypeVertexAI, "quota API not available - check Google Cloud Console"), nil
}
