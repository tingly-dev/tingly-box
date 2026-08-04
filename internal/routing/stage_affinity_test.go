package routing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// tierService builds an active service at the given tier for affinity tests.
func tierService(provider, model string, tier int) *loadbalance.Service {
	svc := testService(provider, model, true)
	svc.Tier = tier
	return svc
}

// tierRule builds a tier-tactic rule with affinity enabled.
func tierRule(uuid, model string, services []*loadbalance.Service) *typ.Rule {
	rule := testRule(uuid, model, services)
	rule.Flags.SessionAffinity = 3600
	rule.LBTactic = typ.Tactic{Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()}
	return rule
}

func TestAffinity_LockedSession(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", testSessionKey("session-1"), testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "session-1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "should return handled=true for locked session")
	require.NotNil(t, result)
	require.Equal(t, "gpt-4", result.Service.Model)
	require.Equal(t, "affinity", result.Source)
}

func TestAffinity_NoLock(t *testing.T) {
	store := newMockAffinityStore()

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "session-1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass to next stage when no lock")
	require.Nil(t, result)
}

func TestAffinity_AffinityDisabled(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", testSessionKey("session-1"), testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 0 // disabled

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "session-1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when affinity disabled")
}

func TestAffinity_SmartDisabled(t *testing.T) {
	// Affinity is a load-balancing concern, independent of smart routing.
	// A locked session must still be honored even when SmartEnabled is false.
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", testSessionKey("session-1"), testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = false
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "session-1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "affinity should apply even when smart routing is disabled")
	require.Equal(t, "provider-a", result.Service.Provider)
}

func TestAffinity_EmptySession(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", testSessionKey("session-1"), testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "") // empty session

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when session is empty")
}

// Pins are partition-scoped: a pin created in the top-level partition must
// not be honored for a request that matched a smart subset (and vice versa) —
// content routing decides the partition, affinity sticks within it.
func TestAffinity_PartitionScoping(t *testing.T) {
	store := newMockAffinityStore()
	topSvc := testService("provider-top", "gpt-4", true)
	subSvc := testService("provider-sub", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	// Pin in the top-level partition (no smart match).
	store.Set("rule-1", AffinitySessionKey(testSessionKey("session-1"), -1), testAffinityEntry(topSvc))
	// Independent pin inside smart partition 1.
	store.Set("rule-1", AffinitySessionKey(testSessionKey("session-1"), 1), testAffinityEntry(subSvc))

	stage := NewAffinityStage(store)

	// A request routed by smart partition 1 must see the subset pin.
	ctx := testContext(rule, "session-1")
	ctx.MatchedSmartRuleIndex = 1
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-sub", result.Service.Provider)

	// A request with no smart match must see the top-level pin.
	ctx = testContext(rule, "session-1")
	result, handled = stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-top", result.Service.Provider)

	// A request routed by a DIFFERENT partition must find no pin at all.
	ctx = testContext(rule, "session-1")
	ctx.MatchedSmartRuleIndex = 2
	_, handled = stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "a pin must never leak across partitions")
}

// --- Tier-scoped affinity (breaker-aware) ---

// When the primary tier (t0) is healthy, a stale pin to a lower tier (t1) must
// be declined so the strategy re-selects the primary. This is the fix for
// "configured t1 but long-term auto-jumps to t2".
func TestAffinity_TierScope_DeclinesStalePinWhenPrimaryHealthy(t *testing.T) {
	store := newMockAffinityStore()
	t0 := tierService("aff-tier-a-p0", "m", 0)
	t1 := tierService("aff-tier-a-p1", "m", 1)
	store.Set("rule-tier-a", testSessionKey("s1"), testAffinityEntry(t1))

	rule := tierRule("rule-tier-a", "m", []*loadbalance.Service{t0, t1})
	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled,
		"stale pin to a lower tier must be declined while the primary tier is healthy")
}

// While the primary tier (t0) is down (breaker open), the pin to the now-top
// available tier (t1) must be honored — within-failover stickiness.
func TestAffinity_TierScope_HonorsPinWhilePrimaryDown(t *testing.T) {
	store := newMockAffinityStore()
	t0 := tierService("aff-tier-b-p0", "m", 0)
	t1 := tierService("aff-tier-b-p1", "m", 1)
	store.Set("rule-tier-b", testSessionKey("s1"), testAffinityEntry(t1))

	rule := tierRule("rule-tier-b", "m", []*loadbalance.Service{t0, t1})

	bs := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		bs.RecordFailure("rule-tier-b", t0.ServiceID())
	}
	defer bs.RecordSuccess("rule-tier-b", t0.ServiceID())

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "pin must be honored while the primary tier breaker is open")
	require.Equal(t, t1.ServiceID(), result.Service.ServiceID())
}

// When the pinned service's own breaker is open and a higher tier is available,
// the pin must be declined (subsumed by the top-available-tier check).
func TestAffinity_TierScope_DeclinesWhenPinnedBreakerOpen(t *testing.T) {
	store := newMockAffinityStore()
	t0 := tierService("aff-tier-c-p0", "m", 0)
	t1 := tierService("aff-tier-c-p1", "m", 1)
	store.Set("rule-tier-c", testSessionKey("s1"), testAffinityEntry(t1))

	rule := tierRule("rule-tier-c", "m", []*loadbalance.Service{t0, t1})

	bs := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		bs.RecordFailure("rule-tier-c", t1.ServiceID())
	}
	defer bs.RecordSuccess("rule-tier-c", t1.ServiceID())

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled,
		"pin to an open lower-tier service must be declined when a higher tier is available")
}

