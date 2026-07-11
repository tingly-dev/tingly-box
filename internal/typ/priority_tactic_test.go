package typ

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func mkService(provider, model string, tier int) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Active:   true,
		Tier:     tier,
	}
}

func mkTierRule(services ...*loadbalance.Service) *Rule {
	return &Rule{
		UUID:     "rule-test",
		Services: services,
		Active:   true,
		LBTactic: Tactic{
			Type:   loadbalance.TacticTier,
			Params: DefaultTierParams(),
		},
	}
}

func TestTierPicksLowestNumberFirst(t *testing.T) {
	primary := mkService("p1", "m1", 0) // T0 — highest priority
	backup := mkService("p2", "m1", 1)  // T1 — fallback
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	got := tactic.SelectService(rule)
	if got != primary {
		t.Fatalf("want T0 primary, got %v", got)
	}
}

func TestTierFallsBackWhenBreakerOpen(t *testing.T) {
	primary := mkService("fb-p1", "m1", 0)
	backup := mkService("fb-p2", "m1", 1)
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	// Trip the primary's breaker.
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(rule.UUID, primary.ServiceID())
	}
	defer store.RecordSuccess(rule.UUID, primary.ServiceID()) // clean up for other tests

	got := tactic.SelectService(rule)
	if got != backup {
		t.Fatalf("T0 breaker open, want T1 backup, got %v", got)
	}
}

func TestTierReturnsToHigherWhenBreakerCloses(t *testing.T) {
	restore := clock.SetClock(func() time.Time { return time.Unix(2_000_000_000, 0) })
	defer restore()

	primary := mkService("recover-p1", "recover-m1", 0)
	backup := mkService("recover-p2", "recover-m1", 1)
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(rule.UUID, primary.ServiceID())
	}
	if got := tactic.SelectService(rule); got != backup {
		t.Fatalf("expected fallback to T1 backup, got %v", got)
	}

	// Recovery now requires consecutive probe successes (hysteresis: a single
	// success no longer closes the breaker). Keep this test focused on tier
	// return by requiring just one successful probe; the streak itself is
	// covered in breaker_test.go.
	b := store.Get(rule.UUID, primary.ServiceID())
	prevThreshold := b.RecoveryThreshold
	b.RecoveryThreshold = 1
	defer func() { b.RecoveryThreshold = prevThreshold }()

	// Advance past the open window so SelectService's internal Allow() flips
	// the breaker Open → HalfOpen and admits the primary again.
	now := func() time.Time { return time.Unix(2_000_000_000, 0).Add(loadbalance.DefaultBreakerOpenDuration + time.Second) }
	clock.SetClock(now)
	got := tactic.SelectService(rule)
	if got != primary {
		t.Fatalf("expected half-open probe to pick T0 primary, got %v", got)
	}
	// The probe succeeded → mark it so the breaker closes for good.
	store.RecordSuccess(rule.UUID, primary.ServiceID())
	if got := tactic.SelectService(rule); got != primary {
		t.Fatalf("expected return to T0 primary after recovery, got %v", got)
	}
}

func TestTierTiesShareLoad(t *testing.T) {
	a := mkService("tie-a", "m1", 0)
	b := mkService("tie-b", "m1", 0)
	backup := mkService("tie-c", "m1", 1)
	rule := mkTierRule(a, b, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		got := tactic.SelectService(rule)
		if got == backup {
			t.Fatalf("T1 backup should never be picked while T0 ties are healthy")
		}
		counts[got.ServiceID()]++
	}
	if counts[a.ServiceID()] == 0 || counts[b.ServiceID()] == 0 {
		t.Fatalf("random within-tier tactic should hit both T0 services, got %v", counts)
	}
}

func TestTierZeroIsHighestPriority(t *testing.T) {
	// T0 (tier=0) is the highest priority and is tried first.
	highest := mkService("ord-p1", "m1", 0)
	fallback := mkService("ord-p2", "m1", 1)
	rule := mkTierRule(highest, fallback)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	if got := tactic.SelectService(rule); got != highest {
		t.Fatalf("T0 should be tried first, got %v", got)
	}
}

