package loadbalance

import (
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/clock"
)

// testRule is the ruleUUID used by store-level tests. The store is rule-scoped,
// so every call needs a ruleUUID; a single constant keeps these tests isolated
// (their serviceIDs are already unique per test).
const testRule = "breaker-test-rule"

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := NewBreaker(3, time.Second)
	if !b.Allow() {
		t.Fatal("fresh breaker should allow")
	}
	for i := 0; i < 3; i++ {
		b.RecordFailure()
	}
	if b.Allow() {
		t.Fatal("breaker should be open after 3 failures")
	}
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want open", b.State())
	}
}

func TestBreakerHalfOpenAfterDuration(t *testing.T) {
	b := NewBreaker(2, 20*time.Millisecond)
	b.RecoveryThreshold = 1 // keep this test focused on the open/half-open path, not hysteresis
	b.RecordFailure()
	b.RecordFailure()
	if b.Allow() {
		t.Fatal("breaker should be open")
	}
	time.Sleep(30 * time.Millisecond)

	// First Allow() after expiry should pass (half-open probe).
	if !b.Allow() {
		t.Fatal("breaker should let one probe through after duration")
	}
	// Concurrent caller must not also pass while probe is in flight.
	if b.Allow() {
		t.Fatal("half-open should only allow one probe at a time")
	}

	// Probe success → closed.
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatalf("state after success = %v, want closed", b.State())
	}
	if !b.Allow() {
		t.Fatal("closed breaker should allow")
	}
}

func TestBreakerHalfOpenFailureReopens(t *testing.T) {
	b := NewBreaker(1, 10*time.Millisecond)
	b.RecoveryThreshold = 1
	b.RecordFailure() // → open
	time.Sleep(15 * time.Millisecond)
	b.Allow() // → half-open
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want open after half-open failure", b.State())
	}
}

func TestBreakerSuccessResetsCounter(t *testing.T) {
	b := NewBreaker(3, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess()
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != BreakerClosed {
		t.Fatalf("state = %v, want closed (success should reset)", b.State())
	}
}

func TestBreakerStoreLazyCreation(t *testing.T) {
	store := NewBreakerStore(2, time.Second)
	b1 := store.Get(testRule, "svc:a")
	b2 := store.Get(testRule, "svc:a")
	if b1 != b2 {
		t.Fatal("store should return the same breaker for the same key")
	}
	if store.Get(testRule, "svc:b") == b1 {
		t.Fatal("store should return different breakers for different keys")
	}
}

func TestBreakerStoreIsAvailable(t *testing.T) {
	store := NewBreakerStore(2, 20*time.Millisecond)

	// Closed → available.
	if !store.IsAvailable(testRule, "svc:avail") {
		t.Fatal("fresh (closed) breaker should be available")
	}

	// Open → not available.
	store.RecordFailure(testRule, "svc:avail")
	store.RecordFailure(testRule, "svc:avail")
	if store.IsAvailable(testRule, "svc:avail") {
		t.Fatal("open breaker should not be available")
	}

	// After the open window, HalfOpen → available, and the read must NOT
	// consume the half-open probe: a following Allow() must still succeed.
	time.Sleep(30 * time.Millisecond)
	if !store.IsAvailable(testRule, "svc:avail") {
		t.Fatal("half-open breaker should report available")
	}
	if !store.Allow(testRule, "svc:avail") {
		t.Fatal("IsAvailable must not consume the half-open probe; Allow() should still pass")
	}
	// The probe is now claimed, so a concurrent Allow() is rejected.
	if store.Allow(testRule, "svc:avail") {
		t.Fatal("half-open should only admit a single probe")
	}
}

func TestBreakerStoreConcurrent(t *testing.T) {
	store := NewBreakerStore(5, time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Allow(testRule, "svc:concurrent")
			store.RecordFailure(testRule, "svc:concurrent")
		}()
	}
	wg.Wait()
	// Just need to ensure no panic / race; state is now Open.
	if store.Get(testRule, "svc:concurrent").State() != BreakerOpen {
		t.Fatal("after many failures the breaker should be open")
	}
}

