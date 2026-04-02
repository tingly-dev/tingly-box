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

// CodexFetcher OpenAI Codex 配额获取器
// Uses: GET https://chatgpt.com/backend-api/wham/usage
// Requires OAuth access_token + optional account_id (from oauth_detail.extra_fields)
type CodexFetcher struct {
	logger *logrus.Logger
}

func NewCodexFetcher(logger *logrus.Logger) *CodexFetcher {
	return &CodexFetcher{logger: logger}
}

func (f *CodexFetcher) Name() string                     { return "codex" }
func (f *CodexFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeCodex }
func (f *CodexFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeOAuth }

func (f *CodexFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}
	return nil
}

// ── API response ───────────────────────────────────────

// codexUsageResponse from GET /backend-api/wham/usage
type codexUsageResponse struct {
	PlanType  string `json:"plan_type"` // guest, free, go, plus, pro, team, business, enterprise
	RateLimit *struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	Credits *struct {
		HasCredits bool    `json:"has_credits"`
		Unlimited  bool    `json:"unlimited"`
		Balance    float64 `json:"balance"`
	} `json:"credits"`
}

type codexWindow struct {
	UsedPercent        int   `json:"used_percent"`
	ResetAt            int64 `json:"reset_at"`             // unix epoch
	LimitWindowSeconds int   `json:"limit_window_seconds"` // window duration in seconds
}

// ── Fetch ──────────────────────────────────────────────

func (f *CodexFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Resolve account_id from OAuth extra_fields
	var accountID string
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if aid, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok {
			accountID = aid
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-cli")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var apiResp codexUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeCodex,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		Account: &quota.UsageAccount{
			Tier: apiResp.PlanType,
		},
	}

	if apiResp.RateLimit != nil {
		if w := apiResp.RateLimit.PrimaryWindow; w != nil {
			resetsAt := time.Unix(w.ResetAt, 0)
			usage.Primary = &quota.UsageWindow{
				Type:          quota.WindowTypeSession,
				Used:          float64(w.UsedPercent),
				Limit:         100,
				UsedPercent:   float64(w.UsedPercent),
				Unit:          quota.UsageUnitRequests,
				ResetsAt:      &resetsAt,
				WindowMinutes: w.LimitWindowSeconds / 60,
				Label:         "Current Window",
				Description:   fmt.Sprintf("%dh window", w.LimitWindowSeconds/3600),
			}
		}
		if w := apiResp.RateLimit.SecondaryWindow; w != nil {
			resetsAt := time.Unix(w.ResetAt, 0)
			usage.Secondary = &quota.UsageWindow{
				Type:          quota.WindowTypeWeekly,
				Used:          float64(w.UsedPercent),
				Limit:         100,
				UsedPercent:   float64(w.UsedPercent),
				Unit:          quota.UsageUnitRequests,
				ResetsAt:      &resetsAt,
				WindowMinutes: w.LimitWindowSeconds / 60,
				Label:         "Weekly",
				Description:   fmt.Sprintf("%dd window", w.LimitWindowSeconds/86400),
			}
		}
	}

	if apiResp.Credits != nil && apiResp.Credits.HasCredits && !apiResp.Credits.Unlimited {
		usage.Cost = &quota.UsageCost{
			Used:         0, // API doesn't report used amount directly
			Limit:        apiResp.Credits.Balance,
			CurrencyCode: "USD",
			Label:        "Credits Balance",
		}
	}

	return usage, nil
}
