package servertest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/routing/routingtest"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestRepro_SingleActiveServiceWithInactiveSibling reproduces "no active
// services for rule ..." on a plain rule with NO smart routing: one active
// service plus one inactive (disabled) service row.
//
// initialCandidateServices seeded inactive services into the pipeline's
// candidate set, and HealthFilter.Filter ignores Active. So when the only
// active service was marked unhealthy (repeated 429/auth failures), the
// inactive sibling still counted as a "healthy" candidate, HealthStage's
// all-unhealthy degrade guard did not fire, and the active service was
// eliminated. The terminal LoadBalancer stage then saw only the inactive
// service and failed the whole rule — instead of falling back to the
// configured active service and surfacing the real upstream error.
func TestRepro_SingleActiveServiceWithInactiveSibling(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	defer loadbalance.DefaultBreakerStore().Reset()

	cfg := newTestGlobalConfig(t)
	providerUUID := addTestProvider(t, cfg, "the-provider")
	healthMonitor, _, selector := newSelectorStack(cfg)

	activeSvc := routingtest.Service(providerUUID, "main-model", true)
	inactiveSvc := routingtest.Service(providerUUID, "old-model", false) // disabled by the user; must never be selected
	rule := &typ.Rule{
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: "main-model",
		UUID:         "one-active-one-inactive",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
		},
		Services: []*loadbalance.Service{activeSvc, inactiveSvc},
		Active:   true,
	}

	ctx := &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}

	// Sanity: while healthy, the active service is selected.
	res, err := selector.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "main-model", res.Service.Model)

	// The active service fails repeatedly -> marked unhealthy.
	for i := 0; i < 5; i++ {
		healthMonitor.ReportRateLimit(activeSvc.ServiceID())
	}
	require.False(t, healthMonitor.IsHealthy(activeSvc.ServiceID()))

	ctx = &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}
	res, err = selector.Select(ctx)
	require.NoError(t, err,
		"the rule must degrade to its active service, not error out")
	require.NotNil(t, res)
	require.Equal(t, "main-model", res.Service.Model,
		"the inactive service must never be selected")
}
