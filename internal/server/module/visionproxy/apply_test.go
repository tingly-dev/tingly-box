package visionproxy

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/visionproxy/visionproxytest"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

func betaReqWithImage() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "what is this?"}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      visionproxytest.PNG,
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

// testService builds a Service with a stub vision processor that echoes the
// model used.
func testService() *Service {
	return NewService(visionproxytest.NewProcessor())
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
	cfg := testConfig("claude_code", visionproxytest.ScenarioExt("p-scn", "scenario-model"))
	rule := visionproxytest.RuleWithService("p-rule", "rule-model")

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
	cfg := testConfig("claude_code", visionproxytest.ScenarioExt("p-scn", "scenario-model"))

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
	cfg := testConfig("claude_code:p1", visionproxytest.ScenarioExt("p-scn", "scenario-model"))

	req := betaReqWithImage()
	s.Apply(context.Background(), cfg, "claude_code:p1", &typ.Rule{}, req)
	if firstImagePresent(req) {
		t.Fatal("profiled-scenario service was not applied")
	}
}
