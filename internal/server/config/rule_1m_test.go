package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMatchRule_Context1MNormalized(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			// Desktop rule carrying the [1m] advertisement in its name.
			{UUID: "d1", Scenario: typ.ScenarioClaudeDesktop, RequestModel: "claude-sonnet-4-6[1m]"},
			// Bare CC profile rule; client env may advertise "haiku[1m]".
			{UUID: "p1", Scenario: typ.RuleScenario("claude_code:p1"), RequestModel: "haiku"},
			// Non-Claude scenario must NOT be normalized.
			{UUID: "o1", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt-4o"},
		},
	}

	// Stale Desktop config sends the bare name → matches the renamed rule.
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("bare name should match [1m]-named desktop rule, got %+v", r)
	}
	// Suffixed pick from /v1/models matches exactly (fast path).
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6[1m]", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("suffixed name should match [1m]-named desktop rule, got %+v", r)
	}
	// Suffixed request against a bare profile rule (profiled scenario → base
	// scenario is claude_code, so normalization applies).
	if r := c.MatchRuleByModelAndScenario("haiku[1m]", typ.RuleScenario("claude_code:p1")); r == nil || r.UUID != "p1" {
		t.Errorf("suffixed request should match bare profile rule, got %+v", r)
	}
	// Non-Claude scenarios keep strict matching.
	if r := c.MatchRuleByModelAndScenario("gpt-4o[1m]", typ.ScenarioOpenAI); r != nil {
		t.Errorf("openai scenario must not 1m-normalize, got %+v", r)
	}
}
