package visionproxy

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

const applyTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func betaReqWithImage() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "what is this?"}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      applyTestPNG,
						MediaType: anthropic.BetaBase64ImageSourceMediaType("image/png"),
					}),
				},
			},
		},
	}
}

// testConfig builds a config.Config whose scenario config carries the given
// Extensions.
func testConfig(scenario typ.RuleScenario, ext map[string]interface{}) *config.Config {
	return &config.Config{
		Scenarios: []typ.ScenarioConfig{
			{Scenario: scenario, Extensions: ext},
		},
	}
}

// testService builds a Service with a fake vision processor that echoes the
// model used and accepts any provider UUID as resolvable (Apply's tests care
// about scope selection, not provider resolution).
func testService() *Service {
	return NewService(&VisionProxyProcessor{
		Client:   echoingVisionClient{},
		Resolver: alwaysResolvingProvider{},
	})
}

// alwaysResolvingProvider treats every provider UUID as usable.
type alwaysResolvingProvider struct{}

func (alwaysResolvingProvider) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return &typ.Provider{UUID: uuid, Name: "stub"}, nil
}

// echoingVisionClient behaves like fakeVisionClient but always succeeds,
// echoing the service model so tests can assert WHICH service was used.
type echoingVisionClient struct{}

func (echoingVisionClient) Describe(_ context.Context, svc *loadbalance.Service, _, _, _ string) (string, error) {
	if svc != nil {
		return "desc via " + svc.Model, nil
	}
	return "desc", nil
}

func firstImagePresent(req *anthropic.BetaMessageNewParams) bool {
	for _, b := range req.Messages[0].Content {
		if b.OfImage != nil {
			return true
		}
	}
	return false
}

func joinedText(req *anthropic.BetaMessageNewParams) string {
	var sb strings.Builder
	for _, b := range req.Messages[0].Content {
		if b.OfText != nil {
			sb.WriteString(b.OfText.Text)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Apply — end behavior (image replacement, scope selection)
// ---------------------------------------------------------------------------

func TestApply_RuleServiceUsedWhenBothConfigured(t *testing.T) {
	s := testService()
	cfg := testConfig("claude_code", scenarioVisionExt("p-scn", "scenario-model"))
	rule := ruleWithVisionService("p-rule", "rule-model")

	req := betaReqWithImage()
	s.Apply(context.Background(), cfg, "claude_code", rule, req)

	if firstImagePresent(req) {
		t.Fatal("image was not replaced")
	}
	if !strings.Contains(joinedText(req), "via rule-model") {
		t.Fatalf("expected rule-model to describe the image, got: %q", joinedText(req))
	}
}

func TestApply_ScenarioFallbackWhenRuleUnset(t *testing.T) {
	s := testService()
	cfg := testConfig("claude_code", scenarioVisionExt("p-scn", "scenario-model"))

	req := betaReqWithImage()
	s.Apply(context.Background(), cfg, "claude_code", &typ.Rule{}, req)

	if firstImagePresent(req) {
		t.Fatal("image was not replaced")
	}
	if !strings.Contains(joinedText(req), "via scenario-model") {
		t.Fatalf("expected scenario-model to describe the image, got: %q", joinedText(req))
	}
}

func TestApply_NoServiceIsNoOp(t *testing.T) {
	s := testService()
	cfg := testConfig("claude_code", map[string]interface{}{})

	req := betaReqWithImage()
	s.Apply(context.Background(), cfg, "claude_code", &typ.Rule{}, req)
	if !firstImagePresent(req) {
		t.Fatal("image was replaced even though neither scope configured a service")
	}
}

// Regression for PR #1082's profiled-scenario wiring: a service stored under
// "claude_code:p1" is found when the request's scenario is "claude_code:p1".
func TestApply_ProfiledScenario(t *testing.T) {
	s := testService()
	cfg := testConfig("claude_code:p1", scenarioVisionExt("p-scn", "scenario-model"))

	req := betaReqWithImage()
	s.Apply(context.Background(), cfg, "claude_code:p1", &typ.Rule{}, req)
	if firstImagePresent(req) {
		t.Fatal("profiled-scenario service was not applied")
	}
}

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
