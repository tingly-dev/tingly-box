package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

// marshalTokenFields serializes a ChatCompletionNewParams to a JSON map so
// tests can assert on wire presence/absence of fields. This proves the SDK's
// omitzero tag actually drops a cleared field instead of emitting it as zero.
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

func jsonKeys(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func TestApplyMaxCompletionTokensRewrite_NilSafe(t *testing.T) {
	ApplyMaxCompletionTokensRewrite(nil)
}

func TestApplyMaxCompletionTokensRewrite_MovesMaxTokens(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}
	ApplyMaxCompletionTokensRewrite(req)

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
	ApplyMaxCompletionTokensRewrite(req)
	if req.MaxTokens.Valid() {
		t.Errorf("MaxTokens unexpectedly valid: %#v", req.MaxTokens)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("MaxCompletionTokens unexpectedly valid: %#v", req.MaxCompletionTokens)
	}
}

func TestApplyMaxCompletionTokensRewrite_PreservesExistingMaxCompletionTokens(t *testing.T) {
	// If both fields are already present (caller supplied them), prefer the
	// MaxTokens migration but document the surprising case.
	req := &openai.ChatCompletionNewParams{
		MaxTokens:           param.NewOpt(int64(512)),
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}
	ApplyMaxCompletionTokensRewrite(req)

	if req.MaxTokens.Valid() {
		t.Errorf("expected MaxTokens cleared, got %v", req.MaxTokens.Value)
	}
	if req.MaxCompletionTokens.Value != 512 {
		t.Errorf("MaxCompletionTokens = %d, want 512 (rewrite overrides existing)", req.MaxCompletionTokens.Value)
	}
}

func TestApplyMaxTokensRewrite_NilSafe(t *testing.T) {
	ApplyMaxTokensRewrite(nil)
}

func TestApplyMaxTokensRewrite_MovesMaxCompletionTokens(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}
	ApplyMaxTokensRewrite(req)

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
	ApplyMaxTokensRewrite(req)
	if req.MaxTokens.Valid() {
		t.Errorf("MaxTokens unexpectedly valid: %#v", req.MaxTokens)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("MaxCompletionTokens unexpectedly valid: %#v", req.MaxCompletionTokens)
	}
}

// TestMaxCompletionTokensRewrite_WireSerialization verifies that after
// ApplyMaxCompletionTokensRewrite the OpenAI SDK serializes
// "max_completion_tokens" and omits "max_tokens" from the JSON body.
// This guards against silent regression where a struct-level change doesn't
// propagate to the actual HTTP request body.
func TestMaxCompletionTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}

	before := marshalTokenFields(t, req)
	if _, ok := before["max_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_tokens' in JSON, got keys: %v", jsonKeys(before))
	}
	if _, ok := before["max_completion_tokens"]; ok {
		t.Errorf("before rewrite: unexpected 'max_completion_tokens' in JSON")
	}

	ApplyMaxCompletionTokensRewrite(req)

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
// ApplyMaxTokensRewrite the SDK emits "max_tokens" and omits
// "max_completion_tokens".
func TestMaxTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}

	before := marshalTokenFields(t, req)
	if _, ok := before["max_completion_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_completion_tokens' in JSON, got keys: %v", jsonKeys(before))
	}

	ApplyMaxTokensRewrite(req)

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
