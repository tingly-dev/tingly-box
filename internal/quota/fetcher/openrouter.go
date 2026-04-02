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

// OpenRouterFetcher OpenRouter 配额获取器
// Uses: GET https://openrouter.ai/api/v1/key (key info with usage)
type OpenRouterFetcher struct {
	logger *logrus.Logger
}

func NewOpenRouterFetcher(logger *logrus.Logger) *OpenRouterFetcher {
	return &OpenRouterFetcher{logger: logger}
}

func (f *OpenRouterFetcher) Name() string                     { return "openrouter" }
func (f *OpenRouterFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeOpenRouter }
func (f *OpenRouterFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeAPIKey }

func (f *OpenRouterFetcher) Validate(provider *typ.Provider) error {
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

func (f *OpenRouterFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/key", nil)
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

	var keyResp openrouterKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
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

	// Primary: usage vs limit (if limit is set)
	if data.Limit != nil && *data.Limit > 0 {
		used := data.Usage
		limit := *data.Limit
		usage.Primary = &quota.UsageWindow{
			Type:        quota.WindowTypeBalance,
			Used:        used,
			Limit:       limit,
			UsedPercent: calcPercent(used, limit),
			Unit:        quota.UsageUnitCurrency,
			Label:       "Key Limit",
			Description: fmt.Sprintf("Balance: $%.2f / $%.2f", limit-used, limit),
		}
	} else {
		// No limit set — show monthly usage as primary
		usage.Primary = &quota.UsageWindow{
			Type:        quota.WindowTypeMonthly,
			Used:        data.UsageMonthly,
			Limit:       0,
			UsedPercent: 0,
			Unit:        quota.UsageUnitCurrency,
			Label:       "Monthly Usage",
			Description: fmt.Sprintf("This month: $%.4f (no limit set)", data.UsageMonthly),
		}
	}

	// Secondary: monthly usage breakdown
	usage.Secondary = &quota.UsageWindow{
		Type:        quota.WindowTypeMonthly,
		Used:        data.UsageMonthly,
		Limit:       0,
		UsedPercent: 0,
		Unit:        quota.UsageUnitCurrency,
		Label:       "Monthly",
		Description: fmt.Sprintf("Daily: $%.4f | Weekly: $%.4f | Monthly: $%.4f | Total: $%.4f",
			data.UsageDaily, data.UsageWeekly, data.UsageMonthly, data.Usage),
	}

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
