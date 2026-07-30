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

// OpenRouterFetcher retrieves OpenRouter quota data.
// Uses: GET https://openrouter.ai/api/v1/key (key info with usage)
type OpenRouterFetcher struct{}

func NewOpenRouterFetcher() *OpenRouterFetcher {
	return &OpenRouterFetcher{}
}

func (f *OpenRouterFetcher) Name() string                     { return "openrouter" }
func (f *OpenRouterFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeOpenRouter }
func (f *OpenRouterFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *OpenRouterFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// openrouterKeyResponse from GET /api/v1/key
type openrouterKeyResponse struct {
	Data struct {
		Label            string   `json:"label"`
		IsFreeTier       bool     `json:"is_free_tier"`
		Limit            *float64 `json:"limit"`           // nullable
		LimitRemaining   *float64 `json:"limit_remaining"` // nullable
		Usage            float64  `json:"usage"`
		UsageDaily       float64  `json:"usage_daily"`
		UsageWeekly      float64  `json:"usage_weekly"`
		UsageMonthly     float64  `json:"usage_monthly"`
		ByokUsage        float64  `json:"byok_usage"`
		ByokUsageDaily   float64  `json:"byok_usage_daily"`
		ByokUsageWeekly  float64  `json:"byok_usage_weekly"`
		ByokUsageMonthly float64  `json:"byok_usage_monthly"`
		ExpiresAt        *string  `json:"expires_at"`
		CreatorUserID    string   `json:"creator_user_id"`
	} `json:"data"`
}

func (f *OpenRouterFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Use provider.APIBase for testing, fallback to production URL
	apiBase := provider.APIBase
	if apiBase == "" {
		apiBase = "https://openrouter.ai"
	}
	url := apiBase + "/api/v1/key"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var keyResp openrouterKeyResponse
	if err := json.Unmarshal(bodyBytes, &keyResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	data := keyResp.Data
	now := time.Now()

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenRouter,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(bodyBytes),
	}

	// Account info
	tier := "paid"
	if data.IsFreeTier {
		tier = "free"
	}
	usage.Account = &quota.UsageAccount{
		ID:   data.CreatorUserID,
		Tier: tier,
	}

	// A key limit is a prepaid credit pool: spending it down is permanent
	// until the key is topped up, so it is a resource rather than an
	// allowance that comes back on its own.
	if data.Limit != nil && *data.Limit > 0 {
		used := data.Usage
		limit := *data.Limit
		usage.AddWindow("key_limit", &quota.UsageWindow{
			Type:        quota.WindowTypeBalance,
			Kind:        quota.WindowKindResource,
			Used:        used,
			Limit:       limit,
			Unit:        quota.UsageUnitCurrency,
			Label:       "Key Limit",
			Description: fmt.Sprintf("Balance: $%.2f / $%.2f", limit-used, limit),
		})
	}

	// Monthly spend is never capped — the key limit caps lifetime usage, not
	// the month — so this window has no cap in either branch and says so.
	// A bare Limit of 0 would be read as a budget with nothing spent.
	monthly := &quota.UsageWindow{
		Type:          quota.WindowTypeMonthly,
		Used:          data.UsageMonthly,
		Unlimited:     true,
		Unit:          quota.UsageUnitCurrency,
		WindowMinutes: 30 * 24 * 60,
		Label:         "Monthly",
		Description: fmt.Sprintf("Daily: $%.4f | Weekly: $%.4f | Monthly: $%.4f | Total: $%.4f",
			data.UsageDaily, data.UsageWeekly, data.UsageMonthly, data.Usage),
	}
	if data.Limit == nil || *data.Limit <= 0 {
		monthly.Label = "Monthly Usage"
		monthly.Description = fmt.Sprintf("This month: $%.4f (no limit set)", data.UsageMonthly)
	}
	usage.AddWindow("monthly", monthly)

	// Cost
	usage.Cost = &quota.UsageCost{
		Used:         data.Usage,
		CurrencyCode: "USD",
		Label:        "Total Usage",
	}
	if data.Limit != nil {
		usage.Cost.Limit = *data.Limit
	}

	return usage, nil
}
