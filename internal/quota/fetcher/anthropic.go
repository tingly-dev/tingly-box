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

// AnthropicFetcher Anthropic (Claude) 配额获取器
type AnthropicFetcher struct {
	logger *logrus.Logger
}

// NewAnthropicFetcher 创建 Anthropic fetcher
func NewAnthropicFetcher(logger *logrus.Logger) *AnthropicFetcher {
	return &AnthropicFetcher{
		logger: logger,
	}
}

func (f *AnthropicFetcher) Name() string {
	return "anthropic"
}

func (f *AnthropicFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeAnthropic
}

func (f *AnthropicFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeOAuth
}

func (f *AnthropicFetcher) Validate(provider *typ.Provider) error {
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

// anthropicUsageResponse Anthropic OAuth usage API 响应
// Endpoint: GET https://api.anthropic.com/api/oauth/usage
// Header: Authorization: Bearer <token>, anthropic-beta: oauth-2025-04-20
type anthropicUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"` // 0-100 percentage
		ResetsAt    *string `json:"resets_at"`   // ISO 8601
	} `json:"five_hour"`

	SevenDay struct {
		Utilization float64 `json:"utilization"` // 0-100 percentage
		ResetsAt    *string `json:"resets_at"`   // ISO 8601
	} `json:"seven_day"`

	ExtraUsage struct {
		IsEnabled    bool    `json:"is_enabled"`
		Utilization  float64 `json:"utilization"`   // 0-100 percentage
		UsedCredits  float64 `json:"used_credits"`  // in cents
		MonthlyLimit float64 `json:"monthly_limit"` // in cents
	} `json:"extra_usage"`
}

func (f *AnthropicFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// 创建带 proxy 支持的 HTTP client
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// 构建请求
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应
	var apiResp anthropicUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 转换为统一格式
	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeAnthropic,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),

		// Primary: 5-hour session quota
		Primary: &quota.UsageWindow{
			Type:          quota.WindowTypeSession,
			Used:          apiResp.FiveHour.Utilization,
			Limit:         100,
			UsedPercent:   apiResp.FiveHour.Utilization,
			Unit:          quota.UsageUnitRequests,
			WindowMinutes: 300, // 5 hours
			Label:         "Current Window",
			Description:   "5-hour rolling window utilization",
		},

		// Secondary: 7-day weekly quota
		Secondary: &quota.UsageWindow{
			Type:        quota.WindowTypeWeekly,
			Used:        apiResp.SevenDay.Utilization,
			Limit:       100,
			UsedPercent: apiResp.SevenDay.Utilization,
			Unit:        quota.UsageUnitRequests,
			Label:       "Weekly",
			Description: "7-day rolling window utilization",
		},
	}

	// 解析 resets_at 时间
	if apiResp.FiveHour.ResetsAt != nil {
		if t, err := time.Parse(time.RFC3339, *apiResp.FiveHour.ResetsAt); err == nil {
			usage.Primary.ResetsAt = &t
		}
	}
	if apiResp.SevenDay.ResetsAt != nil {
		if t, err := time.Parse(time.RFC3339, *apiResp.SevenDay.ResetsAt); err == nil {
			usage.Secondary.ResetsAt = &t
		}
	}

	// Tertiary: Extra usage (Max plan add-on), only if enabled
	if apiResp.ExtraUsage.IsEnabled {
		usage.Tertiary = &quota.UsageWindow{
			Type:        quota.WindowTypeMonthly,
			Used:        apiResp.ExtraUsage.Utilization,
			Limit:       100,
			UsedPercent: apiResp.ExtraUsage.Utilization,
			Unit:        quota.UsageUnitRequests,
			Label:       "Extra",
			Description: "Max plan extra usage",
		}

		usage.Cost = &quota.UsageCost{
			Used:         apiResp.ExtraUsage.UsedCredits / 100,  // cents → dollars
			Limit:        apiResp.ExtraUsage.MonthlyLimit / 100, // cents → dollars
			CurrencyCode: "USD",
			Label:        "Extra Usage",
		}
	}

	return usage, nil
}
