package fetcher

import (
	"cmp"
	"strings"
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

// unreadableUsage describes a provider whose quota cannot be read.
func unreadableUsage(provider *ai.Provider, providerType quota.ProviderType, reason string) *quota.ProviderUsage {
	return quota.Unreadable(provider.UUID, provider.Name, providerType, reason, time.Now(), 1*time.Hour)
}

// windowTypeForMinutes names a period from its length. Upstream types are not
// dependable — Codex reports 604800s for what it calls the primary window on a
// free plan, and MiniMax's "daily" bucket is whatever interval it reports — and
// the name reaches users through the status line.
func windowTypeForMinutes(minutes int) quota.WindowType {
	switch {
	case minutes <= 0:
		return quota.WindowTypeCustom
	case minutes < 24*60:
		return quota.WindowTypeSession
	case minutes < 2*24*60:
		return quota.WindowTypeDaily
	case minutes < 28*24*60:
		return quota.WindowTypeWeekly
	default:
		return quota.WindowTypeMonthly
	}
}

// endpoint resolves a request URL, preferring the test override over the
// production host.
func endpoint(override, production, path string) string {
	return strings.TrimRight(cmp.Or(override, production), "/") + path
}
