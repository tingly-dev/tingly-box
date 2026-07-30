package quota

import (
	"testing"
	"time"
)

func window(pct float64, minutes int, opts ...func(*UsageWindow)) *UsageWindow {
	w := &UsageWindow{UsedPercent: pct, WindowMinutes: minutes, Limit: 100, Used: pct}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func labelled(label string) func(*UsageWindow) {
	return func(w *UsageWindow) { w.Label = label }
}

func TestPctTakesTheTightestWindow(t *testing.T) {
	// The Friday-night case: the 5h window just reset but the 7d window is
	// nearly gone. Reporting the shortest window would say "plenty left".
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(12, 300, labelled("5h")),
		window(96, 10080, labelled("7d")),
	}}

	pct, ok := usage.Pct()
	if !ok || pct != 96 {
		t.Fatalf("Pct() = %v, %v; want 96, true", pct, ok)
	}
	if got := usage.Tightest().Label; got != "7d" {
		t.Fatalf("Tightest() = %q; want %q", got, "7d")
	}
}

func TestPctIsMaxNotAverage(t *testing.T) {
	// An exhausted model averaged against an untouched one reads as half full.
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(100, 1440, labelled("pro")),
		window(0, 1440, labelled("flash")),
	}}

	if pct, ok := usage.Pct(); !ok || pct != 100 {
		t.Fatalf("Pct() = %v, %v; want 100, true", pct, ok)
	}
}

func TestUnknownIsNotZero(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(0, 300, func(w *UsageWindow) { w.Unknown = true }),
	}}

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown", pct, ok)
	}
	if usage.Tightest() != nil {
		t.Fatal("Tightest() should be nil when nothing is countable")
	}
	if usage.RecoversAt() != nil {
		t.Fatal("RecoversAt() should be nil when nothing is countable")
	}
}

func TestUnknownDoesNotSuppressAKnownWindow(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(0, 300, func(w *UsageWindow) { w.Unknown = true }),
		window(40, 10080, labelled("weekly")),
	}}

	if pct, ok := usage.Pct(); !ok || pct != 40 {
		t.Fatalf("Pct() = %v, %v; want 40, true", pct, ok)
	}
}

func TestUnlimitedDoesNotCount(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(0, 43200, func(w *UsageWindow) { w.Unlimited = true; w.Limit = 0 }),
	}}

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown for an unlimited-only provider", pct, ok)
	}
}

func TestNoWindowsIsUnknown(t *testing.T) {
	// Copilot / Cursor / VertexAI today: an error and nothing else. That must
	// not read the same as an untouched quota.
	usage := &ProviderUsage{LastError: "quota API not available"}

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown", pct, ok)
	}
}

func TestRecoversAtComesFromTheBindingWindow(t *testing.T) {
	sunday := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC)
	soon := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)

	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(12, 300, func(w *UsageWindow) { w.ResetsAt = &soon }),
		window(96, 10080, func(w *UsageWindow) { w.ResetsAt = &sunday }),
	}}

	got := usage.RecoversAt()
	if got == nil || !got.Equal(sunday) {
		t.Fatalf("RecoversAt() = %v; want %v", got, sunday)
	}
}

func TestResourcesDoNotRecoverOnTheirOwn(t *testing.T) {
	// A drained balance comes back by topping up, not by waiting, so there is
	// no recovery time to report even if a reset-ish timestamp exists.
	expires := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(90, 0, func(w *UsageWindow) {
			w.Kind = WindowKindResource
			w.ResetsAt = &expires
		}),
	}}

	if got := usage.RecoversAt(); got != nil {
		t.Fatalf("RecoversAt() = %v; want nil for a resource", got)
	}
	if pct, ok := usage.Pct(); !ok || pct != 90 {
		t.Fatalf("Pct() = %v, %v; want 90, true — resources still count toward usage", pct, ok)
	}
}

func TestEqualUsagePrefersTheShorterWindow(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(80, 10080, labelled("7d")),
		window(80, 300, labelled("5h")),
	}}

	if got := usage.Tightest().Label; got != "5h" {
		t.Fatalf("Tightest() = %q; want %q", got, "5h")
	}
}

func TestEqualUsageIgnoresWindowsOfUnknownLength(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		window(80, 0, labelled("unsized")),
		window(80, 300, labelled("5h")),
	}}

	if got := usage.Tightest().Label; got != "5h" {
		t.Fatalf("Tightest() = %q; want %q", got, "5h")
	}
}

func TestPctFallsBackToUsedOverLimit(t *testing.T) {
	// Windows that never populated UsedPercent still have to answer.
	usage := &ProviderUsage{Windows: []*UsageWindow{
		{Used: 340, Limit: 500, Unit: UsageUnitRequests, WindowMinutes: 1440},
	}}

	pct, ok := usage.Pct()
	if !ok || pct != 68 {
		t.Fatalf("Pct() = %v, %v; want 68, true", pct, ok)
	}
}

