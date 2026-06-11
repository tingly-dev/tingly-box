package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newGinContext builds a minimal *gin.Context for tests that only need to
// read request headers (auto-detect path). Header values can be set on the
// returned request before passing the context into the unit under test.
func newGinContext(t *testing.T) *gin.Context {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	return c
}

func TestResolveRuleFlags_NilRule(t *testing.T) {
	got := resolveRuleFlags(newGinContext(t), nil)
	want := typ.RuleFlags{}
	if got != want {
		t.Errorf("resolveRuleFlags(nil) = %#v, want zero value %#v", got, want)
	}
}

func TestResolveRuleFlags_CopiesFlags(t *testing.T) {
	rule := &typ.Rule{
		Flags: typ.RuleFlags{
			CursorCompat:           true,
			SkipUsage:              true,
			CustomUserAgent:        "MyApp/1.0",
			UseMaxCompletionTokens: true,
			UseMaxTokens:           true,
		},
	}
	got := resolveRuleFlags(newGinContext(t), rule)
	if !got.CursorCompat || !got.SkipUsage || !got.UseMaxCompletionTokens || !got.UseMaxTokens {
		t.Errorf("bool flags lost: %#v", got)
	}
	if got.CustomUserAgent != "MyApp/1.0" {
		t.Errorf("CustomUserAgent = %q, want %q", got.CustomUserAgent, "MyApp/1.0")
	}
}

func TestResolveRuleFlags_AutoDetectFoldedIn(t *testing.T) {
	rule := &typ.Rule{
		Flags: typ.RuleFlags{
			CursorCompat:     false,
			CursorCompatAuto: true,
		},
	}
	c := newGinContext(t)
	c.Request.Header.Set("User-Agent", "Cursor/0.42")

	got := resolveRuleFlags(c, rule)
	if !got.CursorCompat {
		t.Errorf("expected CursorCompat folded to true via auto-detect, got %#v", got)
	}
}

func TestResolveRuleFlags_AutoDetectInactiveWithoutHeader(t *testing.T) {
	rule := &typ.Rule{
		Flags: typ.RuleFlags{
			CursorCompat:     false,
			CursorCompatAuto: true,
		},
	}
	got := resolveRuleFlags(newGinContext(t), rule)
	if got.CursorCompat {
		t.Errorf("expected CursorCompat to stay false without Cursor headers, got %#v", got)
	}
}

func TestShouldStripUsage_NilExtra(t *testing.T) {
	if shouldStripUsage(nil) {
		t.Errorf("nil extra map should not strip usage")
	}
}

func TestShouldStripUsage_EmptyExtra(t *testing.T) {
	if shouldStripUsage(map[string]interface{}{}) {
		t.Errorf("empty extra map should not strip usage")
	}
}

func TestShouldStripUsage_CursorCompatTrue(t *testing.T) {
	if !shouldStripUsage(map[string]interface{}{"cursor_compat": true}) {
		t.Errorf("cursor_compat=true should strip usage")
	}
}

func TestShouldStripUsage_SkipUsageTrue(t *testing.T) {
	if !shouldStripUsage(map[string]interface{}{"skip_usage": true}) {
		t.Errorf("skip_usage=true should strip usage")
	}
}

func TestShouldStripUsage_BothTrue(t *testing.T) {
	if !shouldStripUsage(map[string]interface{}{
		"cursor_compat": true,
		"skip_usage":    true,
	}) {
		t.Errorf("both flags true should strip usage")
	}
}

func TestShouldStripUsage_BothFalse(t *testing.T) {
	if shouldStripUsage(map[string]interface{}{
		"cursor_compat": false,
		"skip_usage":    false,
	}) {
		t.Errorf("both flags false should not strip usage")
	}
}

func TestShouldStripUsage_NonBoolValueIgnored(t *testing.T) {
	// Defensive: a non-bool sneaks past the type assertion as false.
	if shouldStripUsage(map[string]interface{}{
		"cursor_compat": "yes",
		"skip_usage":    1,
	}) {
		t.Errorf("non-bool values should be treated as false, not strip")
	}
}