// fakeClock is a controllable time source for deterministic breaker tests.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) advance(d time.Duration) { f.now = f.now.Add(d) }

// withFakeClock installs a controllable clock and returns the clock, a restore
// function, and a helper to advance it. The breaker reads time via clock.Now.
func withFakeClock(t *testing.T) (*fakeClock, func()) {
	t.Helper()
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	restore := clock.SetClock(func() time.Time { return fc.now })
	return fc, restore
}

// TestBreakerHysteresisRequiresConsecutiveSuccesses verifies that a single
// half-open probe success no longer closes the breaker — the core anti-
// oscillation change. RecoveryThreshold (default 3) consecutive successes
// are required.
func TestBreakerHysteresisRequiresConsecutiveSuccesses(t *testing.T) {
	fc, restore := withFakeClock(t)
	defer restore()

	b := NewBreaker(3, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordFailure() // → Open
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want open", b.State())
	}

	// Open window elapses → HalfOpen. First probe succeeds but must NOT close.
	fc.advance(time.Second)
	if !b.Allow() {
		t.Fatal("first probe after open window should be allowed")
	}
	b.RecordSuccess()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("after 1 success state = %v, want half_open (not closed)", b.State())
	}

	// Second probe: Allow again (previous success released the slot), succeed, still not closed.
	if !b.Allow() {
		t.Fatal("second probe should be allowed")
	}
	b.RecordSuccess()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("after 2 successes state = %v, want half_open", b.State())
	}

	// Third consecutive success → Closed.
	if !b.Allow() {
		t.Fatal("third probe should be allowed")
	}
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatalf("after 3 successes state = %v, want closed", b.State())
	}
}

// TestBreakerHysteresisResetsOnFailure verifies that a probe failure during
// the recovery streak resets the success counter and re-opens, requiring a
// fresh full streak afterwards.
func TestBreakerHysteresisResetsOnFailure(t *testing.T) {
	fc, restore := withFakeClock(t)
	defer restore()

	b := NewBreaker(2, time.Second)
	b.RecordFailure()
	b.RecordFailure() // → Open

	// Two successes (below the default threshold of 3).
	fc.advance(time.Second)
	b.Allow()
	b.RecordSuccess()
	b.Allow()
	b.RecordSuccess()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("state = %v, want half_open after 2 successes", b.State())
	}

	// A failure now resets the streak and re-opens, restarting the open timer.
	b.Allow()
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatalf("state = %v, want open after failure reset the streak", b.State())
	}

	// After the open window (base duration), a single success is not enough.
	fc.advance(time.Second)
	b.Allow()
	b.RecordSuccess()
	if b.State() != BreakerHalfOpen {
		t.Fatalf("state = %v, want half_open (streak was reset)", b.State())
	}
}

// TestBreakerClosedSince verifies the recovery timestamp that drives tier
// affinity's PromotionHold: it is stamped when recovery completes and cleared
// on any subsequent open.
func TestBreakerClosedSince(t *testing.T) {
	fc, restore := withFakeClock(t)
	defer restore()

	b := NewBreaker(2, time.Second)
	b.RecoveryThreshold = 1

	if !b.ClosedSince().IsZero() {
		t.Fatal("ClosedSince should be zero on a fresh breaker")
	}

	// Trip, then recover.
	b.RecordFailure()
	b.RecordFailure() // → Open
	if !b.ClosedSince().IsZero() {
		t.Fatal("ClosedSince should be zero while open")
	}
	fc.advance(time.Second)
	b.Allow() // Open → HalfOpen
	recoverAt := fc.now
	b.RecordSuccess() // HalfOpen → Closed
	if got := b.ClosedSince(); !got.Equal(recoverAt) {
		t.Fatalf("ClosedSince = %v, want %v", got, recoverAt)
	}

	// A new failure trips again and clears ClosedSince.
	b.RecordFailure()
	b.RecordFailure() // → Open
	if !b.ClosedSince().IsZero() {
		t.Fatalf("ClosedSince = %v, want zero after re-open", b.ClosedSince())
	}
}

