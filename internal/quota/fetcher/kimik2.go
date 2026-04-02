package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// KimiK2Fetcher Kimi K2 (Moonshot) 配额获取器
// Uses: GET https://kimi-k2.ai/api/user/credits
type KimiK2Fetcher struct {
	logger *logrus.Logger
}

func NewKimiK2Fetcher(logger *logrus.Logger) *KimiK2Fetcher {
	return &KimiK2Fetcher{logger: logger}
}

func (f *KimiK2Fetcher) Name() string                     { return "kimi_k2" }
func (f *KimiK2Fetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeKimiK2 }
func (f *KimiK2Fetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeAPIKey }

func (f *KimiK2Fetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// ── API response ───────────────────────────────────────

type kimiCreditsResponse struct {
	Consumed  float64 `json:"consumed"`
	Remaining float64 `json:"remaining"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *KimiK2Fetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "https://kimi-k2.ai/api/user/credits", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Try JSON body
	var body kimiCreditsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		// Fallback: check X-Credits-Remaining header
		if hdr := resp.Header.Get("X-Credits-Remaining"); hdr != "" {
			var remaining float64
			fmt.Sscanf(hdr, "%f", &remaining)
			body.Remaining = remaining
		}
		if body.Remaining == 0 {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	}

	consumed := body.Consumed
	remaining := body.Remaining
	total := consumed + remaining
	usedPercent := calcPercent(consumed, total)

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeKimiK2,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		Primary: &quota.UsageWindow{
			Type:        quota.WindowTypeBalance,
			Used:        consumed,
			Limit:       total,
			UsedPercent: usedPercent,
			Unit:        quota.UsageUnitCredits,
			Label:       "Credits",
			Description: fmt.Sprintf("%.0f consumed, %.0f remaining", consumed, remaining),
		},
	}

	return usage, nil
}
