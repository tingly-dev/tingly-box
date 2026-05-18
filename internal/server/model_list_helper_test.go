package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestShouldIncludeRuleInModelList_OnlyExactScenarioMatch(t *testing.T) {
	if !shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioOpenCode) {
		t.Fatalf("expected exact scenario match to be included")
	}
	// Scenarios are isolated: sharing a transport must not grant visibility.
	if shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioCodex) {
		t.Fatalf("expected transport-reachable scenario to be excluded")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioAnthropic) {
		t.Fatalf("expected unrelated scenario to be excluded")
	}
}

func TestShouldIncludeRuleInModelList_ClaudeCodeExcludesOtherScenarios(t *testing.T) {
	if !shouldIncludeRuleInModelList(typ.ScenarioClaudeCode, typ.ScenarioClaudeCode) {
		t.Fatalf("expected claude_code to include its own rules")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioClaudeCode, typ.ScenarioAnthropic) {
		t.Fatalf("expected claude_code model list to exclude anthropic scenario")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioClaudeCode, typ.ScenarioOpenAI) {
		t.Fatalf("expected claude_code model list to exclude openai scenario")
	}
}

func TestShouldIncludeRuleInModelList_ProfileOnlyIncludesExactMatch(t *testing.T) {
	profiledScenario := typ.RuleScenario("claude_code:p1")
	baseScenario := typ.RuleScenario("claude_code")

	// Profile should include exact match
	if !shouldIncludeRuleInModelList(profiledScenario, profiledScenario) {
		t.Fatalf("expected profile to include exact match rule")
	}

	// Profile should NOT include base scenario rules
	if shouldIncludeRuleInModelList(profiledScenario, baseScenario) {
		t.Fatalf("expected profile to exclude base scenario rules")
	}

	// Profile should NOT include other scenarios
	if shouldIncludeRuleInModelList(profiledScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected profile to exclude other scenarios")
	}

	// Base scenario should NOT include other scenarios either
	if shouldIncludeRuleInModelList(baseScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected base scenario to exclude unrelated scenarios")
	}
}

func TestShouldIncludeRuleInModelList_BaseScenarioExcludesProfileRules(t *testing.T) {
	baseScenario := typ.RuleScenario("claude_code")
	profiledScenario := typ.RuleScenario("claude_code:p1")

	// Base scenario should NOT include profile rules
	if shouldIncludeRuleInModelList(baseScenario, profiledScenario) {
		t.Fatalf("expected base scenario to exclude profiled scenario rules")
	}

	// Base scenario should still include its own rules
	if !shouldIncludeRuleInModelList(baseScenario, baseScenario) {
		t.Fatalf("expected base scenario to include its own rules")
	}
}

func TestShouldIncludeRuleInModelList_DifferentProfilesAreIsolated(t *testing.T) {
	profile1 := typ.RuleScenario("claude_code:p1")
	profile2 := typ.RuleScenario("claude_code:p2")

	// Profile 1 should not include Profile 2's rules
	if shouldIncludeRuleInModelList(profile1, profile2) {
		t.Fatalf("expected profile p1 to exclude profile p2 rules")
	}

	// Profile 2 should not include Profile 1's rules
	if shouldIncludeRuleInModelList(profile2, profile1) {
		t.Fatalf("expected profile p2 to exclude profile p1 rules")
	}

	// Each profile should only include its own rules
	if !shouldIncludeRuleInModelList(profile1, profile1) {
		t.Fatalf("expected profile p1 to include its own rules")
	}
	if !shouldIncludeRuleInModelList(profile2, profile2) {
		t.Fatalf("expected profile p2 to include its own rules")
	}
}
