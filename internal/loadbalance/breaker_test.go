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
