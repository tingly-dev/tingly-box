package ai

import "time"

// Limits caps what an upstream account (Provider.Limits) or one rule's share of
// it (loadbalance.Service.Limits) may do. All fields are optional; the zero
// value means "no limit". See .design/routing-limits.md.
type Limits struct {
	// MaxConcurrent caps in-flight requests. 0 = unlimited.
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`

	// WindowSeconds is the sliding window for MaxRequests / MaxTokens.
	// Defaults to DefaultLimitWindowSeconds when a budget is set.
	WindowSeconds int `json:"window_seconds,omitempty" yaml:"window_seconds,omitempty"`
	// MaxRequests caps requests admitted per window. 0 = unlimited.
	MaxRequests int64 `json:"max_requests,omitempty" yaml:"max_requests,omitempty"`
	// MaxTokens caps input+output tokens per window. 0 = unlimited.
	MaxTokens int64 `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// SoftPercent (1-99): traffic is steered away from this subject once any
	// limit reaches this percentage, as long as an alternative below it
	// exists. 0 = off (only the hard limit applies).
	SoftPercent int `json:"soft_percent,omitempty" yaml:"soft_percent,omitempty"`

	// QuotaPercent (1-100, provider-level only): stop using the account once
	// its upstream ai/quota usage reaches this percentage. 0 = off.
	QuotaPercent int `json:"quota_percent,omitempty" yaml:"quota_percent,omitempty"`
}

// DefaultLimitWindowSeconds is the usage window used when a budget is set
// without an explicit WindowSeconds.
const DefaultLimitWindowSeconds = 3600

// IsZero reports whether no limit is configured.
func (l *Limits) IsZero() bool {
	return l == nil || *l == Limits{}
}

// HasBudget reports whether a request/token budget is configured.
func (l *Limits) HasBudget() bool {
	return l != nil && (l.MaxRequests > 0 || l.MaxTokens > 0)
}

// Window returns the effective usage window (0 when no budget is set).
func (l *Limits) Window() time.Duration {
	if !l.HasBudget() {
		return 0
	}
	if l.WindowSeconds > 0 {
		return time.Duration(l.WindowSeconds) * time.Second
	}
	return DefaultLimitWindowSeconds * time.Second
}

// Normalize clamps out-of-range values (negative numbers, percentages) so a
// hand-edited config can never produce nonsense. Returns a copy; nil stays nil
// and an all-zero result collapses to nil.
func (l *Limits) Normalize() *Limits {
	if l == nil {
		return nil
	}
	out := *l
	if out.MaxConcurrent < 0 {
		out.MaxConcurrent = 0
	}
	if out.WindowSeconds < 0 {
		out.WindowSeconds = 0
	}
	if out.MaxRequests < 0 {
		out.MaxRequests = 0
	}
	if out.MaxTokens < 0 {
		out.MaxTokens = 0
	}
	if out.SoftPercent < 0 || out.SoftPercent >= 100 {
		out.SoftPercent = 0
	}
	if out.QuotaPercent < 0 {
		out.QuotaPercent = 0
	}
	if out.QuotaPercent > 100 {
		out.QuotaPercent = 100
	}
	if !out.HasBudget() {
		out.WindowSeconds = 0
	}
	if out.IsZero() {
		return nil
	}
	return &out
}
