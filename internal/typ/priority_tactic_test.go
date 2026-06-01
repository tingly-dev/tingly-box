package typ

import (
	"testing"

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
		store.RecordFailure(primary.ServiceID())
	}
	defer store.RecordSuccess(primary.ServiceID()) // clean up for other tests

	got := tactic.SelectService(rule)
	if got != backup {
		t.Fatalf("T0 breaker open, want T1 backup, got %v", got)
	}
}

func TestTierReturnsToHigherWhenBreakerCloses(t *testing.T) {
	primary := mkService("recover-p1", "recover-m1", 0)
	backup := mkService("recover-p2", "recover-m1", 1)
	rule := mkTierRule(primary, backup)
	tactic := NewTierTactic(loadbalance.TacticRandom)

	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(primary.ServiceID())
	}
	if got := tactic.SelectService(rule); got != backup {
		t.Fatalf("expected fallback to T1 backup, got %v", got)
	}
	store.RecordSuccess(primary.ServiceID())
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
			store.RecordFailure(svc.ServiceID())
		}
	}
	defer func() {
		store.RecordSuccess(high.ServiceID())
		store.RecordSuccess(low.ServiceID())
	}()

	got := tactic.SelectService(rule)
	if got == nil {
		t.Fatal("want a fallback service, got nil")
	}
	if got.Tier != 0 {
		t.Fatalf("want T0 fallback when all open, got tier=%d", got.Tier)
	}
}
