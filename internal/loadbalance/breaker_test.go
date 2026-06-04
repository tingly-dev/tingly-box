package loadbalance

import (
	"sync"
	"testing"
	"time"
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

func TestBreakerExponentialBackoff(t *testing.T) {
	base := 20 * time.Millisecond
	b := NewBreaker(1, base)
	b.MaxOpenDuration = 200 * time.Millisecond

	// Trip the breaker.
	b.RecordFailure()

	// 1st open window: base (20 ms).
	time.Sleep(base + 5*time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after 1st open window")
	}
	b.RecordFailure() // half-open → open, halfOpenFails=1

	// 2nd open window: 40 ms (base * 2^1).
	time.Sleep(base + 5*time.Millisecond) // 25 ms — too early
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (backoff doubled to 40 ms)")
	}
	time.Sleep(base) // total ~45 ms ≥ 40 ms
	if !b.Allow() {
		t.Fatal("should allow probe after 2nd backoff window")
	}
	b.RecordFailure() // halfOpenFails=2

	// 3rd open window: 80 ms (base * 2^2).
	time.Sleep(60 * time.Millisecond) // too early for 80 ms
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (backoff at 80 ms)")
	}
	time.Sleep(25 * time.Millisecond) // total ~85 ms ≥ 80 ms
	if !b.Allow() {
		t.Fatal("should allow probe after 3rd backoff window")
	}
	b.RecordFailure() // halfOpenFails=3, next would be 160 ms

	// 4th open window: 160 ms (base * 2^3).
	time.Sleep(165 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after 4th backoff window")
	}
	b.RecordFailure() // halfOpenFails=4, next would be 320 ms → capped at 200 ms

	// 5th open window: capped at 200 ms.
	time.Sleep(195 * time.Millisecond) // just under cap
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (capped at 200 ms)")
	}
	time.Sleep(10 * time.Millisecond) // total ~205 ms ≥ 200 ms
	if !b.Allow() {
		t.Fatal("should allow probe after cap-limited window")
	}
}

func TestBreakerBackoffResetsOnSuccess(t *testing.T) {
	base := 20 * time.Millisecond
	b := NewBreaker(1, base)
	b.MaxOpenDuration = 500 * time.Millisecond

	// Build up backoff: trip → probe fail → trip → probe fail.
	b.RecordFailure()
	time.Sleep(base + 5*time.Millisecond)
	b.Allow()
	b.RecordFailure() // halfOpenFails=1, next window=40 ms

	time.Sleep(45 * time.Millisecond)
	b.Allow()

	// Probe succeeds → everything resets.
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatal("should be closed after success")
	}

	// Trip again — backoff should be back to base, not 80 ms.
	b.RecordFailure()
	time.Sleep(base + 5*time.Millisecond)
	if !b.Allow() {
		t.Fatal("after success+retrip, should probe at base duration, not backed-off")
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
