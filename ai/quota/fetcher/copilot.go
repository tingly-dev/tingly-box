package fetcher

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// CopilotFetcher retrieves GitHub Copilot quota data.
type CopilotFetcher struct{}

// NewCopilotFetcher creates a Copilot quota fetcher.
func NewCopilotFetcher() *CopilotFetcher {
	return &CopilotFetcher{}
}

func (f *CopilotFetcher) Name() string {
	return "copilot"
}

func (f *CopilotFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCopilot
}

func (f *CopilotFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeOAuth
}

func (f *CopilotFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}

	if provider.IsOAuthExpired() {
		return fmt.Errorf("OAuth token is expired")
	}

	return nil
}

func (f *CopilotFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	// GitHub Copilot has no public quota API, so return fallback data.
	return unreadableUsage(provider, quota.ProviderTypeCopilot, "quota API not available"), nil
}