// TestBreakerStoreWithinPromotionHold verifies the store helper that tier
// affinity reads: a freshly-recovered service is within the hold, and leaves
// it once the hold window elapses (or if it never tripped / re-opened).
func TestBreakerStoreWithinPromotionHold(t *testing.T) {
	fc, restore := withFakeClock(t)
	defer restore()

	store := NewBreakerStore(1, time.Second)
	id := "svc:hold"

	// Never tripped → not within hold (no recovery happened).
	if store.WithinPromotionHold(testRule, id, 60*time.Second) {
		t.Fatal("never-tripped service should not be within promotion hold")
	}

	// Trip and recover. RecoveryThreshold defaults to 3, so drive a full
	// 3-probe success streak to close the breaker.
	store.RecordFailure(testRule, id) // → Open
	fc.advance(time.Second)
	for i := 0; i < 3; i++ {
		store.Allow(testRule, id)         // first: Open → HalfOpen; later: next probe slot
		store.RecordSuccess(testRule, id) // accumulate; 3rd closes the breaker
	}

	// Right after recovery: within a 60s hold.
	if !store.WithinPromotionHold(testRule, id, 60*time.Second) {
		t.Fatal("freshly recovered service should be within 60s promotion hold")
	}
	// hold <= 0 disables the check entirely.
	if store.WithinPromotionHold(testRule, id, 0) {
		t.Fatal("hold<=0 should disable the check (always false)")
	}

	// 60s+ later: out of the hold.
	fc.advance(61 * time.Second)
	if store.WithinPromotionHold(testRule, id, 60*time.Second) {
		t.Fatal("service recovered >60s ago should be out of the hold")
	}
}

// TestBreakerStoreIsRuleScoped is the regression guard for rule-scoped breakers:
// tripping a service under one rule must NOT trip the same serviceID under
// another rule. This is the whole point of the re-keying — a busy rule's
// failing traffic cannot fail over a different rule that uses the same
// provider:model as its primary.
func TestBreakerStoreIsRuleScoped(t *testing.T) {
	store := NewBreakerStore(3, time.Second)
	const (
		ruleA = "rule-a"
		ruleB = "rule-b"
		sid   = "provider-x/model-y"
	)

	// Trip the service under ruleA only.
	for i := 0; i < DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(ruleA, sid)
	}

	if store.Get(ruleA, sid).State() != BreakerOpen {
		t.Fatal("ruleA's breaker for the shared service should be open after tripping")
	}
	if store.Get(ruleB, sid).State() != BreakerClosed {
		t.Fatal("ruleB's breaker for the SAME service must stay closed (rule-scoped isolation)")
	}
	// Concretely: ruleB still admits the service, ruleA does not.
	if !store.Allow(ruleB, sid) {
		t.Fatal("ruleB should still allow the service")
	}
	if store.Allow(ruleA, sid) {
		t.Fatal("ruleA should reject the service while its breaker is open")
	}
}

func TestBreakerStaleHalfOpenProbeReclaimed(t *testing.T) {
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()

	b := NewBreaker(3, 30*time.Second)
	for i := 0; i < 3; i++ {
		b.RecordFailure()
	}
	now = now.Add(31 * time.Second)
	if !b.Allow() {
		t.Fatal("expected the half-open probe slot after the open window")
	}
	// The claimer never reports an outcome (e.g. selected but never
	// dispatched). Within the window the slot stays held...
	now = now.Add(10 * time.Second)
	if b.Allow() {
		t.Fatal("probe slot should still be held within the window")
	}
	// ...but after OpenDuration without an outcome it is reclaimed, so the
	// service can still recover instead of being wedged forever.
	now = now.Add(21 * time.Second)
	if !b.Allow() {
		t.Fatal("stale probe slot should be reclaimed after OpenDuration")
	}
}
