package fetcher

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// checkInvariants asserts the rules every fetcher's output has to satisfy for
// its usage figures to mean the same thing as another provider's. Each
// provider test calls it, so a new fetcher that skips the semantics fails here
// rather than silently reporting a number nobody can compare.
//
// See .design/quota-semantics.md.
func checkInvariants(t *testing.T, usage *quota.ProviderUsage) {
	t.Helper()

	if usage == nil {
		t.Fatal("usage is nil")
	}

	for _, w := range usage.Windows {
		if w == nil {
			t.Error("Windows contains a nil entry")
			continue
		}
		checkWindow(t, "window "+w.Key, w)
	}

	for _, bd := range usage.Breakdowns {
		for _, w := range bd.Windows {
			if w == nil {
				t.Errorf("breakdown %q contains a nil window", bd.Key)
				continue
			}
			checkWindow(t, "breakdown "+bd.Key, w)
		}
	}
}

// namesAPeriod reports whether the type commits to a window length.
func namesAPeriod(t quota.WindowType) bool {
	switch t {
	case quota.WindowTypeSession, quota.WindowTypeDaily, quota.WindowTypeWeekly, quota.WindowTypeMonthly:
		return true
	default:
		return false
	}
}

func checkWindow(t *testing.T, where string, w *quota.UsageWindow) {
	t.Helper()

	// A window whose type names a period has to state that period: the duration
	// is what orders windows and what breaks ties for Tightest(). Types that
	// name no period are exempt — "custom" means upstream gave us nothing to
	// classify by, and inventing a length there would be worse than omitting
	// it. Uncountable windows are exempt too; they take part in neither.
	if w.Countable() && namesAPeriod(w.Type) && w.WindowMinutes <= 0 {
		t.Errorf("%s: %s window has no WindowMinutes; callers cannot tell how long the period is", where, w.Type)
	}

	// A resource is brought back by topping up, not by waiting, so a reset
	// time on it would be read as a recovery that never happens.
	if w.EffectiveKind() == quota.WindowKindResource && w.ResetsAt != nil {
		t.Errorf("%s: resource has ResetsAt set; a balance does not refill on its own", where)
	}

	// The direction that catches an omission: a balance left as an allowance
	// would claim it refills, and RecoversAt would promise a recovery that
	// never arrives.
	if w.Type == quota.WindowTypeBalance && w.EffectiveKind() != quota.WindowKindResource {
		t.Errorf("%s: balance is not marked as a resource", where)
	}

	// Unknown and unlimited both mean "this is not a usage figure", so neither
	// may also carry one — otherwise readers that ignore the flags see a
	// plausible number and trust it.
	if w.Unknown && w.Unlimited {
		t.Errorf("%s: window is both unknown and unlimited", where)
	}
	if !w.Countable() && w.UsedPercent != 0 {
		t.Errorf("%s: uncountable window still reports UsedPercent=%v", where, w.UsedPercent)
	}

	if p := w.Percent(); p < 0 || p > 100 {
		t.Errorf("%s: Percent() = %v, outside 0-100", where, p)
	}
}
