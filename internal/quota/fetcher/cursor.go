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

// CursorFetcher Cursor 配额获取器
type CursorFetcher struct {
	client *http.Client
	logger *logrus.Logger
}

// NewCursorFetcher 创建 Cursor fetcher
func NewCursorFetcher(logger *logrus.Logger) *CursorFetcher {
	return &CursorFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (f *CursorFetcher) Name() string {
	return "cursor"
}

func (f *CursorFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCursor
}

func (f *CursorFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *CursorFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *CursorFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// Cursor 没有公开的配额 API，返回默认值
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *CursorFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeCursor,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available",
		LastErrorAt:  &now,
	}
}
