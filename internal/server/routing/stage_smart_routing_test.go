package routing

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// betaRequestWithToolResult builds a BetaMessageNewParams that simulates a
// typical agentic turn: user text → assistant tool_use → user tool_result.
// The keyword appears only in the original user text, not in the tool_result.
func betaRequestWithToolResult(keyword string) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-20241022"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "please search for " + keyword}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "I will search now"}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: "toolu_01",
						Content: []anthropic.BetaToolResultBlockParamContentUnion{
							{OfText: &anthropic.BetaTextBlockParam{Text: "search results"}},
						},
					}},
				},
			},
		},
	}
}

func TestSmartRouting_RuleMatch(t *testing.T) {
	lb := &mockLoadBalancer{service: testService("provider-a", "gpt-4", true)}
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
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

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when rule doesn't match")
}

func TestSmartRouting_Disabled(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	// SmartEnabled is false by default

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
	ctx := testContext(rule, "")
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled)
}

func TestSmartRouting_EmptyRules(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{} // empty

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4")

	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled)
}

func TestSmartRouting_NilRequest(t *testing.T) {
	lb := &mockLoadBalancer{}
	rule := testRule("rule-1", "gpt-4", nil)
	rule.SmartEnabled = true

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
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

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "should pass when matched service is inactive")
}

func TestSmartRouting_SingleService(t *testing.T) {
	lb := &mockLoadBalancer{} // should NOT be called for single service
	services := []*loadbalance.Service{testService("provider-a", "gpt-4", true)}

	rule := testSmartRule("rule-1", "gpt-4", services, testModelContainsOp("gpt"))
	ctx := testContext(rule, "")
	ctx.Request = testOpenAIRequest("gpt-4o")

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
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

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
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

	stage := NewSmartRoutingStage(lb, newMockAffinityStore())
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled)
	require.Equal(t, "provider-b", result.Service.Provider)
	require.Equal(t, 1, result.MatchedSmartRuleIndex, "second rule should match")
}

func TestSmartRouting_Name(t *testing.T) {
	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	require.Equal(t, "smart_routing", stage.Name())
}

func TestSmartRouting_AgentClaudeCode_SubagentRoutesToCheapPool(t *testing.T) {
	cheapSvc := testService("provider-cheap", "haiku", true)
	mainSvc := testService("provider-main", "sonnet", true)
	allServices := []*loadbalance.Service{cheapSvc, mainSvc}

	rule := testRule("rule-cc", "claude-3-5-sonnet", allServices)
	rule.Scenario = typ.ScenarioClaudeCode
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{
			Description: "subagent → cheap pool",
			Ops: []smartrouting.SmartOp{
				{
					Position:  smartrouting.PositionAgentClaudeCode,
					Operation: smartrouting.OpAgentClaudeCodeEquals,
					Value:     smartrouting.ClaudeCodeKindSubagent,
				},
			},
			Services: []*loadbalance.Service{cheapSvc},
		},
	}

	subagentReq := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 100,
		System: []anthropic.TextBlockParam{
			{Text: "You are an agent investigating the user's question. Report back when done."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("find callers of foo")),
		},
	}

	ctx := testContext(rule, "")
	ctx.Scenario = typ.ScenarioClaudeCode
	ctx.Request = subagentReq

	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "subagent request should match")
	require.Equal(t, "provider-cheap", result.Service.Provider)
}

func TestSmartRouting_AgentClaudeCode_MainDoesNotMatchSubagentRule(t *testing.T) {
	cheapSvc := testService("provider-cheap", "haiku", true)

	rule := testRule("rule-cc", "claude-3-5-sonnet", []*loadbalance.Service{cheapSvc})
	rule.Scenario = typ.ScenarioClaudeCode
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{
			Description: "subagent → cheap pool",
			Ops: []smartrouting.SmartOp{
				{
					Position:  smartrouting.PositionAgentClaudeCode,
					Operation: smartrouting.OpAgentClaudeCodeEquals,
					Value:     smartrouting.ClaudeCodeKindSubagent,
				},
			},
			Services: []*loadbalance.Service{cheapSvc},
		},
	}

	mainReq := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 100,
		System: []anthropic.TextBlockParam{
			{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
	}

	ctx := testContext(rule, "")
	ctx.Scenario = typ.ScenarioClaudeCode
	ctx.Request = mainReq

	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "main-agent request should not match subagent-only rule")
}

