package transform

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMCPToolInjectionTransform_AdvisorNativeGuard(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{{
			ID:        "advisor",
			Name:      "advisor",
			Transport: "advisor",
			Enabled:   typ.BoolPtr(true),
			Tools:     []string{"advisor"},
		}},
	}
	rt := runtime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	t.Cleanup(rt.Close)
	rt.VirtualRegistry().Register(coretool.VirtualTool{
		Name:        "advisor",
		Description: "server-side advisor",
		InputSchema: mcp.ToolInputSchema{Type: "object"},
		Visibility:  typ.ToolVisibilityServer,
	})

	tr := NewMCPToolInjectionTransform(rt)

	t.Run("native_advisor_history_filters_advisor", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: "m",
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
			},
		}
		ctx := &protocoltransform.TransformContext{
			Context:          context.Background(),
			Request:          req,
			HasNativeAdvisor: true,
		}

		if err := tr.Apply(ctx); err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if hasToolName(req.Tools, advisorInjectedToolName) {
			t.Fatalf("advisor tool should be filtered when native advisor is present")
		}
		if len(req.Messages) != 1 {
			t.Fatalf("advisor system prompt should not be appended, got %d messages", len(req.Messages))
		}
	})

	t.Run("claude_code_without_native_advisor_keeps_advisor", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: "m",
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
			},
		}
		ctx := &protocoltransform.TransformContext{
			Context: context.Background(),
			Request: req,
		}

		if err := tr.Apply(ctx); err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if !hasToolName(req.Tools, advisorInjectedToolName) {
			t.Fatalf("advisor tool should be injected without native advisor history")
		}
		if len(req.Messages) < 2 {
			t.Fatalf("advisor system prompt should be appended for non-claude providers")
		}
	})

	t.Run("anthropic_v1_native_advisor_filters_advisor", func(t *testing.T) {
		req := &anthropic.MessageNewParams{Model: "m"}
		ctx := &protocoltransform.TransformContext{Context: context.Background(), Request: req, HasNativeAdvisor: true}

		if err := tr.Apply(ctx); err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if hasAnthropicV1ToolName(req.Tools, advisorInjectedToolName) {
			t.Fatalf("advisor tool should be filtered when native advisor is present")
		}
	})

	t.Run("anthropic_beta_native_advisor_filters_advisor", func(t *testing.T) {
		req := &anthropic.BetaMessageNewParams{Model: "m"}
		ctx := &protocoltransform.TransformContext{Context: context.Background(), Request: req, HasNativeAdvisor: true}

		if err := tr.Apply(ctx); err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		if hasAnthropicBetaToolName(req.Tools, advisorInjectedToolName) {
			t.Fatalf("advisor tool should be filtered when native advisor is present")
		}
	})
}

func hasToolName(tools []openai.ChatCompletionToolUnionParam, name string) bool {
	for _, tool := range tools {
		fn := tool.GetFunction()
		if fn != nil && fn.Name == name {
			return true
		}
	}
	return false
}

func hasAnthropicV1ToolName(tools []anthropic.ToolUnionParam, name string) bool {
	for _, tool := range tools {
		if tool.OfTool != nil && tool.OfTool.Name == name {
			return true
		}
	}
	return false
}

func hasAnthropicBetaToolName(tools []anthropic.BetaToolUnionParam, name string) bool {
	for _, tool := range tools {
		if tool.OfTool != nil && tool.OfTool.Name == name {
			return true
		}
	}
	return false
}
