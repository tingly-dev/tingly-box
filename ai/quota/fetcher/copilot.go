package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// CopilotFetcher GitHub Copilot 配额获取器
type CopilotFetcher struct {
	logger *logrus.Logger
}

// NewCopilotFetcher 创建 Copilot fetcher
func NewCopilotFetcher(logger *logrus.Logger) *CopilotFetcher {
	return &CopilotFetcher{
		logger: logger,
	}
}

func (f *CopilotFetcher) Name() string {
	return "copilot"
}

func (f *CopilotFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCopilot
}

func (f *CopilotFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeOAuth
}

func (f *CopilotFetcher) Validate(provider *ai.Provider) error {
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

func (f *CopilotFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	// GitHub Copilot 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *CopilotFetcher) createDefaultUsage(provider *ai.Provider) *quota.ProviderUsage {
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
