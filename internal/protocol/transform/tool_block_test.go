package transform

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"google.golang.org/genai"
)

func blockSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func chatFnTool(name string) openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: name})
}

func TestToolBlockTransform_Name(t *testing.T) {
	if got := NewToolBlockTransform([]string{"x"}).Name(); got != "tool_block" {
		t.Errorf("unexpected name: %q", got)
	}
}

func TestToolBlockTransform_FiltersOpenAIChat(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Tools: []openai.ChatCompletionToolUnionParam{
			chatFnTool("keep"),
			chatFnTool("drop"),
		},
	}
	ctx := &TransformContext{Request: req}

	if err := NewToolBlockTransform([]string{"drop"}).Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].OfFunction.Function.Name != "keep" {
		t.Fatalf("unexpected tools after block: %#v", req.Tools)
	}
}

func TestToolBlockTransform_FiltersAnthropicV1(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "keep"}},
			{OfTool: &anthropic.ToolParam{Name: "drop"}},
		},
	}
	ctx := &TransformContext{Request: req}

	if err := NewToolBlockTransform([]string{"drop"}).Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].OfTool.Name != "keep" {
		t.Fatalf("unexpected tools after block: %#v", req.Tools)
	}
}

func TestToolBlockTransform_EmptyNamesNoOp(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Tools: []openai.ChatCompletionToolUnionParam{chatFnTool("a")},
	}
	ctx := &TransformContext{Request: req}
	if err := NewToolBlockTransform(nil).Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected tools untouched, got %d", len(req.Tools))
	}
}

func TestToolBlockTransform_NilRequest(t *testing.T) {
	ctx := &TransformContext{Request: nil}
	if err := NewToolBlockTransform([]string{"x"}).Apply(ctx); err != nil {
		t.Fatalf("Apply on nil request failed: %v", err)
	}
}

// TestToolBlockTransform_FiresBeforeBase verifies the intended pre-Base
// ordering: tool blocking acts on the inbound (Anthropic) shape by the names
// the client sent, before the stub base converts it to OpenAI Chat. The blocked
// tool must be gone from the converted result.
func TestToolBlockTransform_FiresBeforeBase(t *testing.T) {
	req := &anthropic.MessageNewParams{
		MaxTokens: 1024,
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "keep"}},
			{OfTool: &anthropic.ToolParam{Name: "drop"}},
		},
	}
	ctx := &TransformContext{Request: req}

	chain := NewTransformChain([]Transform{
		NewToolBlockTransform([]string{"drop"}),
		toolCarryingStubBase{},
	})
	if _, err := chain.Execute(ctx); err != nil {
		t.Fatalf("chain.Execute failed: %v", err)
	}

	converted, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		t.Fatalf("expected *openai.ChatCompletionNewParams after chain, got %T", ctx.Request)
	}
	if len(converted.Tools) != 1 || converted.Tools[0].OfFunction.Function.Name != "keep" {
		t.Fatalf("blocked tool leaked past base: %#v", converted.Tools)
	}
}

// TestToolBlockTransform_BlockAllOmitsToolsOnWire guards the omitzero behavior:
// blocking every tool resets the slice to nil so the wire body has no "tools"
// key rather than an empty array.
func TestToolBlockTransform_BlockAllOmitsToolsOnWire(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: "gpt-4",
		Tools: []openai.ChatCompletionToolUnionParam{chatFnTool("a"), chatFnTool("b")},
	}
	ctx := &TransformContext{Request: req}
	if err := NewToolBlockTransform([]string{"a", "b"}).Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, present := m["tools"]; present {
		t.Fatalf("expected tools key to be omitted, got: %s", raw)
	}
}

// toolCarryingStubBase mimics BaseTransform: it converts an Anthropic request to
// OpenAI Chat, carrying over the (already-filtered) tool list by name.
type toolCarryingStubBase struct{}

func (toolCarryingStubBase) Name() string { return "stub_base" }

