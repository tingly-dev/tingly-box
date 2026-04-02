package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// VertexAIFetcher Google Vertex AI 配额获取器
type VertexAIFetcher struct {
	logger *logrus.Logger
}

// NewVertexAIFetcher 创建 VertexAI fetcher
func NewVertexAIFetcher(logger *logrus.Logger) *VertexAIFetcher {
	return &VertexAIFetcher{
		logger: logger,
	}
}

func (f *VertexAIFetcher) Name() string {
	return "vertex_ai"
}

func (f *VertexAIFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeVertexAI
}

func (f *VertexAIFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeAPIKey
}

func (f *VertexAIFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *VertexAIFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	// Vertex AI 配额通过 Google Cloud Console 管理，没有公开的 API
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage 创建默认配额信息
func (f *VertexAIFetcher) createDefaultUsage(provider *typ.Provider) *quota.ProviderUsage {
	now := time.Now()

	return &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeVertexAI,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    "quota API not available - check Google Cloud Console",
		LastErrorAt:  &now,
	}
}
