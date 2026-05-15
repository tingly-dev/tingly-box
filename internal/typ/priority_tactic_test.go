package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func mkService(provider, model string, priority int) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Active:   true,
		Priority: priority,
	}
}

func mkPriorityRule(services ...*loadbalance.Service) *Rule {
	return &Rule{
		UUID:     "rule-test",
		Services: services,
		Active:   true,
		LBTactic: Tactic{
			Type:   loadbalance.TacticPriority,
			Params: DefaultPriorityParams(),
		},
	}
}

func TestPriorityPicksHighestFirst(t *testing.T) {
	primary := mkService("p1", "m1", 10)
	backup := mkService("p2", "m1", 5)
	rule := mkPriorityRule(primary, backup)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	got := tactic.SelectService(rule)
	if got != primary {
		t.Fatalf("want primary (priority=10), got %v", got)
	}
}

func TestPriorityFallsBackWhenBreakerOpen(t *testing.T) {
	primary := mkService("fb-p1", "m1", 10)
	backup := mkService("fb-p2", "m1", 5)
	rule := mkPriorityRule(primary, backup)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	// Trip the primary's breaker.
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(primary.ServiceID())
	}
	defer store.RecordSuccess(primary.ServiceID()) // clean up for other tests

	got := tactic.SelectService(rule)
	if got != backup {
		t.Fatalf("primary breaker open, want backup, got %v", got)
	}
}

func TestPriorityReturnsToHigherWhenBreakerCloses(t *testing.T) {
	primary := mkService("recover-p1", "recover-m1", 10)
	backup := mkService("recover-p2", "recover-m1", 5)
	rule := mkPriorityRule(primary, backup)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(primary.ServiceID())
	}
	if got := tactic.SelectService(rule); got != backup {
		t.Fatalf("expected fallback to backup, got %v", got)
	}
	store.RecordSuccess(primary.ServiceID())
	if got := tactic.SelectService(rule); got != primary {
		t.Fatalf("expected return to primary after recovery, got %v", got)
	}
}

func TestPriorityTiesShareLoad(t *testing.T) {
	a := mkService("tie-a", "m1", 10)
	b := mkService("tie-b", "m1", 10)
	backup := mkService("tie-c", "m1", 5)
	rule := mkPriorityRule(a, b, backup)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		got := tactic.SelectService(rule)
		if got == backup {
			t.Fatalf("backup should never be picked while ties are healthy")
		}
		counts[got.ServiceID()]++
	}
	if counts[a.ServiceID()] == 0 || counts[b.ServiceID()] == 0 {
		t.Fatalf("random within-tier tactic should hit both tied services, got %v", counts)
	}
}

func TestPriorityZeroTreatedAsUnset(t *testing.T) {
	prioritised := mkService("ord-p1", "m1", 1)
	unset := mkService("ord-p2", "m1", 0)
	rule := mkPriorityRule(prioritised, unset)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	// Any explicit priority (even 1) beats the unset (0) tier.
	if got := tactic.SelectService(rule); got != prioritised {
		t.Fatalf("explicit priority should beat unset, got %v", got)
	}
}

func TestPriorityAllBreakersOpenReturnsHighestTier(t *testing.T) {
	// Even when every service is tripped we still pick something so the
	// upstream-error path can surface a real error to the client.
	high := mkService("allopen-a", "m1", 10)
	low := mkService("allopen-b", "m1", 1)
	rule := mkPriorityRule(high, low)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

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
	if got.Priority != 10 {
		t.Fatalf("want priority=10 fallback when all open, got priority=%d", got.Priority)
	}
}
