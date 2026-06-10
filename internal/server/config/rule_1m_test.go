package config

import (
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMatchRule_Context1MNormalized(t *testing.T) {
	c := &Config{
		Rules: []typ.Rule{
			// Desktop rule renamed by the 1M toggle.
			{UUID: "d1", Scenario: typ.ScenarioClaudeDesktop, RequestModel: "claude-sonnet-4-6[1m]"},
			// Bare CC profile rule; client env may advertise "haiku[1m]".
			{UUID: "p1", Scenario: typ.RuleScenario("claude_code:p1"), RequestModel: "haiku"},
			// Non-Claude scenario must NOT be normalized.
			{UUID: "o1", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt-4o"},
		},
	}

	// Stale Desktop config sends the bare name → matches the renamed rule.
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("bare name should match [1m]-renamed desktop rule, got %+v", r)
	}
	// Suffixed pick from /v1/models matches exactly (fast path).
	if r := c.MatchRuleByModelAndScenario("claude-sonnet-4-6[1m]", typ.ScenarioClaudeDesktop); r == nil || r.UUID != "d1" {
		t.Errorf("suffixed name should match renamed desktop rule, got %+v", r)
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

func TestUpdateRule_DesktopSyncsContext1MName(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{UUID: "d1", Scenario: typ.ScenarioClaudeDesktop, RequestModel: "claude-sonnet-4-6", Active: true},
			{UUID: "cc1", Scenario: typ.ScenarioClaudeCode, RequestModel: "tingly/cc-haiku", Active: true},
		},
	}

	// Enabling the flag renames the desktop rule so /v1/models lists [1m].
	r := c.Rules[0]
	r.Flags.Context1M = true
	if err := c.UpdateRule("d1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[0].RequestModel; got != "claude-sonnet-4-6[1m]" {
		t.Errorf("desktop rule should be renamed with [1m], got %q", got)
	}

	// Disabling the flag strips the suffix again.
	r = c.Rules[0]
	r.Flags.Context1M = false
	if err := c.UpdateRule("d1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[0].RequestModel; got != "claude-sonnet-4-6" {
		t.Errorf("desktop rule should be stripped back, got %q", got)
	}

	// Claude Code rules are not renamed — their [1m] travels via the env.
	r = c.Rules[1]
	r.Flags.Context1M = true
	if err := c.UpdateRule("cc1", r); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}
	if got := c.Rules[1].RequestModel; got != "tingly/cc-haiku" {
		t.Errorf("claude_code rule must keep its bare name, got %q", got)
	}
}
