package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestResolveRuleFlags_NilRule(t *testing.T) {
	got := resolveRuleFlags(nil)
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
	got := resolveRuleFlags(rule)
	if !got.CursorCompat || !got.SkipUsage || !got.UseMaxCompletionTokens || !got.UseMaxTokens {
		t.Errorf("bool flags lost: %#v", got)
	}
	if got.CustomUserAgent != "MyApp/1.0" {
		t.Errorf("CustomUserAgent = %q, want %q", got.CustomUserAgent, "MyApp/1.0")
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
	tf, ok := got[0].(*OpenAIMaxTokensRewriteTransform)
	if !ok {
		t.Fatalf("expected *OpenAIMaxTokensRewriteTransform, got %T", got[0])
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
	tf := got[0].(*OpenAIMaxTokensRewriteTransform)
	if tf.UseMaxCompletionTokens || !tf.UseMaxTokens {
		t.Errorf("flag values not propagated: %#v", tf)
	}
}

func TestRuleExtraTransforms_OtherFlagsAlone_NoTransform(t *testing.T) {
	// Flags that have their own injection paths (UA via context, skip_usage
	// via Extra) shouldn't surface here.
	got := ruleExtraTransforms(typ.RuleFlags{
		CursorCompat:    true,
		SkipUsage:       true,
		CustomUserAgent: "Foo/1.0",
	})
	if got != nil {
		t.Errorf("expected nil, got %d transforms", len(got))
	}
}
