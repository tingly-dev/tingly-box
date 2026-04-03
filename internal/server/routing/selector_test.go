package routing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSelect_NoAffinity_FallsToLoadBalancer(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	services := []*loadbalance.Service{svc}
	rule := testRule("rule-1", "gpt-4", services)

	sel := NewServiceSelector(cfg, store, lb)
	result, err := sel.Select(testContext(rule, ""))
	require.NoError(t, err)
	require.Equal(t, "load_balancer", result.Source)
	require.Equal(t, "provider-a", result.Service.Provider)
}

func TestSelect_GlobalAffinity_Hit(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	store.Set("rule-1", "session-1", testAffinityEntry(svc))
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	services := []*loadbalance.Service{svc}
	rule := testRule("rule-1", "gpt-4", services)
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	result, err := sel.Select(testContext(rule, "session-1"))
	require.NoError(t, err)
	require.Equal(t, "affinity", result.Source)
	require.Equal(t, "gpt-4", result.Service.Model)
}

func TestSelect_GlobalAffinity_Miss_SmartHit(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore() // no locked session
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	services := []*loadbalance.Service{svc}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "session-1")
	ctx.Request = testOpenAIRequest("gpt-4o")

	result, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "smart_routing", result.Source)
}

func TestSelect_GlobalAffinity_Miss_SmartMiss(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore() // no locked session
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	// Smart routing won't match (rule looks for "claude", model is "gpt-4")
	services := []*loadbalance.Service{svc}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("claude"))

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "session-1")
	ctx.Request = testOpenAIRequest("gpt-4o")

	result, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "load_balancer", result.Source, "should fall through to LB when smart doesn't match")
}

func TestSelect_ValidatesActiveService(t *testing.T) {
	// Affinity returns an inactive service; pipeline should skip to LB
	inactiveSvc := testService("provider-old", "gpt-4", false)
	activeSvc := testService("provider-new", "gpt-4", true)
	lb := &mockLoadBalancer{service: activeSvc}
	store := newMockAffinityStore()
	store.Set("rule-1", "session-1", testAffinityEntry(inactiveSvc))
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-old": testProvider("provider-old", "Old", true),
			"provider-new": testProvider("provider-new", "New", true),
		},
	}

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{inactiveSvc, activeSvc})
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	result, err := sel.Select(testContext(rule, "session-1"))
	require.NoError(t, err)
	require.Equal(t, "load_balancer", result.Source, "inactive service should be skipped")
	require.Equal(t, "provider-new", result.Service.Provider)
}

func TestSelect_ValidatesProvider(t *testing.T) {
	// Service is active but provider is disabled; pipeline should skip to LB
	disabledSvc := testService("provider-disabled", "gpt-4", true)
	activeSvc := testService("provider-ok", "gpt-4", true)
	lb := &mockLoadBalancer{service: activeSvc}
	store := newMockAffinityStore()
	store.Set("rule-1", "session-1", testAffinityEntry(disabledSvc))
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-disabled": testProvider("provider-disabled", "Disabled", false),
			"provider-ok":       testProvider("provider-ok", "OK", true),
		},
	}

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{disabledSvc, activeSvc})
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	result, err := sel.Select(testContext(rule, "session-1"))
	require.NoError(t, err)
	require.Equal(t, "load_balancer", result.Source, "disabled provider should be skipped")
}

func TestSelect_PostProcess_LocksAffinity(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	services := []*loadbalance.Service{svc}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "session-1")
	ctx.Request = testOpenAIRequest("gpt-4o")

	result, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "smart_routing", result.Source)
	require.Len(t, store.sets, 1, "affinity should be locked after smart routing")
	require.Equal(t, "rule-1", store.sets[0].ruleUUID)
	require.Equal(t, "session-1", store.sets[0].sessionID)
}

func TestSelect_PostProcess_NoLockOnAffinitySource(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	store.Set("rule-1", "session-1", testAffinityEntry(svc))
	require.Len(t, store.sets, 1, "setup set count baseline")
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})
	rule.SmartEnabled = true
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	_, err := sel.Select(testContext(rule, "session-1"))
	require.NoError(t, err)
	require.Len(t, store.sets, 1, "should NOT lock again when source is affinity (only setup set)")
}

func TestSelect_PostProcess_LocksOnLoadBalancer(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	// SmartAffinity=true but smart routing won't match → falls to LB
	services := []*loadbalance.Service{svc}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("claude"))
	rule.SmartAffinity = true

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "session-1")
	ctx.Request = testOpenAIRequest("gpt-4o")

	result, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "load_balancer", result.Source)
	require.Len(t, store.sets, 1, "affinity should be locked even when source is load_balancer")
}

func TestSelect_PostProcess_NoLockWithoutAffinity(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	services := []*loadbalance.Service{svc}
	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	// SmartAffinity=false (default from testSmartRule which doesn't set it)

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "session-1")
	ctx.Request = testOpenAIRequest("gpt-4o")

	_, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Len(t, store.sets, 0, "should NOT lock when SmartAffinity is false")
}

func TestSelect_NoServiceAvailable(t *testing.T) {
	lb := &mockLoadBalancer{err: ErrNoService}
	store := newMockAffinityStore()
	cfg := &mockConfig{}

	rule := testRule("rule-1", "gpt-4", nil)

	sel := NewServiceSelector(cfg, store, lb)
	_, err := sel.Select(testContext(rule, ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no service available")
}

func TestSelect_PipelineCaching(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})

	sel := NewServiceSelector(cfg, store, lb)
	ctx := testContext(rule, "")

	// Call twice — should use cached pipelines without panic
	for i := 0; i < 3; i++ {
		result, err := sel.Select(ctx)
		require.NoError(t, err, "call %d failed", i)
		require.Equal(t, "load_balancer", result.Source, "call %d", i)
	}
}

func TestUpdateServiceIndex(t *testing.T) {
	lb := &mockLoadBalancer{}
	store := newMockAffinityStore()

	svc := testService("provider-a", "gpt-4", true)
	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})

	sel := NewServiceSelector(&mockConfig{}, store, lb)
	err := sel.UpdateServiceIndex(rule, svc)
	require.NoError(t, err)
	require.True(t, lb.updateIndexCalled, "UpdateServiceIndex should call LB")
}

func TestNewServiceSelector_SmartAffinityPipelineOrder(t *testing.T) {
	lb := &mockLoadBalancer{}
	store := newMockAffinityStore()
	sel := NewServiceSelector(&mockConfig{}, store, lb)

	pipeline := sel.pipelines[pipelineModeSmartAffinity]
	require.Len(t, pipeline, 4)
	require.Equal(t, "health", pipeline[0].Name())
	require.Equal(t, "affinity", pipeline[1].Name())
	require.Equal(t, "smart_routing", pipeline[2].Name())
	require.Equal(t, "load_balancer", pipeline[3].Name())
}

// ErrNoService is a sentinel error for tests
var ErrNoService = errors.New("no service available")
