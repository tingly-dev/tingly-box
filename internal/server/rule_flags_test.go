package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// marshalTokenFields serializes only the token-related fields of a
// ChatCompletionNewParams into a JSON map so tests can assert on wire presence.
func marshalTokenFields(t *testing.T, req *openai.ChatCompletionNewParams) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	return m
}

func TestResolveRuleFlags_NilRule(t *testing.T) {
	got := resolveRuleFlags(nil)
	// Zero value: all fields default
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
		},
	}
	got := resolveRuleFlags(rule)
	if !got.CursorCompat || !got.SkipUsage || !got.UseMaxCompletionTokens {
		t.Errorf("bool flags lost: %#v", got)
	}
	if got.CustomUserAgent != "MyApp/1.0" {
		t.Errorf("CustomUserAgent = %q, want %q", got.CustomUserAgent, "MyApp/1.0")
	}
}

func TestApplyMaxCompletionTokensRewrite_NilSafe(t *testing.T) {
	// Must not panic on nil
	applyMaxCompletionTokensRewrite(nil)
}

func TestApplyMaxCompletionTokensRewrite_MovesMaxTokens(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}
	applyMaxCompletionTokensRewrite(req)

	if req.MaxTokens.Valid() {
		t.Errorf("expected MaxTokens cleared after rewrite, got %v", req.MaxTokens.Value)
	}
	if !req.MaxCompletionTokens.Valid() {
		t.Fatalf("expected MaxCompletionTokens populated after rewrite")
	}
	if req.MaxCompletionTokens.Value != 1024 {
		t.Errorf("MaxCompletionTokens = %d, want 1024", req.MaxCompletionTokens.Value)
	}
}

func TestApplyMaxCompletionTokensRewrite_NoMaxTokensNoOp(t *testing.T) {
	// When neither field is set the rewrite is a no-op (avoid emitting an
	// explicit max_completion_tokens=0 which most providers reject).
	req := &openai.ChatCompletionNewParams{}
	applyMaxCompletionTokensRewrite(req)
	if req.MaxTokens.Valid() {
		t.Errorf("MaxTokens unexpectedly valid: %#v", req.MaxTokens)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("MaxCompletionTokens unexpectedly valid: %#v", req.MaxCompletionTokens)
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

func TestApplyMaxCompletionTokensRewrite_PreservesExistingMaxCompletionTokens(t *testing.T) {
	// If both fields are already present (caller supplied them), prefer the
	// MaxTokens migration but document the surprising case.
	req := &openai.ChatCompletionNewParams{
		MaxTokens:           param.NewOpt(int64(512)),
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}
	applyMaxCompletionTokensRewrite(req)

	if req.MaxTokens.Valid() {
		t.Errorf("expected MaxTokens cleared, got %v", req.MaxTokens.Value)
	}
	// Current behavior: MaxTokens overwrites MaxCompletionTokens. Track this
	// in tests so future refactors notice.
	if req.MaxCompletionTokens.Value != 512 {
		t.Errorf("MaxCompletionTokens = %d, want 512 (rewrite overrides existing)", req.MaxCompletionTokens.Value)
	}
}

// --- applyMaxTokensRewrite ---

func TestApplyMaxTokensRewrite_NilSafe(t *testing.T) {
	applyMaxTokensRewrite(nil)
}

func TestApplyMaxTokensRewrite_MovesMaxCompletionTokens(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}
	applyMaxTokensRewrite(req)

	if req.MaxCompletionTokens.Valid() {
		t.Errorf("expected MaxCompletionTokens cleared after rewrite, got %v", req.MaxCompletionTokens.Value)
	}
	if !req.MaxTokens.Valid() {
		t.Fatalf("expected MaxTokens populated after rewrite")
	}
	if req.MaxTokens.Value != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", req.MaxTokens.Value)
	}
}

func TestApplyMaxTokensRewrite_NoMaxCompletionTokensNoOp(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	applyMaxTokensRewrite(req)
	if req.MaxTokens.Valid() {
		t.Errorf("MaxTokens unexpectedly valid: %#v", req.MaxTokens)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("MaxCompletionTokens unexpectedly valid: %#v", req.MaxCompletionTokens)
	}
}

// --- JSON wire serialization: prove the rewrites emit the correct field on the wire ---

// TestMaxCompletionTokensRewrite_WireSerialization verifies that after
// applyMaxCompletionTokensRewrite the OpenAI SDK serializes
// "max_completion_tokens" and omits "max_tokens" from the JSON body.
// This guards against silent regression where a struct-level change doesn't
// propagate to the actual HTTP request body.
func TestMaxCompletionTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}

	// Before rewrite: max_tokens on wire, max_completion_tokens absent.
	before := marshalTokenFields(t, req)
	if _, ok := before["max_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_tokens' in JSON, got keys: %v", jsonKeys(before))
	}
	if _, ok := before["max_completion_tokens"]; ok {
		t.Errorf("before rewrite: unexpected 'max_completion_tokens' in JSON")
	}

	applyMaxCompletionTokensRewrite(req)

	// After rewrite: max_completion_tokens on wire, max_tokens absent.
	after := marshalTokenFields(t, req)
	if _, ok := after["max_completion_tokens"]; !ok {
		t.Errorf("after rewrite: expected 'max_completion_tokens' in JSON, got keys: %v", jsonKeys(after))
	}
	if _, ok := after["max_tokens"]; ok {
		t.Errorf("after rewrite: unexpected 'max_tokens' still present in JSON")
	}
	if v, _ := after["max_completion_tokens"].(float64); int(v) != 1024 {
		t.Errorf("after rewrite: max_completion_tokens = %v, want 1024", after["max_completion_tokens"])
	}
}

// TestMaxTokensRewrite_WireSerialization verifies the inverse: after
// applyMaxTokensRewrite the SDK emits "max_tokens" and omits
// "max_completion_tokens".
func TestMaxTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}

	// Before rewrite: max_completion_tokens on wire.
	before := marshalTokenFields(t, req)
	if _, ok := before["max_completion_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_completion_tokens' in JSON, got keys: %v", jsonKeys(before))
	}

	applyMaxTokensRewrite(req)

	// After rewrite: max_tokens on wire, max_completion_tokens absent.
	after := marshalTokenFields(t, req)
	if _, ok := after["max_tokens"]; !ok {
		t.Errorf("after rewrite: expected 'max_tokens' in JSON, got keys: %v", jsonKeys(after))
	}
	if _, ok := after["max_completion_tokens"]; ok {
		t.Errorf("after rewrite: unexpected 'max_completion_tokens' still present in JSON")
	}
	if v, _ := after["max_tokens"].(float64); int(v) != 2048 {
		t.Errorf("after rewrite: max_tokens = %v, want 2048", after["max_tokens"])
	}
}

func jsonKeys(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
