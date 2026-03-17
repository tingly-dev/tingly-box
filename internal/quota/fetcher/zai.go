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

// ZaiFetcher Zai 配额获取器
type ZaiFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewZaiFetcher 创建 Zai fetcher
func NewZaiFetcher(logger *logrus.Logger) *ZaiFetcher {
	return &ZaiFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *ZaiFetcher) Name() string {
	return "zai"
}

func (f *ZaiFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeZai
}

func (f *ZaiFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *ZaiFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *ZaiFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// Zai 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *ZaiFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeZai,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available",
		LastErrorAt:  &now,
	}
}
