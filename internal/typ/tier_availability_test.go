package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// tripBreaker opens a service's breaker by recording the failure threshold.
func tripBreaker(svc *loadbalance.Service) {
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(svc.ServiceID())
	}
}

func TestIsInTopAvailableTier_TopTierAvailable(t *testing.T) {
	t0 := mkService("ita-a-p0", "m", 0)
	t1 := mkService("ita-a-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	if !IsInTopAvailableTier(svcs, t0) {
		t.Fatal("t0 should be in the top available tier when its breaker is closed")
	}
	if IsInTopAvailableTier(svcs, t1) {
		t.Fatal("t1 must NOT be in the top available tier while t0 (higher priority) is available")
	}
}

func TestIsInTopAvailableTier_TopTierOpen_NextBecomesTop(t *testing.T) {
	t0 := mkService("ita-b-p0", "m", 0)
	t1 := mkService("ita-b-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	tripBreaker(t0)
	defer loadbalance.DefaultBreakerStore().RecordSuccess(t0.ServiceID())

	if IsInTopAvailableTier(svcs, t0) {
		t.Fatal("t0 is open; it should not be the top available tier")
	}
	if !IsInTopAvailableTier(svcs, t1) {
		t.Fatal("t1 should become the top available tier while t0 is open")
	}
}

func TestIsInTopAvailableTier_AllOpen_FallsBackToLowestTier(t *testing.T) {
	t0 := mkService("ita-c-p0", "m", 0)
	t1 := mkService("ita-c-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	tripBreaker(t0)
	tripBreaker(t1)
	defer func() {
		store := loadbalance.DefaultBreakerStore()
		store.RecordSuccess(t0.ServiceID())
		store.RecordSuccess(t1.ServiceID())
	}()

	// Degrade-don't-disappear: the lowest-numbered tier is the fallback top.
	if !IsInTopAvailableTier(svcs, t0) {
		t.Fatal("when every tier is open, the lowest tier (t0) is the fallback top")
	}
	if IsInTopAvailableTier(svcs, t1) {
		t.Fatal("t1 is not the fallback top when every tier is open")
	}
}

func TestIsInTopAvailableTier_NilAndEmpty(t *testing.T) {
	t0 := mkService("ita-d-p0", "m", 0)
	if IsInTopAvailableTier([]*loadbalance.Service{t0}, nil) {
		t.Fatal("nil target should be false")
	}
	if IsInTopAvailableTier(nil, t0) {
		t.Fatal("empty service set should be false")
	}
}