func TestTierAllBreakersOpenReturnsLowestTier(t *testing.T) {
	// When every service is tripped we still pick something so the
	// upstream-error path can surface a real error to the client.
	high := mkService("allopen-a", "m1", 0)
	low := mkService("allopen-b", "m1", 1)
	rule := mkTierRule(high, low)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	store := loadbalance.DefaultBreakerStore()
	for _, svc := range []*loadbalance.Service{high, low} {
		for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
			store.RecordFailure(rule.UUID, svc.ServiceID())
		}
	}
	defer func() {
		store.RecordSuccess(rule.UUID, high.ServiceID())
		store.RecordSuccess(rule.UUID, low.ServiceID())
	}()

	got := tactic.SelectService(rule)
	if got == nil {
		t.Fatal("want a fallback service, got nil")
	}
	if got.Tier != 0 {
		t.Fatalf("want T0 fallback when all open, got tier=%d", got.Tier)
	}
}

// recoverPrimary trips the primary's breaker, then drives a full
// RecoveryThreshold-probe success streak to close it. The fake clock is
// advanced past OpenDuration between trips and probes. It returns the time
// recovery completed (ClosedSince) for hold assertions.
func recoverPrimary(t *testing.T, store *loadbalance.BreakerStore, ruleUUID, id string, now func() time.Time, advance func(time.Duration)) time.Time {
	t.Helper()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(ruleUUID, id)
	}
	advance(loadbalance.DefaultBreakerOpenDuration + time.Second)
	for i := 0; i < loadbalance.DefaultBreakerRecoveryThreshold; i++ {
		store.Allow(ruleUUID, id)         // first: Open → HalfOpen; later: next probe slot
		store.RecordSuccess(ruleUUID, id) // 3rd closes the breaker
	}
	since := store.Get(ruleUUID, id).ClosedSince()
	if since.IsZero() {
		t.Fatal("expected primary to have recovered (ClosedSince set)")
	}
	return since
}

// TestPromotionHoldKeepsFallbackPin verifies the core of plan B: a session
// pinned to a fallback tier (T1) is NOT vacuumed back to a freshly-recovered
// primary (T0) while T0 is within its PromotionHold. Only NEW sessions adopt
// T0 during the hold. After the hold elapses, the T1 pin becomes ineligible
// and the session migrates back.
func TestPromotionHoldKeepsFallbackPin(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()
	advance := func(d time.Duration) { now = now.Add(d) }

	primary := mkService("hold-p1", "m1", 0)
	backup := mkService("hold-p2", "m1", 1)
	rule := mkTierRule(primary, backup)
	store := loadbalance.DefaultBreakerStore()
	defer func() {
		// Reset primary for other tests sharing the global store.
		for i := 0; i < loadbalance.DefaultBreakerRecoveryThreshold; i++ {
			store.RecordSuccess(rule.UUID, primary.ServiceID())
		}
	}()

	// T0 trips, T1 takes over; the session is pinned to T1.
	recoverAt := recoverPrimary(t, store, rule.UUID, primary.ServiceID(), func() time.Time { return now }, advance)

	// T0 just recovered — within PromotionHold. A pin to T1 stays eligible.
	if !IsAffinityEligible(rule.UUID, rule.GetActiveServices(), backup) {
		t.Fatal("T1 pin should stay eligible while T0 is within PromotionHold")
	}
	// A pin to T0 is of course still eligible (it's the top tier).
	if !IsAffinityEligible(rule.UUID, rule.GetActiveServices(), primary) {
		t.Fatal("T0 pin should always be eligible when T0 is available")
	}

	// Advance past PromotionHold. Now the recovered primary has proven stable
	// and reclaims fallback-tier sessions.
	now = recoverAt.Add(loadbalance.DefaultPromotionHold + time.Second)
	if IsAffinityEligible(rule.UUID, rule.GetActiveServices(), backup) {
		t.Fatal("T1 pin should become ineligible after PromotionHold elapses (return to T0)")
	}
}

// TestPromotionHoldNewSessionGoesToPrimary verifies the other half: during the
// hold, NEW sessions (no pin) still adopt the recovered primary — the hold
// only retains EXISTING fallback pins, it never blocks new traffic from T0.
func TestPromotionHoldNewSessionGoesToPrimary(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()
	advance := func(d time.Duration) { now = now.Add(d) }

	primary := mkService("new-p1", "m1", 0)
	backup := mkService("new-p2", "m1", 1)
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)
	store := loadbalance.DefaultBreakerStore()

	recoverPrimary(t, store, rule.UUID, primary.ServiceID(), func() time.Time { return now }, advance)

	// New session: SelectService (no pin consulted) picks the recovered T0
	// even though we are inside PromotionHold.
	got := tactic.SelectService(rule)
	if got != primary {
		t.Fatalf("new session should adopt recovered T0 during hold, got %v", got)
	}
}

