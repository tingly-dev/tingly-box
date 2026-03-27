package typ

import "testing"

func TestRegisterScenario_AllowsRuleBindingWithoutPathUsage(t *testing.T) {
	scenario := RuleScenario("test_shared_registry")

	if err := RegisterScenario(ScenarioDescriptor{
		ID:                 scenario,
		SupportedTransport: []ScenarioTransport{TransportOpenAI, TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	}); err != nil {
		t.Fatalf("RegisterScenario() error = %v", err)
	}

	if !CanBindRulesToScenario(scenario) {
		t.Fatalf("expected %q to allow rule binding", scenario)
	}
	if CanUseScenarioInPath(scenario) {
		t.Fatalf("expected %q to reject direct path use", scenario)
	}
}

func TestRegisterScenario_IsIdempotentForSameDescriptor(t *testing.T) {
	scenario := RuleScenario("test_registry_idempotent")
	descriptor := ScenarioDescriptor{
		ID:                 scenario,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	}

	if err := RegisterScenario(descriptor); err != nil {
		t.Fatalf("RegisterScenario() first call error = %v", err)
	}
	if err := RegisterScenario(descriptor); err != nil {
		t.Fatalf("RegisterScenario() second call error = %v", err)
	}
}

func TestRegisterScenario_RejectsConflictingDescriptor(t *testing.T) {
	scenario := RuleScenario("test_registry_conflict")

	if err := RegisterScenario(ScenarioDescriptor{
		ID:                 scenario,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	}); err != nil {
		t.Fatalf("RegisterScenario() first call error = %v", err)
	}

	if err := RegisterScenario(ScenarioDescriptor{
		ID:                 scenario,
		SupportedTransport: []ScenarioTransport{TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	}); err == nil {
		t.Fatalf("expected conflicting descriptor registration to fail")
	}
}
