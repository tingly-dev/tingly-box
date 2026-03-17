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

// CodexFetcher Codex 配额获取器
type CodexFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewCodexFetcher 创建 Codex fetcher
func NewCodexFetcher(logger *logrus.Logger) *CodexFetcher {
	return &CodexFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *CodexFetcher) Name() string {
	return "codex"
}

func (f *CodexFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCodex
}

func (f *CodexFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *CodexFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *CodexFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// Codex 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *CodexFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeCodex,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available",
		LastErrorAt:  &now,
	}
}
