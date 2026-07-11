package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// CursorFetcher retrieves Cursor quota data.
type CursorFetcher struct{}

// NewCursorFetcher creates a Cursor quota fetcher.
func NewCursorFetcher() *CursorFetcher {
	return &CursorFetcher{}
}

func (f *CursorFetcher) Name() string {
	return "cursor"
}

func (f *CursorFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeCursor
}

func (f *CursorFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeAPIKey
}

func (f *CursorFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

func (f *CursorFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	// Cursor has no public quota API, so return fallback data.
	return f.createDefaultUsage(provider), nil
}

// createDefaultUsage creates fallback quota data.
func (f *CursorFetcher) createDefaultUsage(provider *ai.Provider) *quota.ProviderUsage {
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
