package servertest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/routing/routingtest"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestRepro_BasePoolEliminatedBySmartUnionHealth reproduces the "rule has no
// available service" failure for a rule with a single base service plus a
// smart-routing partition naming a different service.
//
// The pipeline seeds candidates with base ∪ partition services. HealthStage's
// degrade guard ("keep the full set when everything is unhealthy") is
// evaluated on that union, so when only the base service is unhealthy (e.g.
// repeated 429s marked it rate-limited) the guard does not fire and the base
// service is filtered out. A request that does not match the partition then
// narrows back to basePool = candidates ∩ rule.Services = ∅, and the terminal
// LoadBalancer stage fails the whole rule with "no services configured" even
// though the rule's one service is configured correctly and may already have
// recovered.
//
// Post-fix, the pipeline driver restores the rule's active services whenever
// a narrowing comes back empty, so the request reaches the upstream and
// surfaces the real upstream error.
func TestRepro_BasePoolEliminatedBySmartUnionHealth(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	defer loadbalance.DefaultBreakerStore().Reset()

	cfg := newTestGlobalConfig(t)
	baseProvider := addTestProvider(t, cfg, "base-provider")
	partitionProvider := addTestProvider(t, cfg, "partition-provider")
	healthMonitor, _, selector := newSelectorStack(cfg)

	baseSvc := routingtest.Service(baseProvider, "main-model", true)
	partitionSvc := routingtest.Service(partitionProvider, "background-model", true)
	rule := &typ.Rule{
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: "main-model",
		UUID:         "single-service-with-partition",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
		},
		Services:     []*loadbalance.Service{baseSvc},
		SmartEnabled: true,
		SmartRouting: []smartrouting.SmartRouting{{
			Description: "route huge-context requests elsewhere",
			Ops: []smartrouting.SmartOp{{
				Position:  smartrouting.PositionToken,
				Operation: smartrouting.OpTokenGe,
				Value:     "500000",
			}},
			Services: []*loadbalance.Service{partitionSvc},
		}},
		Active: true,
	}

	// ctx.Request == nil -> smart routing cannot match, so the pipeline narrows
	// back to the base pool (same as any request the partition doesn't match).
	ctx := &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}

	// Sanity: while healthy, the base service is selected.
	res, err := selector.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "main-model", res.Service.Model)

	// The base service repeatedly fails with 429 -> marked unhealthy. The
	// partition service stays healthy, so HealthStage's union-level degrade
	// guard does not fire and the base service is eliminated from candidates.
	for i := 0; i < 5; i++ {
		healthMonitor.ReportRateLimit(baseSvc.ServiceID())
	}
	require.False(t, healthMonitor.IsHealthy(baseSvc.ServiceID()))

	ctx = &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}
	res, err = selector.Select(ctx)
	require.NoError(t, err,
		"an unhealthy single base service must degrade to itself, not fail the rule")
	require.NotNil(t, res)
	require.Equal(t, "main-model", res.Service.Model,
		"non-matching requests must stay in the base pool")
}
