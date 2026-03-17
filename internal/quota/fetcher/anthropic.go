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
	client *http.Client
	logger *logrus.Logger
}

// NewAnthropicFetcher 创建 Anthropic fetcher
func NewAnthropicFetcher(logger *logrus.Logger) *AnthropicFetcher {
	return &AnthropicFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
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

	// 检查认证
	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}

	// 检查是否过期
	if provider.IsOAuthExpired() {
		return fmt.Errorf("OAuth token is expired")
	}

	return nil
}

// anthropicUsageResponse Anthropic API 响应
type anthropicUsageResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Current cycle (session) - 5 hour window
	CurrentCycle struct {
		Usage          float64 `json:"usage"`
		MaxUsage       float64 `json:"max_usage"`
		CycleTimeHours float64 `json:"cycle_time_hours"`
	} `json:"current_cycle"`

	// Weekly quota
	Quota struct {
		Usage          float64 `json:"usage"`
		MaxUsage       float64 `json:"max_usage"`
		UsageWindowHours float64 `json:"usage_window_hours"`
	} `json:"quota"`

	// Sonnet specific quota
	ExtraUsage struct {
		Usage    float64 `json:"usage"`
		MaxUsage float64 `json:"max_usage"`
	} `json:"extra_usage"`

	// Monthly cost (in USD)
	Trial struct {
		Usage    float64 `json:"usage"`
		MaxUsage float64 `json:"max_usage"`
	} `json:"trial"`
}

func (f *AnthropicFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// 构建请求
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Anthropic-Version", "2023-06-01")

	// 发送请求
	resp, err := f.client.Do(req)
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
	sessionWindowMins := int(apiResp.CurrentCycle.CycleTimeHours * 60)
	sessionResetsAt := now.Add(time.Duration(apiResp.CurrentCycle.CycleTimeHours) * time.Hour)

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeAnthropic,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute), // 5分钟缓存

		// Primary: Session quota (5-hour window)
		Primary: &quota.UsageWindow{
			Type:          quota.WindowTypeSession,
			Used:          apiResp.CurrentCycle.Usage,
			Limit:         apiResp.CurrentCycle.MaxUsage,
			Unit:          quota.UsageUnitTokens,
			ResetsAt:      &sessionResetsAt,
			WindowMinutes: sessionWindowMins,
			Label:         "Session Quota",
			Description:   "5-hour rolling window token quota",
		},

		// Secondary: Weekly quota
		Secondary: &quota.UsageWindow{
			Type:   quota.WindowTypeWeekly,
			Used:   apiResp.Quota.Usage,
			Limit:  apiResp.Quota.MaxUsage,
			Unit:   quota.UsageUnitTokens,
			Label:  "Weekly Quota",
			Description: "Weekly token quota",
		},

		// Tertiary: Sonnet specific quota
		Tertiary: &quota.UsageWindow{
			Type:   quota.WindowTypeCustom,
			Used:   apiResp.ExtraUsage.Usage,
			Limit:  apiResp.ExtraUsage.MaxUsage,
			Unit:   quota.UsageUnitTokens,
			Label:  "Sonnet Quota",
			Description: "Sonnet model specific quota",
		},

		// Cost: Monthly usage
		Cost: &quota.UsageCost{
			Used:         apiResp.Trial.Usage,
			Limit:        apiResp.Trial.MaxUsage,
			CurrencyCode: "USD",
			Label:        "Monthly Usage",
		},

		// Account info
		Account: &quota.UsageAccount{
			ID:   apiResp.ID,
			Name: apiResp.Name,
			Tier: "standard",
		},
	}

	// 计算百分比
	usage.Primary.UsedPercent = usage.Primary.CalculateUsedPercent()
	usage.Secondary.UsedPercent = usage.Secondary.CalculateUsedPercent()
	usage.Tertiary.UsedPercent = usage.Tertiary.CalculateUsedPercent()

	return usage, nil
}
