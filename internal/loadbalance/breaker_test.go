package loadbalance

import (
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/clock"
)

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
	b1 := store.Get("svc:a")
	b2 := store.Get("svc:a")
	if b1 != b2 {
		t.Fatal("store should return the same breaker for the same key")
	}
	if store.Get("svc:b") == b1 {
		t.Fatal("store should return different breakers for different keys")
	}
}

func TestBreakerStoreIsAvailable(t *testing.T) {
	store := NewBreakerStore(2, 20*time.Millisecond)

	// Closed → available.
	if !store.IsAvailable("svc:avail") {
		t.Fatal("fresh (closed) breaker should be available")
	}

	// Open → not available.
	store.RecordFailure("svc:avail")
	store.RecordFailure("svc:avail")
	if store.IsAvailable("svc:avail") {
		t.Fatal("open breaker should not be available")
	}

	// After the open window, HalfOpen → available, and the read must NOT
	// consume the half-open probe: a following Allow() must still succeed.
	time.Sleep(30 * time.Millisecond)
	if !store.IsAvailable("svc:avail") {
		t.Fatal("half-open breaker should report available")
	}
	if !store.Allow("svc:avail") {
		t.Fatal("IsAvailable must not consume the half-open probe; Allow() should still pass")
	}
	// The probe is now claimed, so a concurrent Allow() is rejected.
	if store.Allow("svc:avail") {
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
			store.Allow("svc:concurrent")
			store.RecordFailure("svc:concurrent")
		}()
	}
	wg.Wait()
	// Just need to ensure no panic / race; state is now Open.
	if store.Get("svc:concurrent").State() != BreakerOpen {
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
