package visionproxy

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func scenarioVisionExt(provider, model string) map[string]interface{} {
	return map[string]interface{}{
		config.ExtensionVisionProxyService: map[string]interface{}{
			"provider": provider,
			"model":    model,
		},
	}
}

func ruleWithVisionService(provider, model string) *typ.Rule {
	return &typ.Rule{Flags: typ.RuleFlags{
		VisionProxyService: &typ.VisionProxyService{Provider: provider, Model: model},
	}}
}

// ---------------------------------------------------------------------------
// Resolve — priority: rule wins over scenario
// ---------------------------------------------------------------------------

func TestResolveVisionService_Priority(t *testing.T) {
	cases := []struct {
		name      string
		ext       map[string]interface{}
		rule      *typ.Rule
		wantModel string // "" => expect nil
	}{
		{
			name:      "rule and scenario both set -> rule wins",
			ext:       scenarioVisionExt("p-scn", "scenario-model"),
			rule:      ruleWithVisionService("p-rule", "rule-model"),
			wantModel: "rule-model",
		},
		{
			name:      "only scenario set -> scenario",
			ext:       scenarioVisionExt("p-scn", "scenario-model"),
			rule:      &typ.Rule{},
			wantModel: "scenario-model",
		},
		{
			name:      "only rule set -> rule",
			ext:       map[string]interface{}{},
			rule:      ruleWithVisionService("p-rule", "rule-model"),
			wantModel: "rule-model",
		},
		{
			name:      "neither set -> nil",
			ext:       map[string]interface{}{},
			rule:      &typ.Rule{},
			wantModel: "",
		},
		{
			name:      "rule set but model empty -> fall back to scenario",
			ext:       scenarioVisionExt("p-scn", "scenario-model"),
			rule:      ruleWithVisionService("p-rule", ""),
			wantModel: "scenario-model",
		},
		{
			name:      "nil rule + scenario set -> scenario",
			ext:       scenarioVisionExt("p-scn", "scenario-model"),
			rule:      nil,
			wantModel: "scenario-model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Scenarios: []typ.ScenarioConfig{
					{Scenario: "claude_code", Extensions: tc.ext},
				},
			}
			s := &Service{}
			svc := s.Resolve(cfg, "claude_code", tc.rule)
			if tc.wantModel == "" {
				if svc != nil {
					t.Fatalf("want nil, got %+v", svc)
				}
				return
			}
			if svc == nil {
				t.Fatal("want non-nil service, got nil")
			}
			if svc.Model != tc.wantModel {
				t.Fatalf("want model %q, got %q", tc.wantModel, svc.Model)
			}
			if !svc.Active {
				t.Fatal("resolved service must be active so the processor accepts it")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseScenarioVisionService — Extensions decoding
// ---------------------------------------------------------------------------

func TestParseScenarioVisionService(t *testing.T) {
	cases := []struct {
		name string
		ext  map[string]interface{}
		want *struct{ provider, model string }
	}{
		{name: "nil extensions", ext: nil, want: nil},
		{name: "missing key", ext: map[string]interface{}{"other": "value"}, want: nil},
		{name: "wrong shape", ext: map[string]interface{}{"vision_proxy_service": "not-an-object"}, want: nil},
		{name: "missing provider", ext: scenarioVisionExt("", "claude-3-5-sonnet"), want: nil},
		{name: "missing model", ext: scenarioVisionExt("p-uuid", ""), want: nil},
		{name: "valid", ext: scenarioVisionExt("p-uuid", "claude-3-5-sonnet"), want: &struct{ provider, model string }{"p-uuid", "claude-3-5-sonnet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseScenarioVisionService(tc.ext)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("want non-nil, got nil")
			}
			if got.Provider != tc.want.provider || got.Model != tc.want.model {
				t.Fatalf("want {%s,%s}, got {%s,%s}", tc.want.provider, tc.want.model, got.Provider, got.Model)
			}
			if !got.Active {
				t.Fatal("parsed service should be active")
			}
		})
	}
}
