package protocolserver

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
	got := ResolveRuleFlags(newGinContext(t), nil)
	if !got.IsZero() {
		t.Errorf("resolveRuleFlags(nil) = %#v, want zero value", got)
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
	got := ResolveRuleFlags(newGinContext(t), rule)
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

	got := ResolveRuleFlags(c, rule)
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
	got := ResolveRuleFlags(newGinContext(t), rule)
	if got.CursorCompat {
		t.Errorf("expected CursorCompat to stay false without Cursor headers, got %#v", got)
	}
}

func TestShouldStripUsage_NilExtra(t *testing.T) {
	if ShouldStripUsage(nil) {
		t.Errorf("nil extra map should not strip usage")
	}
}

func TestShouldStripUsage_EmptyExtra(t *testing.T) {
	if ShouldStripUsage(map[string]interface{}{}) {
		t.Errorf("empty extra map should not strip usage")
	}
}

func TestShouldStripUsage_CursorCompatTrue(t *testing.T) {
	if !ShouldStripUsage(map[string]interface{}{"cursor_compat": true}) {
		t.Errorf("cursor_compat=true should strip usage")
	}
}

func TestShouldStripUsage_SkipUsageTrue(t *testing.T) {
	if !ShouldStripUsage(map[string]interface{}{"skip_usage": true}) {
		t.Errorf("skip_usage=true should strip usage")
	}
}

func TestShouldStripUsage_BothTrue(t *testing.T) {
	if !ShouldStripUsage(map[string]interface{}{
		"cursor_compat": true,
		"skip_usage":    true,
	}) {
		t.Errorf("both flags true should strip usage")
	}
}

func TestShouldStripUsage_BothFalse(t *testing.T) {
	if ShouldStripUsage(map[string]interface{}{
		"cursor_compat": false,
		"skip_usage":    false,
	}) {
		t.Errorf("both flags false should not strip usage")
	}
}

func TestShouldStripUsage_NonBoolValueIgnored(t *testing.T) {
	// Defensive: a non-bool sneaks past the type assertion as false.
	if ShouldStripUsage(map[string]interface{}{
		"cursor_compat": "yes",
		"skip_usage":    1,
	}) {
		t.Errorf("non-bool values should be treated as false, not strip")
	}
}

func TestRulePreBaseTransforms_NoFlags(t *testing.T) {
	got := RulePreBaseTransforms(typ.RuleFlags{})
	if got != nil {
		t.Errorf("expected nil for zero-value flags, got %d transforms", len(got))
	}
}

