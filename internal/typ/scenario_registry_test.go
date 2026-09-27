package typ

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
)

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

func TestDecisionScenarioSupportsOnlyDecisionTransport(t *testing.T) {
	if !ScenarioSupportsTransport(ScenarioDecision, TransportDecision) {
		t.Fatal("decision scenario should support decision transport")
	}
	for _, transport := range []ScenarioTransport{TransportOpenAI, TransportAnthropic, TransportEmbed, TransportImageGen} {
		if ScenarioSupportsTransport(ScenarioDecision, transport) {
			t.Errorf("decision scenario unexpectedly supports %s", transport)
		}
	}
}

func TestAPIStyleAllowedForScenario(t *testing.T) {
	// Decision is capability-gated, not style-gated: at the style level every
	// chat family may bind (what matters is the decision fork URL), which is
	// what lets users configure providers without caring about O vs A.
	for _, style := range []ai.APIStyle{ai.APIStyleOpenAI, ai.APIStyleAnthropic, ai.APIStyleGoogle, ai.APIStyleDecision} {
		if !APIStyleAllowedForScenario(ScenarioDecision, style) {
			t.Errorf("decision scenario must not style-reject %s providers", style)
		}
	}
	// Chat-family scenarios reject Jev-native providers at the style level:
	// their APIBase is the decisions endpoint, they cannot serve chat.
	for _, scenario := range []RuleScenario{ScenarioOpenAI, ScenarioAnthropic, ScenarioEmbed, ScenarioImageGen, ScenarioClaudeCode} {
		if APIStyleAllowedForScenario(scenario, ai.APIStyleDecision) {
			t.Errorf("%s scenario must not accept decision-style providers", scenario)
		}
	}
	// Chat-family providers bind to chat-family scenarios (conversion span),
	// but embed/imagegen stay openai-only.
	if !APIStyleAllowedForScenario(ScenarioOpenAI, ai.APIStyleAnthropic) {
		t.Error("openai scenario must accept anthropic-style providers (cross-style chat conversion)")
	}
	if APIStyleAllowedForScenario(ScenarioEmbed, ai.APIStyleAnthropic) {
		t.Error("embed scenario must not accept anthropic-style providers")
	}
	if !APIStyleAllowedForScenario(ScenarioImageGen, ai.APIStyleOpenAI) {
		t.Error("imagegen scenario must accept openai-style providers")
	}
}

func TestProviderSupportsScenario_DecisionCapability(t *testing.T) {
	fork := &ai.Provider{Name: "gpt-with-decision", APIStyle: ai.APIStyleOpenAI, APIBaseDecision: "https://example.com/api/v1"}
	if !ProviderSupportsScenario(fork, ScenarioDecision) {
		t.Error("an openai-style provider with a decision fork must serve the decision scenario")
	}
	native := &ai.Provider{Name: "jev", APIStyle: ai.APIStyleDecision}
	if !ProviderSupportsScenario(native, ScenarioDecision) {
		t.Error("a Jev-native provider must serve the decision scenario")
	}
	plain := &ai.Provider{Name: "gpt", APIStyle: ai.APIStyleOpenAI}
	if ProviderSupportsScenario(plain, ScenarioDecision) {
		t.Error("an openai-style provider without a decision fork must NOT serve the decision scenario")
	}
	// The fork does not make a provider decision-only: it stays a chat provider.
	if !ProviderSupportsScenario(fork, ScenarioOpenAI) {
		t.Error("a provider with a decision fork keeps serving its chat scenario")
	}
	if ProviderSupportsScenario(native, ScenarioOpenAI) {
		t.Error("a Jev-native provider cannot serve the chat scenario")
	}
	if !ScenarioRequiresDecisionCapability(ScenarioDecision) {
		t.Error("decision scenario must require decision capability")
	}
	if ScenarioRequiresDecisionCapability(ScenarioOpenAI) {
		t.Error("openai scenario must not require decision capability")
	}
}

func TestScenarioSupportsTransport_UnknownScenario(t *testing.T) {
	if ScenarioSupportsTransport(RuleScenario("does_not_exist"), TransportOpenAI) {
		t.Fatalf("unknown scenario must not report transport support")
	}
}

