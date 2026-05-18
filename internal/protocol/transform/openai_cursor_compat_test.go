package transform

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// richContentUserMessage builds an OpenAI Chat user message with array-of-parts
// content — the shape that Cursor-compat is meant to flatten into a plain
// string.
func richContentUserMessage(t *testing.T) openai.ChatCompletionMessageParamUnion {
	t.Helper()
	msgMap := map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "text", "text": "hello"},
			map[string]any{"type": "text", "text": "world"},
		},
	}
	msgBytes, err := json.Marshal(msgMap)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var msg openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return msg
}

// messageContent JSON-roundtrips a message and returns its "content" field so
// tests can assert on the flattened wire shape regardless of which variant
// (OfUser, OfSystem, …) carries the data.
func messageContent(t *testing.T, msg openai.ChatCompletionMessageParamUnion) any {
	t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return m["content"]
}

func TestOpenAICursorCompatTransform_Name(t *testing.T) {
	tf := NewOpenAICursorCompatTransform()
	if tf.Name() != "openai_cursor_compat" {
		t.Errorf("unexpected name: %q", tf.Name())
	}
}

func TestOpenAICursorCompatTransform_FlattensOpenAIChatContent(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			richContentUserMessage(t),
		},
	}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAICursorCompatTransform()
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	got := messageContent(t, req.Messages[0])
	str, ok := got.(string)
	if !ok {
		t.Fatalf("expected flattened string content, got %T (%v)", got, got)
	}
	if str != "hello\nworld" {
		t.Errorf("unexpected flattened content: %q", str)
	}
}

func TestOpenAICursorCompatTransform_NoOpOnAnthropicV1(t *testing.T) {
	req := &anthropic.MessageNewParams{MaxTokens: 1024}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAICursorCompatTransform()
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("Anthropic v1 request unexpectedly modified")
	}
}

func TestOpenAICursorCompatTransform_NoOpOnAnthropicBeta(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{MaxTokens: 1024}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAICursorCompatTransform()
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("Anthropic beta request unexpectedly modified")
	}
}

func TestOpenAICursorCompatTransform_NoOpOnResponses(t *testing.T) {
	req := &responses.ResponseNewParams{}
	ctx := &TransformContext{Request: req}

	tf := NewOpenAICursorCompatTransform()
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// Responses has no flatten op — verify the transform didn't crash and
	// didn't try to swap the request type.
	if _, ok := ctx.Request.(*responses.ResponseNewParams); !ok {
		t.Errorf("Responses request type unexpectedly swapped")
	}
}

func TestOpenAICursorCompatTransform_NilRequest(t *testing.T) {
	ctx := &TransformContext{Request: nil}
	tf := NewOpenAICursorCompatTransform()
	if err := tf.Apply(ctx); err != nil {
		t.Fatalf("Apply on nil request failed: %v", err)
	}
}

// TestOpenAICursorCompatTransform_FiresBeforeBase verifies the intended chain
// ordering: the transform is meant to flatten inbound OpenAI Chat content
// before BaseTransform converts it to a target shape. We chain
// cursor-compat *before* a stub base that "converts" Chat → Chat verbatim,
// and assert flattening happened on the inbound shape.
func TestOpenAICursorCompatTransform_FiresBeforeBase(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			richContentUserMessage(t),
		},
	}
	ctx := &TransformContext{Request: req}

	chain := NewTransformChain([]Transform{
		NewOpenAICursorCompatTransform(),
		stubBaseTransform{},
	})
	if _, err := chain.Execute(ctx); err != nil {
		t.Fatalf("chain.Execute failed: %v", err)
	}

	got := messageContent(t, req.Messages[0])
	str, ok := got.(string)
	if !ok {
		t.Fatalf("expected flattened string content, got %T (%v)", got, got)
	}
	if str != "hello\nworld" {
		t.Errorf("unexpected flattened content: %q", str)
	}
}

// TestOpenAICursorCompatTransform_NoOpOnAnthropicInbound documents the design
// boundary: cursor_compat only acts on OpenAI Chat *inbound*. When inbound is
// Anthropic and base converts it to OpenAI Chat downstream, cursor_compat
// (placed pre-base) sees Anthropic and skips — matching .design/rule-flags.md
// §12 "cursor 归一化只对 OpenAI 入站才有意义".
func TestOpenAICursorCompatTransform_NoOpOnAnthropicInbound(t *testing.T) {
	original := &anthropic.MessageNewParams{MaxTokens: 4096}
	ctx := &TransformContext{Request: original}

	chain := NewTransformChain([]Transform{
		NewOpenAICursorCompatTransform(),
		stubBaseTransform{},
	})
	if _, err := chain.Execute(ctx); err != nil {
		t.Fatalf("chain.Execute failed: %v", err)
	}

	// After stub base swap, request is OpenAI Chat shape — but no flattening
	// happened (no messages to flatten in this fixture, and importantly no
	// panic / no type-confused mutation on the Anthropic shape).
	if _, ok := ctx.Request.(*openai.ChatCompletionNewParams); !ok {
		t.Fatalf("expected OpenAI Chat after stub base, got %T", ctx.Request)
	}
}
