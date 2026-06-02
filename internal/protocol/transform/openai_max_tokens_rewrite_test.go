package transform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

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

func TestOpenAIMaxTokensRewriteTransform_Name(t *testing.T) {
	tf := NewOpenAIMaxTokensRewriteTransform(true, false)
	if tf.Name() != "openai_max_tokens_rewrite" {
		t.Errorf("unexpected name: %q", tf.Name())
	}
}

func TestOpenAIMaxTokensRewriteTransform_AppliesOnOpenAIChat(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(true, false)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxTokens.Valid() {
		t.Errorf("expected MaxTokens cleared, got %v", req.MaxTokens.Value)
	}
	if !req.MaxCompletionTokens.Valid() || req.MaxCompletionTokens.Value != 1024 {
		t.Errorf("expected MaxCompletionTokens=1024, got %#v", req.MaxCompletionTokens)
	}
}

func TestOpenAIMaxTokensRewriteTransform_ReverseDirection(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(false, true)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("expected MaxCompletionTokens cleared, got %v", req.MaxCompletionTokens.Value)
	}
	if !req.MaxTokens.Valid() || req.MaxTokens.Value != 2048 {
		t.Errorf("expected MaxTokens=2048, got %#v", req.MaxTokens)
	}
}

func TestOpenAIMaxTokensRewriteTransform_BothFlagsOff_NoOp(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(false, false)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !req.MaxTokens.Valid() || req.MaxTokens.Value != 1024 {
		t.Errorf("expected MaxTokens untouched, got %#v", req.MaxTokens)
	}
	if req.MaxCompletionTokens.Valid() {
		t.Errorf("expected MaxCompletionTokens untouched (zero), got %#v", req.MaxCompletionTokens)
	}
}

func TestOpenAIMaxTokensRewriteTransform_NoOpOnAnthropicV1(t *testing.T) {
	req := &anthropic.MessageNewParams{MaxTokens: 1024}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(true, false)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("Anthropic MaxTokens unexpectedly modified: %d", req.MaxTokens)
	}
}

func TestOpenAIMaxTokensRewriteTransform_NoOpOnAnthropicBeta(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{MaxTokens: 1024}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(true, false)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("Anthropic Beta MaxTokens unexpectedly modified: %d", req.MaxTokens)
	}
}

func TestOpenAIMaxTokensRewriteTransform_NoOpOnResponses(t *testing.T) {
	req := &responses.ResponseNewParams{}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAIMaxTokensRewriteTransform(true, true)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// No assertion needed beyond "didn't panic / didn't error" — the
	// Responses struct has no max_completion_tokens duality.
}

func TestOpenAIMaxTokensRewriteTransform_NilRequest(t *testing.T) {
	ctx := &TransformContext{Request: nil}
	tf := NewOpenAIMaxTokensRewriteTransform(true, true)
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply on nil request failed: %v", err)
	}
}

// stubBaseTransform mimics BaseTransform's protocol conversion role for the
// regression test: it swaps an Anthropic-typed request for an OpenAI Chat
// one, carrying over the MaxTokens value.
type stubBaseTransform struct{}

func (stubBaseTransform) Name() string { return "stub_base" }

func (stubBaseTransform) Apply(ctx *TransformContext) error {
	if a, ok := ctx.Request.(*anthropic.MessageNewParams); ok {
		ctx.Request = &openai.ChatCompletionNewParams{
			MaxTokens: param.NewOpt(a.MaxTokens),
		}
	}
	return nil
}

// TestOpenAIMaxTokensRewriteTransform_FiresAfterBase is the regression test
// for the bug this refactor addresses: when the inbound request is Anthropic
// but the target is OpenAI Chat, the rewrite must fire on the post-base
// shape. Chaining the stub base transform ahead of the rewrite proves the
// chain ordering works correctly.
func TestOpenAIMaxTokensRewriteTransform_FiresAfterBase(t *testing.T) {
	original := &anthropic.MessageNewParams{MaxTokens: 4096}
	ctx := &TransformContext{Request: original}

	chain := NewTransformChain([]Transform{
		stubBaseTransform{},
		NewOpenAIMaxTokensRewriteTransform(true, false),
	})
	if _, err := chain.Execute(ctx); err != nil {
		t.Fatalf("chain.Execute failed: %v", err)
	}

	converted, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		t.Fatalf("expected *openai.ChatCompletionNewParams after chain, got %T", ctx.Request)
	}
	if converted.MaxTokens.Valid() {
		t.Errorf("expected MaxTokens cleared post-rewrite, got %v", converted.MaxTokens.Value)
	}
	if !converted.MaxCompletionTokens.Valid() || converted.MaxCompletionTokens.Value != 4096 {
		t.Errorf("expected MaxCompletionTokens=4096 post-rewrite, got %#v", converted.MaxCompletionTokens)
	}
}

func TestApplyMaxCompletionTokensRewrite_NilSafe(t *testing.T) {
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

// TestMaxCompletionTokensRewrite_WireSerialization verifies that after the
// rewrite the OpenAI SDK serializes "max_completion_tokens" and omits
// "max_tokens" from the JSON body.
func TestMaxCompletionTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxTokens: param.NewOpt(int64(1024)),
	}

	before := marshalTokenFields(t, req)
	if _, ok := before["max_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_tokens' in JSON, got keys: %v", jsonKeys(before))
	}

	applyMaxCompletionTokensRewrite(req)

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

// TestMaxTokensRewrite_WireSerialization verifies the inverse direction.
func TestMaxTokensRewrite_WireSerialization(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: param.NewOpt(int64(2048)),
	}

	before := marshalTokenFields(t, req)
	if _, ok := before["max_completion_tokens"]; !ok {
		t.Errorf("before rewrite: expected 'max_completion_tokens' in JSON, got keys: %v", jsonKeys(before))
	}

	applyMaxTokensRewrite(req)

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
