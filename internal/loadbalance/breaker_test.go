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

// windForward moves a breaker's openedAt backwards in time so the open
// window appears to have elapsed, without any real time.Sleep.
func windForward(b *Breaker, d time.Duration) {
	b.mu.Lock()
	b.openedAt = b.openedAt.Add(-d)
	b.mu.Unlock()
}

func TestBreakerExponentialBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	b := NewBreaker(1, base)
	b.MaxOpenDuration = 500 * time.Millisecond

	b.RecordFailure() // → open

	// 1st window: base (100 ms). Fast-forward past it.
	windForward(b, base)
	if !b.Allow() {
		t.Fatal("should allow probe after 1st open window")
	}
	b.RecordFailure() // half-open → open, halfOpenFails=1

	// 2nd window: 200 ms. Advance only 150 ms — too early.
	windForward(b, 150*time.Millisecond)
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (backoff doubled to 200 ms)")
	}
	// Advance the remaining 50 ms.
	windForward(b, 50*time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after 2nd backoff window")
	}
	b.RecordFailure() // halfOpenFails=2

	// 3rd window: 400 ms. Advance only 300 ms — too early.
	windForward(b, 300*time.Millisecond)
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (backoff at 400 ms)")
	}
	windForward(b, 100*time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after 3rd backoff window")
	}
	b.RecordFailure() // halfOpenFails=3, next would be 800 ms → capped at 500 ms

	// 4th window: capped at 500 ms. Advance only 450 ms — too early.
	windForward(b, 450*time.Millisecond)
	if b.Allow() {
		t.Fatal("should NOT allow probe yet (capped at 500 ms)")
	}
	windForward(b, 50*time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after cap-limited window")
	}
}

func TestBreakerBackoffResetsOnSuccess(t *testing.T) {
	base := 100 * time.Millisecond
	b := NewBreaker(1, base)
	b.MaxOpenDuration = 5 * time.Second

	// Build up backoff: trip → probe fail → trip → probe fail.
	b.RecordFailure()
	windForward(b, base)
	b.Allow()
	b.RecordFailure() // halfOpenFails=1, next window=200 ms

	windForward(b, 200*time.Millisecond)
	b.Allow()

	// Probe succeeds → everything resets.
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatal("should be closed after success")
	}

	// Trip again — backoff should be back to base, not 400 ms.
	b.RecordFailure()
	windForward(b, base)
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
