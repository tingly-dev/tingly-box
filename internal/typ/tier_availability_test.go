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

// Shape 3 (many tiers): the pin is eligible only in the top available tier.
func TestIsAffinityEligible_TopTierAvailable(t *testing.T) {
	t0 := mkService("ita-a-p0", "m", 0)
	t1 := mkService("ita-a-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	if !IsAffinityEligible(svcs, t0) {
		t.Fatal("t0 should be eligible when its breaker is closed")
	}
	if IsAffinityEligible(svcs, t1) {
		t.Fatal("t1 must NOT be eligible while t0 (higher priority) is available")
	}
}

func TestIsAffinityEligible_TopTierOpen_NextBecomesTop(t *testing.T) {
	t0 := mkService("ita-b-p0", "m", 0)
	t1 := mkService("ita-b-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	tripBreaker(t0)
	defer loadbalance.DefaultBreakerStore().RecordSuccess(t0.ServiceID())

	if IsAffinityEligible(svcs, t0) {
		t.Fatal("t0 is open; it should not be eligible")
	}
	if !IsAffinityEligible(svcs, t1) {
		t.Fatal("t1 should become eligible while t0 is open")
	}
}

// Shape 2 (one tier, many services): a pin to a dead peer must be dropped while
// healthy peers exist; a pin to a healthy peer is honored (stickiness).
func TestIsAffinityEligible_SameTierDeadPeerDropped(t *testing.T) {
	a := mkService("ita-peer-a", "m", 0)
	b := mkService("ita-peer-b", "m", 0)
	svcs := []*loadbalance.Service{a, b}

	tripBreaker(a)
	defer loadbalance.DefaultBreakerStore().RecordSuccess(a.ServiceID())

	if IsAffinityEligible(svcs, a) {
		t.Fatal("pin to a dead same-tier peer must be dropped when a healthy peer exists")
	}
	if !IsAffinityEligible(svcs, b) {
		t.Fatal("pin to a healthy same-tier peer must be honored")
	}
}

func TestIsAffinityEligible_AllOpen_FallsBackToLowestTier(t *testing.T) {
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

	// Degrade-don't-disappear: the lowest-numbered tier is the fallback.
	if !IsAffinityEligible(svcs, t0) {
		t.Fatal("when everything is open, a pin to the lowest tier (t0) is honored")
	}
	if IsAffinityEligible(svcs, t1) {
		t.Fatal("t1 is not the fallback when everything is open")
	}
}

// Shape 1 (single service): always eligible, even while its breaker is open
// (nothing else to pick — failover/upstream-error path handles it).
func TestIsAffinityEligible_SingleService(t *testing.T) {
	only := mkService("ita-solo", "m", 0)
	svcs := []*loadbalance.Service{only}

	if !IsAffinityEligible(svcs, only) {
		t.Fatal("single healthy service must be eligible")
	}

	tripBreaker(only)
	defer loadbalance.DefaultBreakerStore().RecordSuccess(only.ServiceID())
	if !IsAffinityEligible(svcs, only) {
		t.Fatal("single service must stay eligible even when its breaker is open")
	}
}

// An inactive service whose breaker is closed must not make its tier look
// "available" and demote a healthy pin in a lower tier.
func TestIsAffinityEligible_InactiveDoesNotMaskLowerTier(t *testing.T) {
	t0 := mkService("ita-inact-p0", "m", 0)
	t0.Active = false // configured but disabled; breaker untouched (closed)
	t1 := mkService("ita-inact-p1", "m", 1)
	svcs := []*loadbalance.Service{t0, t1}

	// t0 is inactive → top active tier is t1 → a pin to t1 is eligible.
	if !IsAffinityEligible(svcs, t1) {
		t.Fatal("t1 should be eligible when the only higher tier is inactive")
	}
}

func TestIsAffinityEligible_InactiveTargetDeclined(t *testing.T) {
	a := mkService("ita-inact-target-a", "m", 0)
	a.Active = false
	b := mkService("ita-inact-target-b", "m", 0)
	svcs := []*loadbalance.Service{a, b}

	if IsAffinityEligible(svcs, a) {
		t.Fatal("an inactive pinned service must never be eligible")
	}
	if !IsAffinityEligible(svcs, b) {
		t.Fatal("the active peer should still be eligible")
	}
}

func TestIsAffinityEligible_NilAndEmpty(t *testing.T) {
	t0 := mkService("ita-d-p0", "m", 0)
	if IsAffinityEligible([]*loadbalance.Service{t0}, nil) {
		t.Fatal("nil target should be false")
	}
	if IsAffinityEligible(nil, t0) {
		t.Fatal("empty service set should be false")
	}
}