// Two services sharing one tier: the pin is honored regardless of which one,
// preserving within-tier stickiness.
func TestAffinity_TierScope_WithinTierStickinessPreserved(t *testing.T) {
	store := newMockAffinityStore()
	a := tierService("aff-tier-d-pa", "m", 0)
	b := tierService("aff-tier-d-pb", "m", 0)
	store.Set("rule-tier-d", testSessionKey("s1"), testAffinityEntry(b))

	rule := tierRule("rule-tier-d", "m", []*loadbalance.Service{a, b})
	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "within-tier pin must be honored")
	require.Equal(t, b.ServiceID(), result.Service.ServiceID())
}

// Shape 2: one layer, many services, NO tier tactic label (plain horizontal
// rule). A pin to a service whose breaker is open must still be dropped when a
// healthy peer exists — the scoping is config-shape driven, not gated on the
// tactic label.
func TestAffinity_HorizontalRule_DropsPinToDeadPeer(t *testing.T) {
	store := newMockAffinityStore()
	a := testService("aff-horiz-a", "m", true) // tier 0 (default)
	b := testService("aff-horiz-b", "m", true) // tier 0 (default)
	store.Set("rule-horiz", testSessionKey("s1"), testAffinityEntry(a))

	// Plain rule: testRule leaves LBTactic unset (defaults to random).
	rule := testRule("rule-horiz", "m", []*loadbalance.Service{a, b})
	rule.Flags.SessionAffinity = 3600

	bs := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		bs.RecordFailure("rule-horiz", a.ServiceID())
	}
	defer bs.RecordSuccess("rule-horiz", a.ServiceID())

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled,
		"pin to a dead same-tier peer must be dropped even without a tier tactic label")
}

func TestAffinity_Name(t *testing.T) {
	stage := NewAffinityStage(newMockAffinityStore())
	require.Equal(t, "affinity", stage.Name())
}

func TestAffinity_MatchedSmartRuleIndex_Propagated(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	// The pin lives in partition 2's bucket, matching the request's route.
	store.Set("rule-1", AffinitySessionKey(testSessionKey("session-1"), 2), testAffinityEntry(svc))

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "session-1")
	ctx.MatchedSmartRuleIndex = 2

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, 2, result.MatchedSmartRuleIndex)
}

func TestAffinity_StoreInterface(t *testing.T) {
	// Verify mockAffinityStore satisfies AffinityStore interface at compile time
	var _ AffinityStore = newMockAffinityStore()

	// Verify the real AffinityStore methods work with routing.AffinityEntry
	store := newMockAffinityStore()
	svc := &loadbalance.Service{Provider: "p1", Model: "m1", Weight: 1, Active: true}
	entry := &AffinityEntry{Service: svc, LockedAt: time.Now(), ExpiresAt: time.Now().Add(2 * time.Hour)}

	store.Set("r1", "s1", entry)
	got, ok := store.Get("r1", "s1")
	require.True(t, ok)
	require.Equal(t, svc, got.Service)

	_, ok = store.Get("r1", "other")
	require.False(t, ok)
}

func TestAffinity_MultipleSessions(t *testing.T) {
	store := newMockAffinityStore()
	svcA := testService("provider-a", "gpt-4", true)
	svcB := testService("provider-b", "claude-3", true)
	store.Set("rule-1", testSessionKey("session-a"), testAffinityEntry(svcA))
	store.Set("rule-1", testSessionKey("session-b"), testAffinityEntry(svcB))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)

	// Session A should get provider A
	ctxA := testContext(rule, "session-a")
	resultA, handledA := stage.Evaluate(ctxA, newSelectionState(ctxA.Rule))
	require.True(t, handledA)
	require.Equal(t, "provider-a", resultA.Service.Provider)

	// Session B should get provider B
	ctxB := testContext(rule, "session-b")
	resultB, handledB := stage.Evaluate(ctxB, newSelectionState(ctxB.Rule))
	require.True(t, handledB)
	require.Equal(t, "provider-b", resultB.Service.Provider)
}

// --- Strict TTL tests ---

// TestAffinity_StrictTTL_Expired verifies that an expired affinity entry is
// dropped and the session passes through to the strategy for re-selection.
func TestAffinity_StrictTTL_Expired(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)

	// Create an already-expired entry
	entry := &AffinityEntry{
		Service:   svc,
		LockedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // expired 1 hour ago
	}
	store.Set("rule-ttl", testSessionKey("s1"), entry)

	rule := testRule("rule-ttl", "gpt-4", nil)
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "expired affinity entry must be dropped")
}

// TestAffinity_StrictTTL_NotExpired verifies that a valid (unexpired) affinity
// entry is honored normally.
func TestAffinity_StrictTTL_NotExpired(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)

	// Create a still-valid entry (expires in the future)
	entry := &AffinityEntry{
		Service:   svc,
		LockedAt:  time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour), // expires in 1 hour
	}
	store.Set("rule-ttl", testSessionKey("s1"), entry)

	rule := testRule("rule-ttl", "gpt-4", nil)
	rule.Flags.SessionAffinity = 3600

	stage := NewAffinityStage(store)
	ctx := testContext(rule, "s1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "valid affinity entry must be honored")
	require.Equal(t, "provider-a", result.Service.Provider)
}
