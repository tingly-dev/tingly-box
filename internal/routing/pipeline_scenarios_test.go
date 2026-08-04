package routing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// pipelineScenarioLB makes the candidate set handed to the terminal load
// balancer observable. Returning the first candidate also creates deliberate
// conflicts with upstream stages: without the expected narrowing or affinity
// short-circuit, a different service wins.
type pipelineScenarioLB struct {
	health *typ.HealthFilter
	calls  int
	seen   []string
}

func (l *pipelineScenarioLB) HealthFilter() *typ.HealthFilter { return l.health }

func (l *pipelineScenarioLB) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	l.calls++
	l.seen = l.seen[:0]
	for _, svc := range rule.GetActiveServices() {
		l.seen = append(l.seen, svc.ServiceID())
	}
	if len(rule.GetActiveServices()) == 0 {
		return nil, nil
	}
	return rule.GetActiveServices()[0], nil
}

func pipelineScenarioConfig(services ...*loadbalance.Service) *mockConfig {
	providers := make(map[string]*typ.Provider, len(services))
	for _, svc := range services {
		providers[svc.Provider] = testProvider(svc.Provider, svc.Provider, true)
	}
	return &mockConfig{providers: providers}
}

func pipelineSmartRule(base []*loadbalance.Service, partition []*loadbalance.Service) *typ.Rule {
	rule := testRule("pipeline-rule", "request-model", base)
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{{
		Description: "matched partition",
		Ops:         []smartrouting.SmartOp{testModelContainsOp("smart")},
		Services:    partition,
	}}
	return rule
}

func TestServiceSelectorPipelineScenarios(t *testing.T) {
	t.Run("pipeline-health-before-smart-routing", func(t *testing.T) {
		base := testService("base", "base-model", true)
		unhealthy := testService("unhealthy", "smart-unhealthy", true)
		healthy := testService("healthy", "smart-healthy", true)
		rule := pipelineSmartRule([]*loadbalance.Service{base}, []*loadbalance.Service{unhealthy, healthy})

		monitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
		monitor.ReportRateLimit(unhealthy.ServiceID())
		lb := &pipelineScenarioLB{health: typ.NewHealthFilter(monitor)}
		sel := NewServiceSelector(pipelineScenarioConfig(base, unhealthy, healthy), newMockAffinityStore(), lb)
		ctx := testContext(rule, "")
		ctx.Request = testOpenAIRequest("smart-request")

		result, err := sel.Select(ctx)
		require.NoError(t, err)
		require.Equal(t, healthy.ServiceID(), result.Service.ServiceID())
		require.Equal(t, []string{healthy.ServiceID()}, lb.seen,
			"health must remove the unhealthy service before smart routing intersects its partition")
	})

	t.Run("pipeline-smart-routing-before-affinity", func(t *testing.T) {
		base := testService("base", "base-model", true)
		smart := testService("smart", "smart-model", true)
		rule := pipelineSmartRule([]*loadbalance.Service{base}, []*loadbalance.Service{smart})
		rule.Flags.SessionAffinity = 300
		store := newMockAffinityStore()
		store.Set(rule.UUID, AffinitySessionKey(testSessionKey("session"), -1), testAffinityEntry(base))
		lb := &pipelineScenarioLB{}
		sel := NewServiceSelector(pipelineScenarioConfig(base, smart), store, lb)
		ctx := testContext(rule, "session")
		ctx.Request = testOpenAIRequest("smart-request")

		result, err := sel.Select(ctx)
		require.NoError(t, err)
		require.Equal(t, smart.ServiceID(), result.Service.ServiceID(),
			"the global affinity pin must not capture a request that first enters a smart partition")
		require.Equal(t, 0, ctx.MatchedSmartRuleIndex)
	})

	t.Run("pipeline-affinity-before-load-balancer", func(t *testing.T) {
		first := testService("first", "smart-first", true)
		pinned := testService("pinned", "smart-pinned", true)
		rule := pipelineSmartRule(nil, []*loadbalance.Service{first, pinned})
		rule.Flags.SessionAffinity = 300
		store := newMockAffinityStore()
		store.Set(rule.UUID, AffinitySessionKey(testSessionKey("session"), 0), testAffinityEntry(pinned))
		lb := &pipelineScenarioLB{}
		sel := NewServiceSelector(pipelineScenarioConfig(first, pinned), store, lb)
		ctx := testContext(rule, "session")
		ctx.Request = testOpenAIRequest("smart-request")

		result, err := sel.Select(ctx)
		require.NoError(t, err)
		require.Equal(t, SourceAffinity, result.Source)
		require.Equal(t, pinned.ServiceID(), result.Service.ServiceID())
		require.Zero(t, lb.calls, "an eligible affinity hit must terminate before load balancing")
	})

	t.Run("pipeline-smart-routing-before-load-balancer", func(t *testing.T) {
		base := testService("base", "base-model", true)
		firstSmart := testService("smart-a", "smart-a-model", true)
		secondSmart := testService("smart-b", "smart-b-model", true)
		rule := pipelineSmartRule([]*loadbalance.Service{base}, []*loadbalance.Service{firstSmart, secondSmart})
		lb := &pipelineScenarioLB{}
		sel := NewServiceSelector(pipelineScenarioConfig(base, firstSmart, secondSmart), newMockAffinityStore(), lb)
		ctx := testContext(rule, "")
		ctx.Request = testOpenAIRequest("smart-request")

		result, err := sel.Select(ctx)
		require.NoError(t, err)
		require.Equal(t, firstSmart.ServiceID(), result.Service.ServiceID())
		require.Equal(t, []string{firstSmart.ServiceID(), secondSmart.ServiceID()}, lb.seen,
			"load balancing must receive the smart partition, not the global candidate pool")
	})
}
