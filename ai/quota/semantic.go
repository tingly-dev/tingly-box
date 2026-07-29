package quota

import "time"

// Every provider reports quota in its own currency — percentages, request
// counts, credits, dollars. The three accessors here reduce that to the two
// things a caller can actually act on: how much is used, and when it comes
// back. See .design/quota-semantics.md.

// EffectiveKind reports the window kind, defaulting to WindowKindLimit for
// windows written before the field existed.
func (w *UsageWindow) EffectiveKind() WindowKind {
	if w == nil || w.Kind == "" {
		return WindowKindLimit
	}
	return w.Kind
}

// Pct returns the window's used percentage, falling back to used/limit for
// windows that never populated UsedPercent.
func (w *UsageWindow) Pct() float64 {
	if w == nil {
		return 0
	}
	if w.UsedPercent != 0 {
		return w.UsedPercent
	}
	return w.CalculateUsedPercent()
}

// Countable reports whether this window carries a usage figure that can be
// compared with another provider's. Unknown means upstream did not say, and
// unlimited means there is nothing to be used up — neither is a usage of 0%.
func (w *UsageWindow) Countable() bool {
	return w != nil && !w.Unknown && !w.Unlimited
}

// Pct returns the provider's usage as a single 0-100 figure: the tightest
// countable window, because that is the one that runs out first. ok is false
// when no window is countable, which means usage is unknown — deliberately
// distinct from a usage of zero.
//
// Only Windows are considered. Breakdowns are scoped to one model or feature
// and do not describe the provider as a whole.
func (p *ProviderUsage) Pct() (float64, bool) {
	w := p.Tightest()
	if w == nil {
		return 0, false
	}
	return w.Pct(), true
}

// Tightest returns the window Pct came from, so callers can say which window
// is binding rather than just quoting a number. Ties go to the shorter window,
// which is the more actionable one to show.
func (p *ProviderUsage) Tightest() *UsageWindow {
	if p == nil {
		return nil
	}

	var best *UsageWindow
	for _, w := range p.Windows {
		if !w.Countable() {
			continue
		}
		if best == nil || tighter(w, best) {
			best = w
		}
	}
	return best
}

// RecoversAt returns when the binding window refills. It is nil when usage is
// unknown, when the binding window is a resource (a top-up, not a reset, is
// what brings it back), or when upstream did not report a reset time.
func (p *ProviderUsage) RecoversAt() *time.Time {
	w := p.Tightest()
	if w == nil || w.EffectiveKind() == WindowKindResource {
		return nil
	}
	return w.ResetsAt
}

// tighter reports whether a should displace b as the binding window.
func tighter(a, b *UsageWindow) bool {
	if pa, pb := a.Pct(), b.Pct(); pa != pb {
		return pa > pb
	}
	// Equally used: prefer the shorter window. A window with no known duration
	// sorts last — it cannot be the more urgent one if we cannot say when it
	// resets.
	return periodRank(a) < periodRank(b)
}

// Unobservable builds usage for a provider whose quota could not be read —
// no API, a failed fetch, an unsupported type. It reports an explicit unknown
// window rather than an empty one: with nothing in Windows, "we cannot see
// this account" is indistinguishable from "this account is untouched", and
// the more useful of those two readings is the one a caller will assume.
func Unobservable(providerUUID, providerName string, providerType ProviderType, reason string, now time.Time, ttl time.Duration) *ProviderUsage {
	usage := &ProviderUsage{
		ProviderUUID: providerUUID,
		ProviderName: providerName,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(ttl),
		LastError:    reason,
		LastErrorAt:  &now,
	}
	usage.AddWindow("unavailable", 0, &UsageWindow{
		Type:        WindowTypeCustom,
		Unknown:     true,
		Label:       "Quota unavailable",
		Description: reason,
	})
	return usage
}
