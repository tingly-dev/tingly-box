package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestShouldIncludeRuleInModelList_StrictForCustomScenario(t *testing.T) {
	if !shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioOpenCode) {
		t.Fatalf("expected exact scenario match to be included")
	}
	if shouldIncludeRuleInModelList(typ.ScenarioOpenCode, typ.ScenarioCodex) {
		t.Fatalf("expected non-matching custom scenario to be excluded")
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
