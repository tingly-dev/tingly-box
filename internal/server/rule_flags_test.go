package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestRuleExtraTransforms_NoFlags(t *testing.T) {
	got := ruleExtraTransforms(typ.RuleFlags{})
	if got != nil {
		t.Errorf("expected nil for zero-value flags, got %d transforms", len(got))
	}
}

func TestRuleExtraTransforms_UseMaxCompletionTokens(t *testing.T) {
	got := ruleExtraTransforms(typ.RuleFlags{UseMaxCompletionTokens: true})
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

func TestRuleExtraTransforms_UseMaxTokens(t *testing.T) {
	got := ruleExtraTransforms(typ.RuleFlags{UseMaxTokens: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 transform, got %d", len(got))
	}
	tf := got[0].(*transform.OpenAIMaxTokensRewriteTransform)
	if tf.UseMaxCompletionTokens || !tf.UseMaxTokens {
		t.Errorf("flag values not propagated: %#v", tf)
	}
}

func TestRuleExtraTransforms_ThinkingEffort(t *testing.T) {
	got := ruleExtraTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortHigh})
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

func TestRuleExtraTransforms_ThinkingEffortEmpty_NoTransform(t *testing.T) {
	got := ruleExtraTransforms(typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortDefault})
	if got != nil {
		t.Errorf("empty thinking effort should add no transform, got %d", len(got))
	}
}

func TestRuleExtraTransforms_CursorCompatAlone_NoTransform(t *testing.T) {
	// CursorCompat is a pre-Base flag — it must not surface in the post-Base
	// extras list. This is the safety net for the rule-flag-to-transform
	// migration: if anyone wires cursor_compat into ruleExtraTransforms by
	// mistake, this test goes red.
	got := ruleExtraTransforms(typ.RuleFlags{
		CursorCompat:    true,
		SkipUsage:       true,
		CustomUserAgent: "Foo/1.0",
	})
	if got != nil {
		t.Errorf("expected nil, got %d transforms", len(got))
	}
}