func TestRulePreBaseTransforms_NoFlags(t *testing.T) {
	got := rulePreBaseTransforms(typ.RuleFlags{})
	if got != nil {
		t.Errorf("expected nil for zero-value flags, got %d transforms", len(got))
	}
}

func TestRulePreBaseTransforms_CursorCompat(t *testing.T) {
	got := rulePreBaseTransforms(typ.RuleFlags{CursorCompat: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	if _, ok := got[0].(*transform.OpenAICursorCompatTransform); !ok {
		t.Errorf("expected *transform.OpenAICursorCompatTransform, got %T", got[0])
	}
}

func TestRulePreBaseTransforms_OtherFlagsAlone_NoTransform(t *testing.T) {
	// Post-base flags must not surface in the pre-base list.
	got := rulePreBaseTransforms(typ.RuleFlags{
		UseMaxCompletionTokens: true,
		UseMaxTokens:           true,
		SkipUsage:              true,
		CustomUserAgent:        "Foo/1.0",
	})
	if got != nil {
		t.Errorf("expected nil, got %d transforms", len(got))
	}
}

func TestRulePreVendorTransforms_NoFlags(t *testing.T) {
	got := rulePreVendorTransforms(typ.RuleFlags{})
	if got != nil {
		t.Errorf("expected nil for zero-value flags, got %d transforms", len(got))
	}
}

func TestRulePreVendorTransforms_UseMaxCompletionTokens(t *testing.T) {
	got := rulePreVendorTransforms(typ.RuleFlags{UseMaxCompletionTokens: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	tf, ok := got[0].(*transform.OpenAIMaxTokensRewriteTransform)
	if !ok {
		t.Fatalf("expected *transform.OpenAIMaxTokensRewriteTransform, got %T", got[0])
	}
	if !tf.UseMaxCompletionTokens || tf.UseMaxTokens {
		t.Errorf("flag values not propagated: %#v", tf)
	}
}

func TestRulePreVendorTransforms_UseMaxTokens(t *testing.T) {
	got := rulePreVendorTransforms(typ.RuleFlags{UseMaxTokens: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	tf := got[0].(*transform.OpenAIMaxTokensRewriteTransform)
	if tf.UseMaxCompletionTokens || !tf.UseMaxTokens {
		t.Errorf("flag values not propagated: %#v", tf)
	}
}

func TestRulePreVendorTransforms_ThinkingEffort(t *testing.T) {
	got := rulePreVendorTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortHigh})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	tf, ok := got[0].(*transform.RuleThinkingTransform)
	if !ok {
		t.Fatalf("expected *transform.RuleThinkingTransform, got %T", got[0])
	}
	if tf.Effort != typ.ThinkingEffortHigh {
		t.Errorf("effort not propagated: got %q, want %q", tf.Effort, typ.ThinkingEffortHigh)
	}
}

func TestRulePreVendorTransforms_ThinkingEffortEmpty_NoTransform(t *testing.T) {
	got := rulePreVendorTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault})
	if got != nil {
		t.Errorf("empty thinking effort should add no transform, got %d", len(got))
	}
}

func TestRulePreVendorTransforms_CursorCompatAlone_NoTransform(t *testing.T) {
	// CursorCompat is a preBase flag — it must not surface in the preVendor
	// list. This is the safety net for the rule-flag-to-transform migration: if
	// anyone wires cursor_compat into rulePreVendorTransforms by mistake, this
	// test goes red.
	got := rulePreVendorTransforms(typ.RuleFlags{
		CursorCompat:    true,
		SkipUsage:       true,
		CustomUserAgent: "Foo/1.0",
	})
	if got != nil {
		t.Errorf("expected nil, got %d transforms", len(got))
	}
}

// TestIsBillingHeaderScenario verifies the detection of billing header scenarios
func TestIsBillingHeaderScenario(t *testing.T) {
	tests := []struct {
		name     string
		scenario typ.RuleScenario
		want     bool
	}{
		{
			name:     "Claude Code scenario",
			scenario: typ.ScenarioClaudeCode,
			want:     true,
		},
		{
			name:     "Claude Desktop scenario",
			scenario: typ.ScenarioClaudeDesktop,
			want:     true,
		},
		{
			name:     "OpenAI scenario",
			scenario: typ.ScenarioOpenAI,
			want:     false,
		},
		{
			name:     "Anthropic scenario",
			scenario: typ.ScenarioAnthropic,
			want:     false,
		},
		{
			name:     "Claude Code profiled scenario",
			scenario: typ.RuleScenario("claude_code:p1"),
			want:     true,
		},
		{
			name:     "Claude Desktop profiled scenario",
			scenario: typ.RuleScenario("claude_desktop:p1"),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBillingHeaderScenario(tt.scenario)
			if got != tt.want {
				t.Errorf("isBillingHeaderScenario() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAutoSetCleanHeaderFlag verifies the flag auto-setting logic
func TestAutoSetCleanHeaderFlag(t *testing.T) {
	tests := []struct {
		name            string
		flags           typ.RuleFlags
		sourceAPI      protocol.APIType
		targetAPI      protocol.APIType
		scenario       typ.RuleScenario
		wantCleanHeader bool
	}{
		{
			name:            "Auto-set for Claude Code transformation",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeOpenAIChat,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: true,
		},
		{
			name:            "Manual CleanHeader=true preserved",
			flags:           typ.RuleFlags{CleanHeader: true},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeAnthropicV1,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: true,
		},
		{
			name:            "No transformation, not set",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeAnthropicV1,
			scenario:        typ.ScenarioClaudeCode,
			wantCleanHeader: false,
		},
		{
			name:            "Non-billing scenario, not set",
			flags:           typ.RuleFlags{CleanHeader: false},
			sourceAPI:       protocol.TypeAnthropicV1,
			targetAPI:       protocol.TypeOpenAIChat,
			scenario:        typ.ScenarioOpenAI,
			wantCleanHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := autoSetCleanHeaderFlag(tt.flags, tt.sourceAPI, tt.targetAPI, tt.scenario)
			if result.CleanHeader != tt.wantCleanHeader {
				t.Errorf("autoSetCleanHeaderFlag() CleanHeader = %v, want %v", result.CleanHeader, tt.wantCleanHeader)
			}
		})
	}
}

// TestRulePreBaseTransformsWithCleanHeader verifies the transform building
func TestRulePreBaseTransformsWithCleanHeader(t *testing.T) {
	tests := []struct {
		name            string
		flags           typ.RuleFlags
		wantCleanCount int
	}{
		{
			name:            "CleanHeader flag adds transform",
			flags:           typ.RuleFlags{CleanHeader: true},
			wantCleanCount: 1,
		},
		{
			name:            "No CleanHeader flag, no transform",
			flags:           typ.RuleFlags{CleanHeader: false},
			wantCleanCount: 0,
		},
		{
			name:            "CursorCompat + CleanHeader both added",
			flags:           typ.RuleFlags{CursorCompat: true, CleanHeader: true},
			wantCleanCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transforms := rulePreBaseTransforms(tt.flags)

			cleanCount := 0
			for _, transform := range transforms {
				if transform.Name() == "clean_header" {
					cleanCount++
				}
			}

			if cleanCount != tt.wantCleanCount {
				t.Errorf("CleanHeader count = %v, want %v", cleanCount, tt.wantCleanCount)
			}
		})
	}
}

// TestResolveRuleFlagsWithScenario_ThinkingEffort verifies the ThinkingEffort flag
// merging logic ensures rule-level flags take priority over scenario defaults.
func TestResolveRuleFlagsWithScenario_ThinkingEffort(t *testing.T) {
	tests := []struct {
		name                 string
		ruleFlags           typ.RuleFlags
		scenarioFlags       typ.ScenarioFlags
		wantThinkingEffort  typ.ThinkingEffortLevel
	}{
		{
			name: "Rule explicit setting preserved over scenario default",
			ruleFlags:           typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortOff},
			scenarioFlags:       typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortHigh},
			wantThinkingEffort:  typ.ThinkingEffortOff,
		},
		{
			name: "Rule explicit level preserved over scenario different level",
			ruleFlags:           typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortLow},
			scenarioFlags:       typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortMedium},
			wantThinkingEffort:  typ.ThinkingEffortLow,
		},
		{
			name: "Scenario default injected when rule is default",
			ruleFlags:           typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			scenarioFlags:       typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortHigh},
			wantThinkingEffort:  typ.ThinkingEffortHigh,
		},
		{
			name: "Both default remains default",
			ruleFlags:           typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			scenarioFlags:       typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			wantThinkingEffort:  typ.ThinkingEffortDefault,
		},
		{
			name: "Rule explicit level preserved when scenario is default",
			ruleFlags:           typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortMedium},
			scenarioFlags:       typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			wantThinkingEffort:  typ.ThinkingEffortMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			rule := &typ.Rule{Flags: tt.ruleFlags}
			scenarioConfig := &typ.ScenarioConfig{Flags: tt.scenarioFlags}

			result := resolveRuleFlagsWithScenario(
				c,
				rule,
				typ.ScenarioClaudeCode,
				scenarioConfig,
				protocol.TypeAnthropicV1,
				protocol.TypeOpenAIChat,
				nil,
			)

			if result.ThinkingEffort != tt.wantThinkingEffort {
				t.Errorf("ThinkingEffort = %v, want %v", result.ThinkingEffort, tt.wantThinkingEffort)
			}
		})
	}
}

