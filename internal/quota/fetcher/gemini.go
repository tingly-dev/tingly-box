package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GeminiFetcher Google Gemini 配额获取器
type GeminiFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewGeminiFetcher 创建 Gemini fetcher
func NewGeminiFetcher(logger *logrus.Logger) *GeminiFetcher {
	return &GeminiFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *GeminiFetcher) Name() string {
	return "gemini"
}

func (f *GeminiFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeGemini
}

func (f *GeminiFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeOAuth
}

func (f *GeminiFetcher) Validate(provider *typ.Provider) error {
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

// geminiUsageRequest Gemini API 请求
type geminiUsageRequest struct {
	_              string `json:"-"` // ignore
	RequestedQuota string `json:"requestedQuota"`
}

// geminiUsageResponse Gemini API 响应
type geminiUsageResponse struct {
	UserQuota struct {
		ModelQuotas []struct {
			ModelName       string  `json:"modelName"`
			QuotaLimit      float64 `json:"quotaLimit"`
			QuotaUsed       float64 `json:"quotaUsed"`
			WindowSizeHours float64 `json:"windowSizeHours"`
			Tier            string  `json:"tier"`
		} `json:"modelQuotas"`
	} `json:"userQuota"`
	Tier             string `json:"tier"`
	WorkspaceQuotas  []any   `json:"workspaceQuotas"`
}

func (f *GeminiFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// 构建请求
	reqBody := geminiUsageRequest{
		RequestedQuota: "",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

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
	var apiResp geminiUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 转换为统一格式
	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeGemini,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		Account: &quota.UsageAccount{
			Tier: apiResp.Tier,
		},
	}

	// 按模型聚合配额
	if len(apiResp.UserQuota.ModelQuotas) > 0 {
		// Primary: 总配额（所有模型合计）
		totalUsed := 0.0
		totalLimit := 0.0
		for _, mq := range apiResp.UserQuota.ModelQuotas {
			totalUsed += mq.QuotaUsed
			totalLimit += mq.QuotaLimit
		}

		usage.Primary = &quota.UsageWindow{
			Type:          quota.WindowTypeDaily,
			Used:          totalUsed,
			Limit:         totalLimit,
			Unit:          quota.UsageUnitRequests,
			WindowMinutes: 24 * 60, // 24小时
			Label:         "Daily Requests",
			Description:   "Total daily request quota across all models",
		}
		usage.Primary.UsedPercent = usage.Primary.CalculateUsedPercent()

		// Secondary: Pro 模型配额
		for _, mq := range apiResp.UserQuota.ModelQuotas {
			if mq.ModelName == "gemini-pro" || mq.ModelName == "gemini-1.5-pro" {
				usage.Secondary = &quota.UsageWindow{
					Type:          quota.WindowTypeDaily,
					Used:          mq.QuotaUsed,
					Limit:         mq.QuotaLimit,
					Unit:          quota.UsageUnitRequests,
					WindowMinutes: int(mq.WindowSizeHours * 60),
					Label:         "Pro Model Quota",
					Description:   "Daily request quota for Pro models",
				}
				usage.Secondary.UsedPercent = usage.Secondary.CalculateUsedPercent()
				break
			}
		}

		// Tertiary: Flash 模型配额
		for _, mq := range apiResp.UserQuota.ModelQuotas {
			if mq.ModelName == "gemini-flash" || mq.ModelName == "gemini-1.5-flash" {
				usage.Tertiary = &quota.UsageWindow{
					Type:          quota.WindowTypeDaily,
					Used:          mq.QuotaUsed,
					Limit:         mq.QuotaLimit,
					Unit:          quota.UsageUnitRequests,
					WindowMinutes: int(mq.WindowSizeHours * 60),
					Label:         "Flash Model Quota",
					Description:   "Daily request quota for Flash models",
				}
				usage.Tertiary.UsedPercent = usage.Tertiary.CalculateUsedPercent()
				break
			}
		}
	}

	return usage, nil
}
