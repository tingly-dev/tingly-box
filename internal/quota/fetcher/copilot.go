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

// CopilotFetcher GitHub Copilot 配额获取器
type CopilotFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewCopilotFetcher 创建 Copilot fetcher
func NewCopilotFetcher(logger *logrus.Logger) *CopilotFetcher {
	return &CopilotFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *CopilotFetcher) Name() string {
	return "copilot"
}

func (f *CopilotFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCopilot
}

func (f *CopilotFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeOAuth
}

func (f *CopilotFetcher) Validate(provider *typ.Provider) error {
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

func (f *CopilotFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// GitHub Copilot 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *CopilotFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeCopilot,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available",
		LastErrorAt:  &now,
	}
}
