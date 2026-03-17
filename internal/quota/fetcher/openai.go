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

// OpenAIFetcher OpenAI 配额获取器
type OpenAIFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewOpenAIFetcher 创建 OpenAI fetcher
func NewOpenAIFetcher(logger *logrus.Logger) *OpenAIFetcher {
	return &OpenAIFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *OpenAIFetcher) Name() string {
	return "openai"
}

func (f *OpenAIFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeOpenAI
}

func (f *OpenAIFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *OpenAIFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

// openaiUsageResponse OpenAI API 响应
type openaiUsageResponse struct {
	Object string `json:"object"`
	Data   []struct {
		AggregationType string  `json:"aggregation_type"`
		NRequests       int     `json:"n_requests"`
		Operation       string  `json:"operation"`
		Metadata        *struct {
			ResponseType string `json:"response_format"`
			Model        string `json:"model"`
		} `json:"metadata"`
		NUnits             float64 `json:"n_units"`
		CurrentUsageUSD     float64 `json:"current_usage_usd"`
		CurrentAvailableUSD float64 `json:"current_available_usd"`
	} `json:"data"`
}

func (f *OpenAIFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// 构建请求
	apiBase := provider.APIBase
	if apiBase == "" {
		apiBase = "https://api.openai.com"
	}

	url := fmt.Sprintf("%s/v1/usage", apiBase)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	// 发送请求
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// OpenAI 不提供统一的用量 API，返回默认值
		return f.createDefaultUsage(provider), nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应
	var apiResp openaiUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// OpenAI 的 API 返回的是按时间序列的数据，我们需要聚合
	// 由于 OpenAI 没有提供配额限制信息，我们只能返回已使用金额
	now := time.Now()

	totalUsed := 0.0
	for _, item := range apiResp.Data {
		totalUsed += item.CurrentUsageUSD
	}

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenAI,
		FetchedAt:    now,
		ExpiresAt:    now.Add(10 * time.Minute), // 10分钟缓存

		// Cost: 预付费余额
		Cost: &quota.UsageCost{
			Used:         totalUsed,
			Limit:        0, // OpenAI 不提供限制信息
			CurrencyCode: "USD",
			Label:        "Prepaid Credits",
		},
	}

	return usage, nil
}

// createDefaultUsage 创建默认配额信息（当 API 不可用时）
func (f *OpenAIFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenAI,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour), // 1小时缓存

		// OpenAI 不提供配额信息
		Cost: &quota.UsageCost{
			Used:         0,
			Limit:        0,
			CurrencyCode: "USD",
			Label:        "Prepaid Credits (see dashboard)",
		},
	}
}
