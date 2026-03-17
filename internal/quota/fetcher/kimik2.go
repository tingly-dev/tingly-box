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

// KimiK2Fetcher Kimi K2 (Moonshot) 配额获取器
type KimiK2Fetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewKimiK2Fetcher 创建 KimiK2 fetcher
func NewKimiK2Fetcher(logger *logrus.Logger) *KimiK2Fetcher {
	return &KimiK2Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *KimiK2Fetcher) Name() string {
	return "kimi_k2"
}

func (f *KimiK2Fetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeKimiK2
}

func (f *KimiK2Fetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *KimiK2Fetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *KimiK2Fetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// Moonshot (Kimi) 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *KimiK2Fetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeKimiK2,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available",
		LastErrorAt:  &now,
	}
}
