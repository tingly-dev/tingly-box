package server

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestTierRouting_EndToEnd exercises the full chain the frontend
// triggers when a user assigns a tier to any service:
//
//  1. A rule JSON arrives carrying lb_tactic.type = "tier".
//  2. The Rule is unmarshalled — Tactic.UnmarshalJSON allocates *TierParams.
//  3. LoadBalancer.SelectService is called for that rule.
//  4. Internally rule.LBTactic.Instantiate() routes through
//     CreateTacticWithTypedParams → NewTierTactic.
//  5. TierTactic.SelectService groups by Tier (lower = higher priority)
//     and consults the process-wide breaker store.
//  6. After enough failures on the T0 service the breaker trips and the
//     next call falls back to T1; once the breaker closes it returns to T0.
func TestTierRouting_EndToEnd(t *testing.T) {
	ruleJSON := `{
		"uuid": "rule-e2e",
		"scenario": "openai",
		"request_model": "gpt-4",
		"active": true,
		"services": [
			{"provider": "e2e-primary",  "model": "gpt-4", "active": true, "tier": 0},
			{"provider": "e2e-fallback", "model": "gpt-4", "active": true, "tier": 1}
		],
		"lb_tactic": {
			"type": "tier",
			"params": {"within_tier_tactic": "random"}
		}
	}`

	var rule typ.Rule
	if err := json.Unmarshal([]byte(ruleJSON), &rule); err != nil {
		t.Fatalf("rule unmarshal failed: %v", err)
	}

	// Sanity-check the JSON contract the frontend depends on.
	if rule.LBTactic.Type != loadbalance.TacticTier {
		t.Fatalf("lb_tactic.type = %v, want TacticTier", rule.LBTactic.Type)
	}
	pp, ok := rule.LBTactic.Params.(*typ.TierParams)
	if !ok || pp == nil {
		t.Fatalf("LBTactic.Params = %T, want *TierParams", rule.LBTactic.Params)
	}
	if pp.WithinTierTactic != loadbalance.TacticRandom {
		t.Fatalf("WithinTierTactic = %v, want random", pp.WithinTierTactic)
	}

	// Build a load balancer with a no-op health filter so the only
	// thing that can hide a service from the tactic is its breaker.
	hf := typ.NewHealthFilter(nil)
	lb := &LoadBalancer{
		tactics:      make(map[loadbalance.TacticType]typ.LoadBalancingTactic),
		healthFilter: hf,
	}

	primaryID := "e2e-primary/gpt-4"
	fallbackID := "e2e-fallback/gpt-4"

	// 1. Fresh state: the T0 service is chosen.
	got, err := lb.SelectService(&rule)
	if err != nil {
		t.Fatalf("initial SelectService: %v", err)
	}
	if got.ServiceID() != primaryID {
		t.Fatalf("initial pick = %s, want %s", got.ServiceID(), primaryID)
	}

	// 2. Trip the primary's breaker via the package-level store — this
	//    is the same store the recorder writes into on real failures.
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(primaryID)
	}
	defer store.RecordSuccess(primaryID)
	defer store.RecordSuccess(fallbackID)

	got, err = lb.SelectService(&rule)
	if err != nil {
		t.Fatalf("after-trip SelectService: %v", err)
	}
	if got.ServiceID() != fallbackID {
		t.Fatalf("after-trip pick = %s, want fallback %s", got.ServiceID(), fallbackID)
	}

	// 3. Mark the primary healthy again (mirrors what RecordResponse →
	//    RecordServiceSuccess does at the end of a good request).
	store.RecordSuccess(primaryID)

	got, err = lb.SelectService(&rule)
	if err != nil {
		t.Fatalf("after-recovery SelectService: %v", err)
	}
	if got.ServiceID() != primaryID {
		t.Fatalf("after-recovery pick = %s, want %s", got.ServiceID(), primaryID)
	}
}
