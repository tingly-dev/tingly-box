package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestShouldIncludeRuleInModelList_TransportReachabilityForOpenAI(t *testing.T) {
	if !shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioOpenCode) {
		t.Fatalf("expected exact scenario match to be included")
	}
	if !shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioCodex) {
		t.Fatalf("expected openai-transport scenario to include transport-reachable scenario")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioAnthropic) {
		t.Fatalf("expected openai-transport scenario to exclude anthropic-only scenario")
	}
}

func TestShouldIncludeRuleInModelList_TransportReachabilityForAnthropic(t *testing.T) {
	customScenario := typ.RuleScenario("general_test_transport_anthropic")
	err := typ.RegisterScenario(typ.ScenarioDescriptor{
		ID:                 customScenario,
		SupportedTransport: []typ.ScenarioTransport{typ.TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	})
	if err != nil {
		t.Fatalf("register custom scenario: %v", err)
	}

	if !shouldIncludeRuleInModelList(typ.ScenarioAnthropic, customScenario) {
		t.Fatalf("expected anthropic model list to include transport-reachable custom scenario")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioOpenAI, customScenario) {
		t.Fatalf("expected openai model list to exclude anthropic-only custom scenario")
	}
}

func TestShouldIncludeRuleInModelList_ClaudeCodeUsesAnthropicTransport(t *testing.T) {
	if !shouldIncludeRuleInModelList(typ.ScenarioClaudeCode, typ.ScenarioAnthropic) {
		t.Fatalf("expected claude_code model list to include anthropic-transport scenario")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioClaudeCode, typ.ScenarioOpenAI) {
		t.Fatalf("expected claude_code model list to exclude openai-only scenario")
	}
}

func TestShouldIncludeRuleInModelList_CustomScenarioWithBothTransports(t *testing.T) {
	customScenario := typ.RuleScenario("general_test_transport_both")
	err := typ.RegisterScenario(typ.ScenarioDescriptor{
		ID:                 customScenario,
		SupportedTransport: []typ.ScenarioTransport{typ.TransportOpenAI, typ.TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	})
	if err != nil {
		t.Fatalf("register custom scenario: %v", err)
	}

	if !shouldIncludeRuleInModelList(customScenario, typ.ScenarioOpenAI) {
		t.Fatalf("expected custom scenario to include openai transport scenario")
	}
	if !shouldIncludeRuleInModelList(customScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected custom scenario to include anthropic transport scenario")
	}
}

func TestShouldIncludeRuleInModelList_ProfileOnlyIncludesExactMatch(t *testing.T) {
	// Test that profile scenarios only include exact matches, not transport-reachable scenarios
	profiledScenario := typ.RuleScenario("claude_code:p1")
	baseScenario := typ.RuleScenario("claude_code")

	// Profile should include exact match
	if !shouldIncludeRuleInModelList(profiledScenario, profiledScenario) {
		t.Fatalf("expected profile to include exact match rule")
	}

	// Profile should NOT include base scenario rules (even though they share transport)
	if shouldIncludeRuleInModelList(profiledScenario, baseScenario) {
		t.Fatalf("expected profile to exclude base scenario rules")
	}

	// Profile should NOT include other scenarios with compatible transport
	if shouldIncludeRuleInModelList(profiledScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected profile to exclude other anthropic transport scenarios")
	}

	// Base scenario should still include transport-reachable scenarios
	if !shouldIncludeRuleInModelList(baseScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected base scenario to include transport-reachable scenarios")
	}
}

func TestShouldIncludeRuleInModelList_BaseScenarioExcludesProfileRules(t *testing.T) {
	baseScenario := typ.RuleScenario("claude_code")
	profiledScenario := typ.RuleScenario("claude_code:p1")

	// Base scenario should NOT include profile rules (profiles are exclusively scoped)
	if shouldIncludeRuleInModelList(baseScenario, profiledScenario) {
		t.Fatalf("expected base scenario to exclude profiled scenario rules")
	}

	// Base scenario should still include its own rules
	if !shouldIncludeRuleInModelList(baseScenario, baseScenario) {
		t.Fatalf("expected base scenario to include its own rules")
	}

	// Base scenario should still include transport-reachable non-profile scenarios
	if !shouldIncludeRuleInModelList(baseScenario, typ.ScenarioAnthropic) {
		t.Fatalf("expected base scenario to include transport-reachable scenarios")
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
