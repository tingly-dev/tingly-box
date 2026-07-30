package quota

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Every provider reports quota in its own currency — percentages, request
// counts, credits, dollars. This file reduces that to the two things a caller
// can act on: how much is used, and when it comes back.
// See .design/quota-semantics.md.

// EffectiveKind reports the window kind, defaulting to WindowKindLimit for
// windows written before the field existed.
func (w *UsageWindow) EffectiveKind() WindowKind {
	if w == nil || w.Kind == "" {
		return WindowKindLimit
	}
	return w.Kind
}

// Percent returns the window's used percentage, falling back to used/limit for
// windows that have not had UsedPercent filled in yet.
func (w *UsageWindow) Percent() float64 {
	if w == nil {
		return 0
	}
	if w.UsedPercent != 0 {
		return w.UsedPercent
	}
	return w.CalculateUsedPercent()
}

// Countable reports whether this window carries a usage figure that can be
// compared with another provider's. Unknown means upstream did not say,
// unlimited means there is nothing to be used up, and without a cap there is
// nothing to measure against — none of the three is a usage of 0%.
//
// The cap check is the backstop: a fetcher that reports spend without a limit
// and forgets the flag would otherwise contribute a fabricated 0%, which sorts
// ahead of real windows and wins ties against them.
func (w *UsageWindow) Countable() bool {
	return w != nil && !w.Unknown && !w.Unlimited && w.Limit > 0
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
	return w.Percent(), true
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
	if pa, pb := a.Percent(), b.Percent(); pa != pb {
		return pa > pb
	}
	// Equally used: prefer the shorter window. A window with no known duration
	// sorts last — it cannot be the more urgent one if we cannot say when it
	// resets.
	return periodRank(a) < periodRank(b)
}

// NormalizeWindows puts Windows into display order: allowances first, shortest
// period leading, then standing resources, then anything with no usage figure
// to show.
//
// Short periods lead because they move fastest and are what a user watches; a
// balance has no period at all, and an unknown or uncapped entry has nothing
// to compare, so both sort after the windows that do.
func (p *ProviderUsage) NormalizeWindows() {
	if p == nil {
		return
	}

	for i, window := range p.Windows {
		if window == nil {
			continue
		}
		if window.Key == "" {
			window.Key = fmt.Sprintf("window_%d", i)
		}
		applyWindowSemantics(window)
	}

	sort.SliceStable(p.Windows, func(i, j int) bool {
		left, right := p.Windows[i], p.Windows[j]
		if left == nil || right == nil {
			return left != nil
		}
		if lr, rr := windowRank(left), windowRank(right); lr != rr {
			return lr < rr
		}
		return periodRank(left) < periodRank(right)
	})
}

// periodRank orders by window length, putting unsized windows last: the same
// rule tighter() uses, so a window cannot be least urgent for Tightest() and
// most prominent for display.
func periodRank(w *UsageWindow) int {
	if w.WindowMinutes <= 0 {
		return math.MaxInt
	}
	return w.WindowMinutes
}

// windowRank groups windows for display; within a group they order by period.
func windowRank(w *UsageWindow) int {
	switch {
	case !w.Countable():
		return 2
	case w.EffectiveKind() == WindowKindResource:
		return 1
	default:
		return 0
	}
}

// Unreadable builds usage for a provider whose quota could not be read — no
// API, a failed fetch, an unsupported type. It records the reason and reports
// no windows: Pct already answers "unknown" for a usage with nothing countable
// in it, so a placeholder window would add a row to every surface without
// adding anything a reader can act on.
func Unreadable(providerUUID, providerName string, providerType ProviderType, reason string, now time.Time, ttl time.Duration) *ProviderUsage {
	usage := &ProviderUsage{
		ProviderUUID: providerUUID,
		ProviderName: providerName,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(ttl),
	}
	usage.MarkUnreadable(reason, now)
	return usage
}

// MarkUnreadable records why a usage could not be read, leaving whatever the
// caller already gathered in place.
func (p *ProviderUsage) MarkUnreadable(reason string, now time.Time) {
	if p == nil {
		return
	}
	p.LastError = reason
	p.LastErrorAt = &now
}
