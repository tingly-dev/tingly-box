package fetcher

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// MiniMaxCNFetcher retrieves MiniMax CN quota data.
// Uses: GET https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains
type MiniMaxCNFetcher struct{}

func NewMiniMaxCNFetcher() *MiniMaxCNFetcher {
	return &MiniMaxCNFetcher{}
}

func (f *MiniMaxCNFetcher) Name() string                     { return "minimax-cn" }
func (f *MiniMaxCNFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeMiniMaxCN }
func (f *MiniMaxCNFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *MiniMaxCNFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// ── Fetch ──────────────────────────────────────────────

func (f *MiniMaxCNFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	return fetchMiniMaxQuota(ctx, provider, "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains", quota.ProviderTypeMiniMaxCN)
}