func TestRulePreBaseTransforms_CursorCompat(t *testing.T) {
	got := RulePreBaseTransforms(typ.RuleFlags{CursorCompat: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	if _, ok := got[0].(*transform.OpenAICursorCompatTransform); !ok {
		t.Errorf("expected *transform.OpenAICursorCompatTransform, got %T", got[0])
	}
}

func TestRulePreBaseTransforms_OtherFlagsAlone_NoTransform(t *testing.T) {
	// Post-base flags must not surface in the pre-base list.
	got := RulePreBaseTransforms(typ.RuleFlags{
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
	got := RulePreVendorTransforms(typ.RuleFlags{})
	if got != nil {
		t.Errorf("expected nil for zero-value flags, got %d transforms", len(got))
	}
}

func TestRulePreVendorTransforms_UseMaxCompletionTokens(t *testing.T) {
	got := RulePreVendorTransforms(typ.RuleFlags{UseMaxCompletionTokens: true})
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
	got := RulePreVendorTransforms(typ.RuleFlags{UseMaxTokens: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	tf := got[0].(*transform.OpenAIMaxTokensRewriteTransform)
	if tf.UseMaxCompletionTokens || !tf.UseMaxTokens {
		t.Errorf("flag values not propagated: %#v", tf)
	}
}

func TestRulePreVendorTransforms_ThinkingEffort(t *testing.T) {
	got := RulePreVendorTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortHigh})
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
	got := RulePreVendorTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault})
	if got != nil {
		t.Errorf("empty thinking effort should add no transform, got %d", len(got))
	}
}

func TestRulePreVendorTransforms_CursorCompatAlone_NoTransform(t *testing.T) {
	// CursorCompat is a preBase flag — it must not surface in the preVendor
	// list. This is the safety net for the rule-flag-to-transform migration: if
	// anyone wires cursor_compat into rulePreVendorTransforms by mistake, this
	// test goes red.
	got := RulePreVendorTransforms(typ.RuleFlags{
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
		sourceAPI       protocol.APIType
		targetAPI       protocol.APIType
		scenario        typ.RuleScenario
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
		name           string
		flags          typ.RuleFlags
		wantCleanCount int
	}{
		{
			name:           "CleanHeader flag adds transform",
			flags:          typ.RuleFlags{CleanHeader: true},
			wantCleanCount: 1,
		},
		{
			name:           "No CleanHeader flag, no transform",
			flags:          typ.RuleFlags{CleanHeader: false},
			wantCleanCount: 0,
		},
		{
			name:           "CursorCompat + CleanHeader both added",
			flags:          typ.RuleFlags{CursorCompat: true, CleanHeader: true},
			wantCleanCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transforms := RulePreBaseTransforms(tt.flags)

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
		name               string
		ruleFlags          typ.RuleFlags
		scenarioFlags      typ.ScenarioFlags
		wantThinkingEffort typ.ThinkingEffortLevel
	}{
		{
			name:               "Rule explicit setting preserved over scenario default",
			ruleFlags:          typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortOff},
			scenarioFlags:      typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortHigh},
			wantThinkingEffort: typ.ThinkingEffortOff,
		},
		{
			name:               "Rule explicit level preserved over scenario different level",
			ruleFlags:          typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortLow},
			scenarioFlags:      typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortMedium},
			wantThinkingEffort: typ.ThinkingEffortLow,
		},
		{
			name:               "Scenario default injected when rule is default",
			ruleFlags:          typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			scenarioFlags:      typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortHigh},
			wantThinkingEffort: typ.ThinkingEffortHigh,
		},
		{
			name:               "Both default remains default",
			ruleFlags:          typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			scenarioFlags:      typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			wantThinkingEffort: typ.ThinkingEffortDefault,
		},
		{
			name:               "Rule explicit level preserved when scenario is default",
			ruleFlags:          typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortMedium},
			scenarioFlags:      typ.ScenarioFlags{ThinkingEffort: typ.ThinkingEffortDefault},
			wantThinkingEffort: typ.ThinkingEffortMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			rule := &typ.Rule{Flags: tt.ruleFlags}
			scenarioConfig := &typ.ScenarioConfig{Flags: tt.scenarioFlags}

			result := ResolveRuleFlagsWithScenario(
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

			result := ResolveRuleFlagsWithScenario(
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

func TestApplyClientUserAgent_AttachesInboundUA(t *testing.T) {
	c := newGinContext(t)
	c.Request.Header.Set("User-Agent", "cherry-studio/1.2")

	applyClientUserAgent(c)

	if got := typ.GetClientUserAgent(c.Request.Context()); got != "cherry-studio/1.2" {
		t.Errorf("GetClientUserAgent = %q, want %q", got, "cherry-studio/1.2")
	}
}

func TestApplyClientUserAgent_NoHeaderIsNoOp(t *testing.T) {
	c := newGinContext(t)
	// No User-Agent header set on the inbound request.

	applyClientUserAgent(c)

	if got := typ.GetClientUserAgent(c.Request.Context()); got != "" {
		t.Errorf("GetClientUserAgent = %q, want empty (no inbound UA to forward)", got)
	}
}

func TestApplyClientUserAgent_NilRequestIsNoOp(t *testing.T) {
	// A gin.Context with a nil Request must not panic.
	c, _ := gin.CreateTestContext(nil)
	applyClientUserAgent(c) // must not panic
}

// TestResolveRuleFlagsWithScenario_AttachesClientUserAgent verifies the single
// merge point forwards the inbound client UA into the request context
// unconditionally — ruleFlagTransport is the sole arbiter of the UA
// precedence, so the merge point attaches both candidates and takes no
// precedence decision of its own.
func TestResolveRuleFlagsWithScenario_AttachesClientUserAgent(t *testing.T) {
	t.Run("inbound UA attached when no explicit override", func(t *testing.T) {
		c := newGinContext(t)
		c.Request.Header.Set("User-Agent", "cherry-studio/1.2")
		rule := &typ.Rule{Flags: typ.RuleFlags{}}

		ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioAnthropic, &typ.ScenarioConfig{},
			protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, nil)

		if got := typ.GetClientUserAgent(c.Request.Context()); got != "cherry-studio/1.2" {
			t.Errorf("client UA in ctx = %q, want %q", got, "cherry-studio/1.2")
		}
	})

	t.Run("override and client UA are both attached", func(t *testing.T) {
		c := newGinContext(t)
		c.Request.Header.Set("User-Agent", "cherry-studio/1.2")
		rule := &typ.Rule{Flags: typ.RuleFlags{CustomUserAgent: "Rule/2.0"}}

		ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioAnthropic, &typ.ScenarioConfig{},
			protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, nil)

		ctx := c.Request.Context()
		if got := typ.GetRuleFlags(ctx).CustomUserAgent; got != "Rule/2.0" {
			t.Errorf("custom UA in ctx = %q, want %q", got, "Rule/2.0")
		}
		// Both candidates ride the ctx; the transport resolves the precedence
		// (override wins there — pinned by TestRuleFlagTransport_RuleWinsOverClient).
		if got := typ.GetClientUserAgent(ctx); got != "cherry-studio/1.2" {
			t.Errorf("client UA in ctx = %q, want %q", got, "cherry-studio/1.2")
		}
	})
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
	got := ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, oauthProvider)
	if got.CleanHeader {
		t.Error("CleanHeader should be suppressed for Claude OAuth provider")
	}

	// CleanHeader should be preserved for any other provider type.
	got = ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, otherProvider)
	if !got.CleanHeader {
		t.Error("CleanHeader should be preserved for non-OAuth provider")
	}

	// nil provider: no suppression.
	got = ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, scenarioConfig,
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, nil)
	if !got.CleanHeader {
		t.Error("CleanHeader should be preserved when provider is nil")
	}
}