func TestRuleScenario_Base(t *testing.T) {
	tests := []struct {
		input RuleScenario
		want  RuleScenario
	}{
		{ScenarioClaudeCode, ScenarioClaudeCode},
		{"claude_code:p1", ScenarioClaudeCode},
		{"claude_code:profile-abc", ScenarioClaudeCode},
		{ScenarioOpenAI, ScenarioOpenAI},
		{"openai:p2", ScenarioOpenAI},
	}
	for _, tt := range tests {
		if got := tt.input.Base(); got != tt.want {
			t.Errorf("(%q).Base() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRuleScenario_Is(t *testing.T) {
	tests := []struct {
		scenario RuleScenario
		base     RuleScenario
		want     bool
	}{
		{ScenarioClaudeCode, ScenarioClaudeCode, true},
		{"claude_code:p1", ScenarioClaudeCode, true},
		{"claude_code:profile-abc", ScenarioClaudeCode, true},
		{ScenarioOpenAI, ScenarioClaudeCode, false},
		{"openai:p1", ScenarioClaudeCode, false},
		{ScenarioAnthropic, ScenarioAnthropic, true},
		{"anthropic:p1", ScenarioAnthropic, true},
	}
	for _, tt := range tests {
		if got := tt.scenario.Is(tt.base); got != tt.want {
			t.Errorf("(%q).Is(%q) = %v, want %v", tt.scenario, tt.base, got, tt.want)
		}
	}
}

func TestClaudeCodeSupportsProfiles(t *testing.T) {
	d, ok := GetScenarioDescriptor(ScenarioClaudeCode)
	if !ok {
		t.Fatal("claude_code descriptor not found")
	}
	if !d.SupportsProfiles {
		t.Error("claude_code should have SupportsProfiles=true")
	}
}

func TestNonProfileScenariosDoNotSupportProfiles(t *testing.T) {
	for _, s := range []RuleScenario{ScenarioOpenAI, ScenarioAnthropic, ScenarioCustom, ScenarioTeam} {
		d, ok := GetScenarioDescriptor(s)
		if !ok {
			continue
		}
		if d.SupportsProfiles {
			t.Errorf("scenario %q should not have SupportsProfiles=true", s)
		}
	}
}

func TestTeamScenarioDescriptor(t *testing.T) {
	d, ok := GetScenarioDescriptor(ScenarioTeam)
	if !ok {
		t.Fatal("team descriptor not found")
	}
	if !d.AllowRuleBinding {
		t.Error("team should allow rule binding")
	}
	if !d.AllowDirectPathUse {
		t.Error("team should allow direct path use")
	}
	for _, transport := range []ScenarioTransport{TransportOpenAI, TransportAnthropic, TransportImageGen} {
		if !ScenarioSupportsTransport(ScenarioTeam, transport) {
			t.Errorf("team should support transport %q", transport)
		}
	}
}

func TestDshScenarioDescriptor(t *testing.T) {
	d, ok := GetScenarioDescriptor(ScenarioDsh)
	if !ok {
		t.Fatal("dsh descriptor not found")
	}
	if !d.AllowRuleBinding {
		t.Error("dsh should allow rule binding")
	}
	if !d.AllowDirectPathUse {
		t.Error("dsh should allow direct path use")
	}
	for _, transport := range []ScenarioTransport{TransportOpenAI, TransportAnthropic} {
		if !ScenarioSupportsTransport(ScenarioDsh, transport) {
			t.Errorf("dsh should support transport %q", transport)
		}
	}
}

func TestResolveScenarioAlias(t *testing.T) {
	canonical, ok := ResolveScenarioAlias(ScenarioAgent)
	if !ok {
		t.Fatal("expected \"agent\" to resolve as a legacy alias")
	}
	if canonical != ScenarioCustom {
		t.Errorf("ResolveScenarioAlias(agent) = %q, want %q", canonical, ScenarioCustom)
	}

	if _, ok := ResolveScenarioAlias(ScenarioCustom); ok {
		t.Error("\"custom\" itself should not resolve as an alias")
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

func TestIsSimpleProfileAlias(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"cc", true},
		{"mine", true},
		{"p1", true},
		{"my-profile", true},
		{"my_profile", true},
		{"work2", true},
		{"", false},
		{"my profile", false},     // interior space
		{" mine", false},          // leading space
		{"mine ", false},          // trailing space
		{"claude_code:p1", false}, // contains separator
		{"a/b", false},            // path separator
	}
	for _, tc := range cases {
		if got := IsSimpleProfileAlias(tc.in); got != tc.want {
			t.Errorf("IsSimpleProfileAlias(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestValidateProfileName(t *testing.T) {
	valid := []string{"cc", "mine", "profile-1", "team2", "my_cc", "p1x", "px"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("ValidateProfileName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",           // empty
		"default",    // reserved settings filename
		"DEFAULT",    // reserved on case-insensitive filesystems
		"my profile", // space
		"a:b",        // separator
		"a/b",        // path separator
		"café",       // non-ASCII
		"p1",         // reserved ID shape
		"p07",        // reserved ID shape
	}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("ValidateProfileName(%q) = nil, want error", name)
		}
	}
}
