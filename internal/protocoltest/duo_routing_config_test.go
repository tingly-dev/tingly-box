package protocoltest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRoutingPipelineSmartBeforeLoadBalancerRuleShape(t *testing.T) {
	sc := routingPipelineSmartBeforeLoadBalancer()
	rule, err := sc.toRule()
	require.NoError(t, err)
	require.Equal(t, loadbalance.TacticTier, rule.GetTacticType())

	params, ok := rule.LBTactic.Params.(*typ.TierParams)
	require.True(t, ok)
	require.Equal(t, loadbalance.TacticRandom, params.WithinTierTactic)
	require.Equal(t, 0, rule.Services[0].Tier)
	require.Equal(t, 1, rule.SmartRouting[0].Services[0].Tier)
	require.Equal(t, 2, rule.SmartRouting[0].Services[1].Tier)
}

func TestRoutingRuleRejectsInvalidTactics(t *testing.T) {
	sc := routingPipelineSmartBeforeLoadBalancer()
	sc.Rule.LBTactic = "typo"
	_, err := sc.toRule()
	require.ErrorContains(t, err, "unknown lb_tactic")

	sc = routingPipelineSmartBeforeLoadBalancer()
	sc.Rule.LBTactic = "random"
	sc.Rule.WithinTierTactic = "random"
	_, err = sc.toRule()
	require.ErrorContains(t, err, "requires lb_tactic tier")
}
