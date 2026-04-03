package routing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

func TestSmartRouting_RuleMatch(t *testing.T) {
	lb := &mockLoadBalancer{service: testService("provider-a", "gpt-4", true)}
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.NotNil(t, result)
	require.Equal(t, "gpt-4", result.Service.Model)
	require.Equal(t, "smart_routing", result.Source)
	require.Equal(t, 0, result.MatchedSmartRuleIndex)
}

func TestSmartRouting_NoMatch(t *testing.T) {
	lb := &mockLoadBalancer{}
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("claude"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when rule doesn't match")
}

func TestSmartRouting_Disabled(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	// SmartEnabled is false by default

	stage := NewSmartRoutingStage(lb)
	ctx := testContext(rule, "")
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled)
}

func TestSmartRouting_EmptyRules(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{} // empty

	stage := NewSmartRoutingStage(lb)
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled)
}

func TestSmartRouting_NilRequest(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true

	stage := NewSmartRoutingStage(lb)
	ctx := testContext(rule, "")
	ctx.Request = nil

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled)
}

func TestSmartRouting_InactiveServiceFiltered(t *testing.T) {
	lb := &mockLoadBalancer{}
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", false)} // inactive

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when matched service is inactive")
}

func TestSmartRouting_SingleService(t *testing.T) {
	lb := &mockLoadBalancer{} // should NOT be called for single service
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-a", result.Service.Provider)
}

func TestSmartRouting_MultipleServices_LB(t *testing.T) {
	lb := &mockLoadBalancer{service: testService("provider-b", "gpt-4", true)}
	services := []*loadbalance.Service{
		testService("provider-a", "gpt-4", true),
		testService("provider-b", "gpt-4", true),
	}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-b", result.Service.Provider, "should use LB-selected service")
}

func TestSmartRouting_MatchedRuleIndex(t *testing.T) {
	lb := &mockLoadBalancer{}

	// Rule 0: matches claude, Rule 1: matches gpt
	servicesA := []*loadbalance.Service{testService("provider-a", "claude-3", true)}
	servicesB := []*loadbalance.Service{testService("provider-b", "gpt-4", true)}

	rule := testRule("rule-1", "gpt-4", append(servicesA, servicesB...))
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{Description: "claude-rule", Ops: []smartrouting.SmartOp{testModelContainsOp("claude")}, Services: servicesA},
		{Description: "gpt-rule", Ops: []smartrouting.SmartOp{testModelContainsOp("gpt")}, Services: servicesB},
	}

	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb)
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-b", result.Service.Provider)
	require.Equal(t, 1, result.MatchedSmartRuleIndex, "second rule should match")
}

func TestSmartRouting_Name(t *testing.T) {
	stage := NewSmartRoutingStage(&mockLoadBalancer{})
	require.Equal(t, "smart_routing", stage.Name())
}