// TestSmartRouting_LatestUser_ToolResultDoesNotLockBranch is the stage-level
// regression test for the "lock-in" bug: after a user sends a tool_result
// (role=user but no text), the latest_user contains rule must NOT match against
// the stale previous user text and must fall through to the load-balancer
// (default) instead of staying locked on the smart-routing branch.
func TestSmartRouting_LatestUser_ToolResultDoesNotLockBranch(t *testing.T) {
	specialSvc := testService("provider-special", "model-special", true)

	rule := testSmartRule("rule-lu", "claude-3-5-sonnet-20241022",
		[]*loadbalance.Service{specialSvc},
		smartrouting.SmartOp{
			Position:  smartrouting.PositionLatestUser,
			Operation: smartrouting.OpLatestUserContains,
			Value:     "keyword",
		},
	)

	// Latest message is a tool_result — must fall through, not lock on branch.
	ctx := testContext(rule, "")
	ctx.Request = betaRequestWithToolResult("keyword")

	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled,
		"tool_result as last user message must not match latest_user contains rule")
}

// TestSmartRouting_LatestUser_TextMatchesAfterToolResult verifies that when a
// real user text message follows the tool_result exchange and contains the
// keyword, the rule correctly fires again.
func TestSmartRouting_LatestUser_TextMatchesAfterToolResult(t *testing.T) {
	specialSvc := testService("provider-special", "model-special", true)

	rule := testSmartRule("rule-lu", "claude-3-5-sonnet-20241022",
		[]*loadbalance.Service{specialSvc},
		smartrouting.SmartOp{
			Position:  smartrouting.PositionLatestUser,
			Operation: smartrouting.OpLatestUserContains,
			Value:     "keyword",
		},
	)

	// Four-turn conversation ending with a real user text containing the keyword.
	req := &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-20241022"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "initial message"}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "response"}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "now search for keyword please"}},
				},
			},
		},
	}

	ctx := testContext(rule, "")
	ctx.Request = req

	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	result, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.True(t, handled, "real user text with keyword must match")
	require.Equal(t, "provider-special", result.Service.Provider)
}

func TestSmartRouting_AgentClaudeCode_NonClaudeCodeScenarioDoesNotMatch(t *testing.T) {
	// Even with subagent-shaped body, a non-claude_code scenario must NOT trigger
	// the agent.claude_code op (no detection runs, kind stays empty).
	svc := testService("provider", "gpt-4", true)

	rule := testRule("rule-x", "gpt-4", []*loadbalance.Service{svc})
	rule.Scenario = typ.ScenarioOpenAI
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{
			Description: "subagent → cheap",
			Ops: []smartrouting.SmartOp{
				{
					Position:  smartrouting.PositionAgentClaudeCode,
					Operation: smartrouting.OpAgentClaudeCodeEquals,
					Value:     smartrouting.ClaudeCodeKindSubagent,
				},
			},
			Services: []*loadbalance.Service{svc},
		},
	}

	ctx := testContext(rule, "")
	ctx.Scenario = typ.ScenarioOpenAI
	ctx.Request = testOpenAIRequest("gpt-4")

	stage := NewSmartRoutingStage(&mockLoadBalancer{}, newMockAffinityStore())
	_, handled := stage.Evaluate(ctx, newSelectionState(ctx.Rule))
	require.False(t, handled, "non-claude_code scenarios should not satisfy agent.claude_code ops")
}
