package routing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestAffinity_LockedSession(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", "session-1", testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "global")
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
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "global")
	ctx := testContext(rule, "session-1")

	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass to next stage when no lock")
	require.Nil(t, result)
}

func TestAffinity_AffinityDisabled(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", "session-1", testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = false // disabled

	stage := NewAffinityStage(store, "global")
	ctx := testContext(rule, "session-1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when affinity disabled")
}

func TestAffinity_SmartDisabled(t *testing.T) {
	store := newMockAffinityStore()

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = false

	stage := NewAffinityStage(store, "global")
	ctx := testContext(rule, "session-1")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when smart disabled")
}

func TestAffinity_EmptySession(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", "session-1", testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "global")
	ctx := testContext(rule, "") // empty session

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when session is empty")
}

func TestAffinity_SmartRuleScope_NoIndex(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", "session-1", testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "smart_rule")
	ctx := testContext(rule, "session-1")
	ctx.MatchedSmartRuleIndex = -1 // smart routing hasn't run yet

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when smart_rule scope but no index")
}

func TestAffinity_Name(t *testing.T) {
	stage := NewAffinityStage(newMockAffinityStore(), "global")
	require.Equal(t, "affinity", stage.Name())
}

func TestAffinity_MatchedSmartRuleIndex_Propagated(t *testing.T) {
	store := newMockAffinityStore()
	svc := testService("provider-a", "gpt-4", true)
	store.Set("rule-1", "session-1", testAffinityEntry(svc))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "smart_rule")
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
	store.Set("rule-1", "session-a", testAffinityEntry(svcA))
	store.Set("rule-1", "session-b", testAffinityEntry(svcB))

	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	stage := NewAffinityStage(store, "global")

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
