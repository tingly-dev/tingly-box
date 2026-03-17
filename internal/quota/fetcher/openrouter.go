package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenRouterFetcher OpenRouter 配额获取器
type OpenRouterFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewOpenRouterFetcher 创建 OpenRouter fetcher
func NewOpenRouterFetcher(logger *logrus.Logger) *OpenRouterFetcher {
	return &OpenRouterFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *OpenRouterFetcher) Name() string {
	return "openrouter"
}

func (f *OpenRouterFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeOpenRouter
}

func (f *OpenRouterFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *OpenRouterFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *OpenRouterFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// OpenRouter 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *OpenRouterFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenRouter,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available - check openrouter.ai dashboard",
		LastErrorAt:  &now,
	}
}
