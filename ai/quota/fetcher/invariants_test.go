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

func checkWindow(t *testing.T, where string, w *quota.UsageWindow) {
	t.Helper()

	// A periodic allowance has to say how long its period is: the duration is
	// what orders windows for display and what breaks ties for Tightest().
	// Only windows that carry a usage figure take part in either, so an
	// unknown or uncapped one is exempt — its period is unknowable too.
	if w.Countable() && w.EffectiveKind() == quota.WindowKindLimit && w.WindowMinutes <= 0 {
		t.Errorf("%s: allowance has no WindowMinutes; callers cannot tell how long the period is", where)
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

	if p := w.Pct(); p < 0 || p > 100 {
		t.Errorf("%s: Pct() = %v, outside 0-100", where, p)
	}
}