func TestApplyRuleFlags(t *testing.T) {
	t.Run("attaches the resolved flags to the request context", func(t *testing.T) {
		c := newGinContext(t)
		applyRuleFlags(c, typ.RuleFlags{ClaudeOrgID: "org-uuid"})
		if got := typ.GetRuleFlags(c.Request.Context()).ClaudeOrgID; got != "org-uuid" {
			t.Errorf("claude org id in ctx = %q, want %q", got, "org-uuid")
		}
	})

	t.Run("zero flags read back as zero", func(t *testing.T) {
		c := newGinContext(t)
		applyRuleFlags(c, typ.RuleFlags{})
		if got := typ.GetRuleFlags(c.Request.Context()); !got.IsZero() {
			t.Errorf("flags in ctx = %+v, want zero value", got)
		}
	})

	t.Run("nil request must not panic", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		applyRuleFlags(c, typ.RuleFlags{}) // must not panic
	})
}

func TestResolveRuleFlagsWithScenario_ClaudeOrgIDReachesContext(t *testing.T) {
	c := newGinContext(t)
	rule := &typ.Rule{Flags: typ.RuleFlags{ClaudeOrgID: "org-uuid"}}
	got := ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioClaudeCode, &typ.ScenarioConfig{},
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, nil)
	if got.ClaudeOrgID != "org-uuid" {
		t.Errorf("ClaudeOrgID = %q, want %q", got.ClaudeOrgID, "org-uuid")
	}
	if ctxVal := typ.GetRuleFlags(c.Request.Context()).ClaudeOrgID; ctxVal != "org-uuid" {
		t.Errorf("claude org id in ctx = %q, want %q", ctxVal, "org-uuid")
	}
}

