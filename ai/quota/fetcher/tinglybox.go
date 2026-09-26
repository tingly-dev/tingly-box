package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// TinglyBoxFetcher reads the quota a central tingly-box shares with this
// provider's credential, from GET /tingly/<scenario>/quota on the same host
// the provider already sends model traffic to. See .design/quota-relay.md.
type TinglyBoxFetcher struct{}

func NewTinglyBoxFetcher() *TinglyBoxFetcher {
	return &TinglyBoxFetcher{}
}

func (f *TinglyBoxFetcher) Name() string                     { return "tingly_box" }
func (f *TinglyBoxFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeTinglyBox }
func (f *TinglyBoxFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *TinglyBoxFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if _, ok := quota.GatewayQuotaURL(provider.APIBase); !ok {
		return fmt.Errorf("API base is not a tingly-box route")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

func (f *TinglyBoxFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	if err := f.Validate(provider); err != nil {
		return nil, err
	}
	quotaURL, _ := quota.GatewayQuotaURL(provider.APIBase)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, quotaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")

	resp, err := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		// The upstream team keeps its quota private; say so plainly rather
		// than as a generic failure, so the user knows whom to ask.
		return nil, fmt.Errorf("upstream team does not share quota with this key")
	case http.StatusNotFound:
		return nil, fmt.Errorf("upstream tingly-box does not serve quota (upgrade it to share quota)")
	default:
		return nil, fmt.Errorf("unexpected status code: %d%s", resp.StatusCode, errorDetail(body))
	}

	var gq quota.GatewayQuota
	if err := json.Unmarshal(body, &gq); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	usage := quota.GatewayUsage(&gq)
	usage.ProviderUUID = provider.UUID
	usage.ProviderName = provider.Name
	usage.ProviderType = quota.ProviderTypeTinglyBox
	usage.FetchedAt = time.Now()
	usage.ExpiresAt = usage.FetchedAt.Add(5 * time.Minute)
	usage.RawResponse = body
	return usage, nil
}