func TestBreakdownsDoNotAnswerForTheProvider(t *testing.T) {
	// Breakdowns are scoped to one model; an exhausted model must not be read
	// as the provider being exhausted, nor stand in when Windows is empty.
	usage := &ProviderUsage{Breakdowns: []*UsageBreakdown{{
		Key:     "gemini-2.5-pro",
		Windows: []*UsageWindow{window(100, 1440)},
	}}}

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown", pct, ok)
	}
}

func TestKindDefaultsToLimit(t *testing.T) {
	if got := (&UsageWindow{}).EffectiveKind(); got != WindowKindLimit {
		t.Fatalf("EffectiveKind() = %q; want %q", got, WindowKindLimit)
	}
	if got := (&UsageWindow{Kind: WindowKindResource}).EffectiveKind(); got != WindowKindResource {
		t.Fatalf("EffectiveKind() = %q; want %q", got, WindowKindResource)
	}
}

func TestNormalizeWindowsOrdersByPeriodThenKind(t *testing.T) {
	// Deliberately declared out of order, to show the order comes from the
	// windows themselves.
	usage := &ProviderUsage{Windows: []*UsageWindow{
		{Key: "unknown", Unknown: true},
		{Key: "balance", Kind: WindowKindResource, UsedPercent: 40, Limit: 100},
		{Key: "weekly", UsedPercent: 30, Limit: 100, WindowMinutes: 10080},
		{Key: "session", UsedPercent: 20, Limit: 100, WindowMinutes: 300},
	}}

	usage.NormalizeWindows()

	var got []string
	for _, w := range usage.Windows {
		got = append(got, w.Key)
	}
	want := []string{"session", "weekly", "balance", "unknown"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v; want %v", got, want)
		}
	}
}

func TestNormalizeWindowsIsStableForEqualPeriods(t *testing.T) {
	usage := &ProviderUsage{Windows: []*UsageWindow{
		{Key: "a", UsedPercent: 10, Limit: 100, WindowMinutes: 1440},
		{Key: "b", UsedPercent: 20, Limit: 100, WindowMinutes: 1440},
	}}

	usage.NormalizeWindows()

	if usage.Windows[0].Key != "a" || usage.Windows[1].Key != "b" {
		t.Errorf("equal periods should keep fetcher order, got %q then %q",
			usage.Windows[0].Key, usage.Windows[1].Key)
	}
}

func TestBalanceDefaultsToResource(t *testing.T) {
	// A fetcher that states the type but forgets the kind would otherwise
	// produce a balance claiming it refills.
	usage := &ProviderUsage{}
	reset := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC)
	w := usage.AddWindow("credits", &UsageWindow{
		Type: WindowTypeBalance, Used: 30, Limit: 100, ResetsAt: &reset,
	})

	if w.EffectiveKind() != WindowKindResource {
		t.Errorf("EffectiveKind() = %q; want resource", w.EffectiveKind())
	}
	if usage.RecoversAt() != nil {
		t.Error("a balance must not report a recovery time")
	}
}

func TestUncountableWindowsNeverCarryAPercentage(t *testing.T) {
	// Backfilling used_percent onto an unknown window would hand a plausible
	// figure to any reader that skips the flag.
	usage := &ProviderUsage{}
	w := usage.AddWindow("spend", &UsageWindow{
		Unknown: true, Used: 12.5, Limit: 100,
	})

	if w.UsedPercent != 0 {
		t.Errorf("UsedPercent = %v; want 0 for an unknown window", w.UsedPercent)
	}
}

func TestOrderingAndTieBreakAgreeOnUnsizedWindows(t *testing.T) {
	// An unsized window is least urgent for Tightest(), so it must not be the
	// most prominent for display.
	usage := &ProviderUsage{Windows: []*UsageWindow{
		{Key: "unsized", UsedPercent: 50, Limit: 100},
		{Key: "weekly", UsedPercent: 50, Limit: 100, WindowMinutes: 10080},
	}}

	if got := usage.Tightest().Key; got != "weekly" {
		t.Errorf("Tightest() = %q; want weekly", got)
	}
	usage.NormalizeWindows()
	if got := usage.Windows[0].Key; got != "weekly" {
		t.Errorf("first window = %q; want weekly — display must match the tie-break", got)
	}
}

func TestWindowWithNoCapIsNotCountable(t *testing.T) {
	// A fetcher reporting spend with no cap and no flag would otherwise
	// contribute a fabricated 0% that sorts ahead of real windows.
	usage := &ProviderUsage{Windows: []*UsageWindow{
		{Key: "uncapped", Used: 8.1, Limit: 0, WindowMinutes: 43200},
		{Key: "capped", Used: 0, Limit: 20, WindowMinutes: 43200},
	}}

	if got := usage.Tightest().Key; got != "capped" {
		t.Errorf("Tightest() = %q; want capped — the uncapped window has nothing to compare", got)
	}
	usage.NormalizeWindows()
	if usage.Windows[0].Key != "capped" {
		t.Errorf("first window = %q; want capped", usage.Windows[0].Key)
	}
}
