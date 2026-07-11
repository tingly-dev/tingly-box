package fetcher

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// MiniMaxFetcher retrieves MiniMax quota data.
// Uses: GET https://api.minimax.io/v1/api/openplatform/coding_plan/remains
type MiniMaxFetcher struct{}

func NewMiniMaxFetcher() *MiniMaxFetcher {
	return &MiniMaxFetcher{}
}

func (f *MiniMaxFetcher) Name() string                     { return "minimax" }
func (f *MiniMaxFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeMiniMax }
func (f *MiniMaxFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *MiniMaxFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

func (f *MiniMaxFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	return fetchMiniMaxQuota(ctx, provider, "https://api.minimax.io/v1/api/openplatform/coding_plan/remains", quota.ProviderTypeMiniMax)
}
