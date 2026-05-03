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

func TestEmbedScenarioDescriptor(t *testing.T) {
	d, ok := GetScenarioDescriptor(ScenarioEmbed)
	if !ok {
		t.Fatalf("expected %q descriptor to be registered", ScenarioEmbed)
	}
	if !d.AllowRuleBinding || !d.AllowDirectPathUse {
		t.Fatalf("expected embed descriptor to allow rule binding and path use, got %+v", d)
	}
	if !ScenarioSupportsTransport(ScenarioEmbed, TransportEmbed) {
		t.Fatalf("expected embed scenario to support TransportEmbed")
	}
	if ScenarioSupportsTransport(ScenarioEmbed, TransportOpenAI) {
		t.Fatalf("embed scenario must NOT support TransportOpenAI (chat must be rejected)")
	}
	if ScenarioSupportsTransport(ScenarioEmbed, TransportAnthropic) {
		t.Fatalf("embed scenario must NOT support TransportAnthropic")
	}
}

func TestOpenAIScenarioSupportsBothTransports(t *testing.T) {
	if !ScenarioSupportsTransport(ScenarioOpenAI, TransportOpenAI) {
		t.Fatalf("openai scenario should support TransportOpenAI")
	}
	if !ScenarioSupportsTransport(ScenarioOpenAI, TransportEmbed) {
		t.Fatalf("openai scenario should also support TransportEmbed (mixin extension)")
	}
}

func TestScenarioSupportsTransport_UnknownScenario(t *testing.T) {
	if ScenarioSupportsTransport(RuleScenario("does_not_exist"), TransportOpenAI) {
		t.Fatalf("unknown scenario must not report transport support")
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
