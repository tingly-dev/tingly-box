package fetcher

import (
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// calcPercent returns used/limit * 100, capped at 100.
func calcPercent(used, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	p := (used / limit) * 100
	if p > 100 {
		return 100
	}
	return p
}

// unobservableUsage describes a provider that exposes no quota API.
func unobservableUsage(provider *ai.Provider, providerType quota.ProviderType, reason string) *quota.ProviderUsage {
	return quota.Unobservable(provider.UUID, provider.Name, providerType, reason, time.Now(), 1*time.Hour)
}
