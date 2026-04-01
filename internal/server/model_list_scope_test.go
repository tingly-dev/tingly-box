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
