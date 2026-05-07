package transform

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
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
	rt.VirtualRegistry().Register(runtime.VirtualTool{
		Name:         "advisor",
		Description:  "server-side advisor",
		InputSchema:  mcp.ToolInputSchema{Type: "object"},
		IsClientTool: false,
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
			ProviderType:     "claude_code",
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
			Context:      context.Background(),
			Request:      req,
			ProviderType: "claude_code",
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