// TestTierSelectionDoesNotLeakHalfOpenProbe pins the two-phase selection
// contract: gathering candidates must not consume breaker probe slots; only
// the picked service claims one. Before the fix, a half-open service that
// shared a tier with a healthy peer had its probe slot consumed by Allow()
// during candidate collection even when the peer was picked — with no
// outcome ever reported, the slot stayed taken and the service could never
// finish recovering.
func TestTierSelectionDoesNotLeakHalfOpenProbe(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()

	a := mkService("leak-a", "m1", 0)
	b := mkService("leak-b", "m1", 0)
	rule := mkTierRule(a, b)
	tactic := NewTierTactic(loadbalance.TacticRandom)
	store := loadbalance.DefaultBreakerStore()

	for iter := 0; iter < 20; iter++ {
		// Trip A and advance past the open window so it is half-open-eligible.
		for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
			store.RecordFailure(rule.UUID, a.ServiceID())
		}
		now = now.Add(loadbalance.DefaultBreakerOpenDuration + time.Second)

		got := tactic.SelectService(rule)
		switch got {
		case b:
			// A was not picked: its probe slot must remain claimable.
			if !store.Allow(rule.UUID, a.ServiceID()) {
				t.Fatalf("iter %d: probe slot of unpicked half-open service was leaked", iter)
			}
		case a:
			// A claimed its own probe slot; a second claim must fail.
			if store.Allow(rule.UUID, a.ServiceID()) {
				t.Fatalf("iter %d: half-open probe slot was claimed twice", iter)
			}
		default:
			t.Fatalf("iter %d: unexpected selection %v", iter, got)
		}
		// Report a probe failure for A so it re-opens cleanly for the next round.
		store.RecordFailure(rule.UUID, a.ServiceID())
	}
}

// TestTierHalfOpenProbeHeldFallsBackToPeer verifies that when the only
// half-open service's probe slot is already in flight, selection re-picks
// among tier peers instead of returning the blocked service.
func TestTierHalfOpenProbeHeldFallsBackToPeer(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()

	a := mkService("held-a", "m1", 0)
	b := mkService("held-b", "m1", 0)
	rule := mkTierRule(a, b)
	tactic := NewTierTactic(loadbalance.TacticRandom)
	store := loadbalance.DefaultBreakerStore()

	// Trip A, pass the open window, and claim its probe slot out-of-band
	// (simulating another in-flight request holding the probe).
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(rule.UUID, a.ServiceID())
	}
	now = now.Add(loadbalance.DefaultBreakerOpenDuration + time.Second)
	if !store.Allow(rule.UUID, a.ServiceID()) {
		t.Fatal("setup: expected to claim A's probe slot")
	}

	for i := 0; i < 10; i++ {
		if got := tactic.SelectService(rule); got != b {
			t.Fatalf("expected peer B while A's probe is in flight, got %v", got)
		}
	}
}

// TestTierPreviewDoesNotClaimProbe pins the preview contract: PreviewService
// walks the same tiers as SelectService but never consumes the half-open
// probe slot, so a read-only preview cannot block real traffic's recovery
// probe.
func TestTierPreviewDoesNotClaimProbe(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()

	primary := mkService("prev-p1", "m1", 0)
	backup := mkService("prev-p2", "m1", 1)
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)
	store := loadbalance.DefaultBreakerStore()

	// Trip the primary and move past the open window: it is half-open-eligible.
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(rule.UUID, primary.ServiceID())
	}
	now = now.Add(loadbalance.DefaultBreakerOpenDuration + time.Second)

	// Preview repeatedly: it must pick the half-open primary (top available
	// tier) yet leave the probe slot unclaimed every time.
	for i := 0; i < 5; i++ {
		if got := tactic.PreviewService(rule); got != primary {
			t.Fatalf("preview %d: want half-open primary, got %v", i, got)
		}
	}
	if !store.Allow(rule.UUID, primary.ServiceID()) {
		t.Fatal("preview must not consume the half-open probe slot")
	}
	// Clean up: report a success so shared-store state doesn't leak.
	store.RecordSuccess(rule.UUID, primary.ServiceID())
}