func (toolCarryingStubBase) Apply(ctx *TransformContext) error {
	a, ok := ctx.Request.(*anthropic.MessageNewParams)
	if !ok {
		return nil
	}
	out := &openai.ChatCompletionNewParams{}
	for _, tool := range a.Tools {
		if tool.OfTool != nil {
			out.Tools = append(out.Tools, chatFnTool(tool.OfTool.Name))
		}
	}
	ctx.Request = out
	return nil
}

func TestApplyToolBlockOpenAIChat(t *testing.T) {
	t.Run("removes only blocked tools", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Tools: []openai.ChatCompletionToolUnionParam{
				chatFnTool("keep_a"),
				chatFnTool("block_me"),
				chatFnTool("keep_b"),
			},
		}
		applyToolBlockOpenAIChat(req, blockSet("block_me"))
		if len(req.Tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(req.Tools))
		}
		for _, tool := range req.Tools {
			if openAIChatToolName(tool) == "block_me" {
				t.Fatalf("blocked tool still present")
			}
		}
	})

	t.Run("empty block set is a no-op", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Tools: []openai.ChatCompletionToolUnionParam{chatFnTool("a")},
		}
		applyToolBlockOpenAIChat(req, blockSet())
		if len(req.Tools) != 1 {
			t.Fatalf("expected tools untouched, got %d", len(req.Tools))
		}
	})

	t.Run("blocking all tools clears slice to nil", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Tools: []openai.ChatCompletionToolUnionParam{chatFnTool("a"), chatFnTool("b")},
		}
		applyToolBlockOpenAIChat(req, blockSet("a", "b"))
		if req.Tools != nil {
			t.Fatalf("expected nil tools, got %#v", req.Tools)
		}
	})

	t.Run("nil request does not panic", func(t *testing.T) {
		applyToolBlockOpenAIChat(nil, blockSet("a"))
	})
}

func TestApplyToolBlockResponses(t *testing.T) {
	req := &responses.ResponseNewParams{
		Tools: []responses.ToolUnionParam{
			{OfFunction: &responses.FunctionToolParam{Name: "keep"}},
			{OfFunction: &responses.FunctionToolParam{Name: "drop"}},
		},
	}
	applyToolBlockResponses(req, blockSet("drop"))
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].OfFunction.Name != "keep" {
		t.Fatalf("wrong tool kept: %q", req.Tools[0].OfFunction.Name)
	}
}

func TestApplyToolBlockAnthropic(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "keep"}},
			{OfTool: &anthropic.ToolParam{Name: "drop"}},
		},
	}
	applyToolBlockAnthropic(req, blockSet("drop"))
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].OfTool.Name != "keep" {
		t.Fatalf("wrong tool kept: %q", req.Tools[0].OfTool.Name)
	}
}

func TestApplyToolBlockAnthropicBeta(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Tools: []anthropic.BetaToolUnionParam{
			{OfTool: &anthropic.BetaToolParam{Name: "keep"}},
			{OfTool: &anthropic.BetaToolParam{Name: "drop"}},
		},
	}
	applyToolBlockAnthropicBeta(req, blockSet("drop"))
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].OfTool.Name != "keep" {
		t.Fatalf("wrong tool kept: %q", req.Tools[0].OfTool.Name)
	}
}

func TestApplyToolBlockGoogle(t *testing.T) {
	req := &protocol.GoogleRequest{
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{
					FunctionDeclarations: []*genai.FunctionDeclaration{
						{Name: "keep"},
						{Name: "drop"},
					},
				},
			},
		},
	}
	applyToolBlockGoogle(req, blockSet("drop"))
	decls := req.Config.Tools[0].FunctionDeclarations
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(decls))
	}
	if decls[0].Name != "keep" {
		t.Fatalf("wrong declaration kept: %q", decls[0].Name)
	}

	t.Run("nil config does not panic", func(t *testing.T) {
		applyToolBlockGoogle(&protocol.GoogleRequest{}, blockSet("x"))
	})
}
