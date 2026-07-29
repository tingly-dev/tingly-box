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

// unobservableUsage describes a provider that exposes no quota API. It reports
// an explicit unknown window rather than an empty one: with nothing in
// Windows, "we cannot see this account" is indistinguishable from "this
// account is untouched", and the more useful of those two readings is the one
// a caller will assume.
func unobservableUsage(provider *ai.Provider, providerType quota.ProviderType, reason string) *quota.ProviderUsage {
	now := time.Now()

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(1 * time.Hour),
		LastError:    reason,
		LastErrorAt:  &now,
	}
	usage.AddWindow("unavailable", 0, &quota.UsageWindow{
		Type:        quota.WindowTypeCustom,
		Unknown:     true,
		Label:       "Quota unavailable",
		Description: reason,
	})
	return usage
}
