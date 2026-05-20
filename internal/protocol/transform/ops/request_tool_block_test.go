package ops

import (
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

func TestApplyToolBlockOpenAIChat(t *testing.T) {
	t.Run("removes only blocked tools", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Tools: []openai.ChatCompletionToolUnionParam{
				chatFnTool("keep_a"),
				chatFnTool("block_me"),
				chatFnTool("keep_b"),
			},
		}
		ApplyToolBlockOpenAIChat(req, blockSet("block_me"))
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
		ApplyToolBlockOpenAIChat(req, blockSet())
		if len(req.Tools) != 1 {
			t.Fatalf("expected tools untouched, got %d", len(req.Tools))
		}
	})

	t.Run("blocking all tools clears the slice to nil", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Tools: []openai.ChatCompletionToolUnionParam{chatFnTool("a"), chatFnTool("b")},
		}
		ApplyToolBlockOpenAIChat(req, blockSet("a", "b"))
		if req.Tools != nil {
			t.Fatalf("expected nil tools, got %#v", req.Tools)
		}
	})

	t.Run("nil request does not panic", func(t *testing.T) {
		ApplyToolBlockOpenAIChat(nil, blockSet("a"))
	})
}

func TestApplyToolBlockResponses(t *testing.T) {
	req := &responses.ResponseNewParams{
		Tools: []responses.ToolUnionParam{
			{OfFunction: &responses.FunctionToolParam{Name: "keep"}},
			{OfFunction: &responses.FunctionToolParam{Name: "drop"}},
		},
	}
	ApplyToolBlockResponses(req, blockSet("drop"))
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
	ApplyToolBlockAnthropic(req, blockSet("drop"))
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
	ApplyToolBlockAnthropicBeta(req, blockSet("drop"))
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
	ApplyToolBlockGoogle(req, blockSet("drop"))
	decls := req.Config.Tools[0].FunctionDeclarations
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(decls))
	}
	if decls[0].Name != "keep" {
		t.Fatalf("wrong declaration kept: %q", decls[0].Name)
	}

	t.Run("nil config does not panic", func(t *testing.T) {
		ApplyToolBlockGoogle(&protocol.GoogleRequest{}, blockSet("x"))
	})
}