func TestResolveRuleFlagsWithScenario_CustomUserAgent(t *testing.T) {
	tests := []struct {
		name          string
		ruleFlags     typ.RuleFlags
		scenarioFlags typ.ScenarioFlags
		wantUA        string
	}{
		{
			name:          "Scenario default injected when rule is empty",
			ruleFlags:     typ.RuleFlags{},
			scenarioFlags: typ.ScenarioFlags{CustomUserAgent: "Scenario/1.0"},
			wantUA:        "Scenario/1.0",
		},
		{
			name:          "Rule explicit UA wins over scenario default",
			ruleFlags:     typ.RuleFlags{CustomUserAgent: "Rule/2.0"},
			scenarioFlags: typ.ScenarioFlags{CustomUserAgent: "Scenario/1.0"},
			wantUA:        "Rule/2.0",
		},
		{
			name:          "Rule UA preserved when scenario empty",
			ruleFlags:     typ.RuleFlags{CustomUserAgent: "Rule/2.0"},
			scenarioFlags: typ.ScenarioFlags{},
			wantUA:        "Rule/2.0",
		},
		{
			name:          "Both empty stays empty",
			ruleFlags:     typ.RuleFlags{},
			scenarioFlags: typ.ScenarioFlags{},
			wantUA:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			rule := &typ.Rule{Flags: tt.ruleFlags}
			scenarioConfig := &typ.ScenarioConfig{Flags: tt.scenarioFlags}

			result := resolveRuleFlagsWithScenario(
				c,
				rule,
				typ.ScenarioClaudeCode,
				scenarioConfig,
				protocol.TypeAnthropicV1,
				protocol.TypeOpenAIChat,
				nil,
			)

			if result.CustomUserAgent != tt.wantUA {
				t.Errorf("CustomUserAgent = %q, want %q", result.CustomUserAgent, tt.wantUA)
			}
		})
	}
}

func TestResolveRuleFlagsWithScenario_CleanHeaderSuppressedForClaudeOAuth(t *testing.T) {
	oauthProvider := &typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{Issuer: ai.IssuerClaudeCode},
	}
	otherProvider := &typ.Provider{AuthType: typ.AuthTypeAPIKey}

	c, _ := gin.CreateTestContext(nil)
	rule := &typ.Rule{Flags: typ.RuleFlags{CleanHeader: true}}
	scenarioConfig := &typ.ScenarioConfig{}

	// CleanHeader should be suppressed for Claude OAuth provider.
	got := resolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, oauthProvider)
	if got.CleanHeader {
		t.Error("CleanHeader should be suppressed for Claude OAuth provider")
	}

	// CleanHeader should be preserved for any other provider type.
	got = resolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, otherProvider)
	if !got.CleanHeader {
		t.Error("CleanHeader should be preserved for non-OAuth provider")
	}

	// nil provider: no suppression.
	got = resolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, nil)
	if !got.CleanHeader {
		t.Error("CleanHeader should be preserved when provider is nil")
	}
}
