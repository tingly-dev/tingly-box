package server

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

func betaReqWithLatestUserText(text string) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-sonnet-4"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: text}}},
			},
		},
	}
}

func betaReqWithLatestToolResult() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-sonnet-4"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "please compact"}}},
			},
			{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "ok, using tool"}}},
			},
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfToolResult: &anthropic.BetaToolResultBlockParam{ToolUseID: "t1"}},
				},
			},
		},
	}
}

func TestCompactWakeMatches(t *testing.T) {
	rule := &typ.Rule{}

	cases := []struct {
		name     string
		scenario typ.RuleScenario
		rule     *typ.Rule
		keyword  string
		req      any
		want     bool
	}{
		{"matches case-insensitively", typ.ScenarioClaudeCode, rule, "compact", betaReqWithLatestUserText("please COMPACT now"), true},
		{"no keyword in message", typ.ScenarioClaudeCode, rule, "compact", betaReqWithLatestUserText("hello there"), false},
		{"empty keyword disables the check", typ.ScenarioClaudeCode, rule, "", betaReqWithLatestUserText("please compact"), false},
		{"non-claude_code scenario never matches", typ.ScenarioAnthropic, rule, "compact", betaReqWithLatestUserText("please compact"), false},
		{"nil rule never matches", typ.ScenarioClaudeCode, nil, "compact", betaReqWithLatestUserText("please compact"), false},
		{"nil request never matches", typ.ScenarioClaudeCode, rule, "compact", nil, false},
		{"custom keyword", typ.ScenarioClaudeCode, rule, "/compact", betaReqWithLatestUserText("/compact"), true},
		{
			"latest message is a tool_result -> does not fall back to earlier text",
			typ.ScenarioClaudeCode, rule, "compact", betaReqWithLatestToolResult(), false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compactWakeMatches(tc.scenario, tc.rule, tc.keyword, tc.req)
			if got != tc.want {
				t.Fatalf("compactWakeMatches() = %v, want %v", got, tc.want)
			}
		})
	}
}

// compactWakeTestServer builds a Server backed by a real *config.Config (temp
// dir, real provider store) so GetEffectiveCompactKeyword / GetProviderByUUID
// behave exactly as in production.
func compactWakeTestServer(t *testing.T, keyword string, seedBuiltinProvider bool) *Server {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	if seedBuiltinProvider {
		if err := cfg.AddProvider(&typ.Provider{
			UUID:     virtualserver.BuiltinAnthropicUUID,
			Name:     virtualserver.BuiltinAnthropicName,
			APIBase:  "vmodel://local",
			AuthType: typ.AuthTypeVirtual,
			Enabled:  true,
		}); err != nil {
			t.Fatalf("failed to seed builtin provider: %v", err)
		}
	}
	if err := cfg.SetScenarioStringFlag(typ.ScenarioClaudeCode, config.FlagCompactKeyword, keyword); err != nil {
		t.Fatalf("failed to set scenario compact_keyword: %v", err)
	}
	return &Server{config: cfg}
}

func TestApplyCompactWake_MatchReturnsBuiltinVModelService(t *testing.T) {
	s := compactWakeTestServer(t, "compact", true)
	rule := &typ.Rule{}

	provider, svc := s.applyCompactWake(typ.ScenarioClaudeCode, rule, betaReqWithLatestUserText("please compact"))
	if provider == nil || svc == nil {
		t.Fatal("expected a forced (provider, service) pair on match")
	}
	if svc.Provider != virtualserver.BuiltinAnthropicUUID || svc.Model != compactWakeModel {
		t.Fatalf("unexpected forced service: %+v", svc)
	}
	if !svc.Active {
		t.Fatal("forced service must be active")
	}
}

func TestApplyCompactWake_NoMatchIsNoOp(t *testing.T) {
	s := compactWakeTestServer(t, "compact", true)
	rule := &typ.Rule{}

	provider, svc := s.applyCompactWake(typ.ScenarioClaudeCode, rule, betaReqWithLatestUserText("hello there"))
	if provider != nil || svc != nil {
		t.Fatalf("expected no-op, got provider=%+v svc=%+v", provider, svc)
	}
}

func TestApplyCompactWake_MissingBuiltinProviderIsNoOp(t *testing.T) {
	s := compactWakeTestServer(t, "compact", false)
	rule := &typ.Rule{}

	provider, svc := s.applyCompactWake(typ.ScenarioClaudeCode, rule, betaReqWithLatestUserText("please compact"))
	if provider != nil || svc != nil {
		t.Fatalf("expected no-op when builtin provider row is absent, got provider=%+v svc=%+v", provider, svc)
	}
}

func TestApplyCompactWake_RuleOverridesScenarioKeyword(t *testing.T) {
	s := compactWakeTestServer(t, "compact", true)
	rule := &typ.Rule{Flags: typ.RuleFlags{CompactKeyword: "squash"}}

	// Scenario default "compact" must NOT match once the rule overrides it.
	if _, svc := s.applyCompactWake(typ.ScenarioClaudeCode, rule, betaReqWithLatestUserText("please compact")); svc != nil {
		t.Fatal("expected no match against the overridden-away scenario keyword")
	}
	// The rule's own keyword must match.
	_, svc := s.applyCompactWake(typ.ScenarioClaudeCode, rule, betaReqWithLatestUserText("please squash"))
	if svc == nil {
		t.Fatal("expected match against the rule-level keyword override")
	}
}