func TestResolveRuleFlagsWithScenario_ExtraHeaders(t *testing.T) {
	// Rule-level headers land in the request context for the outbound
	// transport, verbatim.
	c := newGinContext(t)
	rule := &typ.Rule{Flags: typ.RuleFlags{ExtraHeaders: map[string]string{"X-Team-Tag": "research"}}}
	ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioOpenAI, &typ.ScenarioConfig{},
		protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, nil)
	got := typ.GetRuleFlags(c.Request.Context()).ExtraHeaders
	if got["X-Team-Tag"] != "research" {
		t.Errorf("ctx headers = %v, want rule extra_headers attached", got)
	}

	// Nothing configured → nothing attached.
	c = newGinContext(t)
	ResolveRuleFlagsWithScenario(c, &typ.Rule{}, typ.ScenarioOpenAI, &typ.ScenarioConfig{},
		protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, nil)
	if got := typ.GetRuleFlags(c.Request.Context()).ExtraHeaders; got != nil {
		t.Errorf("ctx headers = %v, want nil when the rule sets none", got)
	}
}

// Provider-level probes run under a synthetic rule with no flags. For a
// Claude OAuth provider that means the legacy emulation, which Anthropic no
// longer accepts, so the probe defaults to the latest native profile — and
// only there: matched rules keep the flag's off-by-default rollout, other
// providers are untouched, and an explicit overlay still wins.
func TestResolveRuleFlagsWithScenario_ProbeDefaultsClaudeCodeVersionForOAuth(t *testing.T) {
	oauthProvider := &typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{Issuer: ai.IssuerClaudeCode},
	}
	apiKeyProvider := &typ.Provider{AuthType: typ.AuthTypeAPIKey}
	synthetic := func() *typ.Rule { return &typ.Rule{UUID: ProbeSyntheticRuleUUID} }
	resolve := func(c *gin.Context, rule *typ.Rule, p *typ.Provider) typ.RuleFlags {
		return ResolveRuleFlagsWithScenario(c, rule, typ.ScenarioAnthropic, &typ.ScenarioConfig{},
			protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, p)
	}

	if got := resolve(newGinContext(t), synthetic(), oauthProvider).ClaudeCodeVersion; got != typ.ClaudeCodeVersionLatest {
		t.Errorf("synthetic probe rule + Claude OAuth provider: ClaudeCodeVersion = %q, want %q", got, typ.ClaudeCodeVersionLatest)
	}
	if got := resolve(newGinContext(t), synthetic(), apiKeyProvider).ClaudeCodeVersion; got != "" {
		t.Errorf("synthetic probe rule + API-key provider must not get a profile, got %q", got)
	}
	if got := resolve(newGinContext(t), &typ.Rule{UUID: "real-rule"}, oauthProvider).ClaudeCodeVersion; got != "" {
		t.Errorf("a matched rule without the flag must stay legacy (rollout default), got %q", got)
	}
	if got := resolve(newGinContext(t), nil, oauthProvider).ClaudeCodeVersion; got != "" {
		t.Errorf("nil rule must stay legacy, got %q", got)
	}

	// Scenario-level value still wins over the probe default.
	c := newGinContext(t)
	got := ResolveRuleFlagsWithScenario(c, synthetic(), typ.ScenarioClaudeCode,
		&typ.ScenarioConfig{Flags: typ.ScenarioFlags{ClaudeCodeVersion: typ.ClaudeCodeVersion2_1_258}},
		protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, oauthProvider)
	if got.ClaudeCodeVersion != typ.ClaudeCodeVersion2_1_258 {
		t.Errorf("scenario flag must win over the probe default, got %q", got.ClaudeCodeVersion)
	}

	// An explicit overlay "" forces the legacy emulation for a diagnostic run.
	c = newGinContext(t)
	encoded, err := typ.EncodeFlagOverlay(typ.FlagOverlay{"claude_code_version": []byte(`""`)})
	if err != nil {
		t.Fatalf("EncodeFlagOverlay: %v", err)
	}
	c.Request.Header.Set(typ.ProbeFlagsHeader, encoded)
	if got := resolve(c, synthetic(), oauthProvider).ClaudeCodeVersion; got != "" {
		t.Errorf("overlay \"\" must force legacy, got %q", got)
	}
	// The resolved value reaches the context (NewClaudeClient reads it there).
	c = newGinContext(t)
	resolve(c, synthetic(), oauthProvider)
	if got := typ.GetRuleFlags(c.Request.Context()).ClaudeCodeVersion; got != typ.ClaudeCodeVersionLatest {
		t.Errorf("ClaudeCodeVersion in ctx = %q, want %q", got, typ.ClaudeCodeVersionLatest)
	}
}
