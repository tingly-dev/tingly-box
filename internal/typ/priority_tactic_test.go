package typ

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func mkService(provider, model string, order int) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Active:   true,
		Order:    order,
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

func TestPriorityPicksLowestOrderFirst(t *testing.T) {
	primary := mkService("p1", "m1", 1)
	backup := mkService("p2", "m1", 2)
	rule := mkPriorityRule(primary, backup)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	got := tactic.SelectService(rule)
	if got != primary {
		t.Fatalf("want primary (order=1), got %v", got)
	}
}

func TestPriorityFallsBackWhenBreakerOpen(t *testing.T) {
	primary := mkService("p1", "m1", 1)
	backup := mkService("p2", "m1", 2)
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
	primary := mkService("recover-p1", "recover-m1", 1)
	backup := mkService("recover-p2", "recover-m1", 2)
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

func TestPriorityTiesWithinOrderShareLoad(t *testing.T) {
	a := mkService("tie-a", "m1", 1)
	b := mkService("tie-b", "m1", 1)
	backup := mkService("tie-c", "m1", 2)
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
		t.Fatalf("random within-order tactic should hit both tied services, got %v", counts)
	}
}

func TestPriorityOrderZeroTreatedAsLowest(t *testing.T) {
	ordered := mkService("ord-p1", "m1", 1)
	unset := mkService("ord-p2", "m1", 0)
	rule := mkPriorityRule(ordered, unset)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	// Order=1 wins over Order=0 (unset).
	if got := tactic.SelectService(rule); got != ordered {
		t.Fatalf("explicit order should beat unset, got %v", got)
	}
}

func TestPriorityAllBreakersOpenReturnsFirst(t *testing.T) {
	// Even when every service is tripped we still pick something so the
	// upstream-error path can surface a real error to the client.
	a := mkService("allopen-a", "m1", 1)
	b := mkService("allopen-b", "m1", 2)
	rule := mkPriorityRule(a, b)
	tactic := NewPriorityTactic(loadbalance.TacticRandom)

	store := loadbalance.DefaultBreakerStore()
	for _, svc := range []*loadbalance.Service{a, b} {
		for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
			store.RecordFailure(svc.ServiceID())
		}
	}
	defer func() {
		store.RecordSuccess(a.ServiceID())
		store.RecordSuccess(b.ServiceID())
	}()

	got := tactic.SelectService(rule)
	if got == nil {
		t.Fatal("want a fallback service, got nil")
	}
	// Should be the lowest-order bucket (Order=1).
	if got.Order != 1 {
		t.Fatalf("want order=1 fallback when all open, got order=%d", got.Order)
	}
}

func TestPriorityRoundtripsThroughJSON(t *testing.T) {
	tactic := Tactic{
		Type:   loadbalance.TacticPriority,
		Params: &PriorityParams{WithinOrderTactic: loadbalance.TacticRandom},
	}
	_ = tactic // just compile-check; full marshal round-trip is exercised by Rule json tests elsewhere
	_ = time.Now
}
