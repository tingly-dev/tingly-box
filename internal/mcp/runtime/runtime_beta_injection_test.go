package runtime

import (
	"context"
	"testing"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestListServerToolsForAnthropicBetaInjectionUsesBetaTypesDirectly(t *testing.T) {
	runtime := NewRuntime(func() *typ.MCPRuntimeConfig { return &typ.MCPRuntimeConfig{} })
	t.Cleanup(runtime.Close)
	runtime.VirtualRegistry().Register(coretool.VirtualTool{
		Name:        "lookup",
		Description: "Look up a value",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
			"required":   []string{"query"},
		},
		Visibility: typ.ToolVisibilityServer,
	})

	tools := runtime.ListServerToolsForAnthropicBetaInjection(context.Background())
	if len(tools) != 1 || tools[0].OfTool == nil {
		t.Fatalf("Beta tools = %#v", tools)
	}
	tool := tools[0].OfTool
	if tool.Name != NormalizeToolName("builtin", "lookup") || tool.Description.Value != "Look up a value" {
		t.Fatalf("Beta tool = %#v", tool)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "query" {
		t.Fatalf("Beta input schema = %#v", tool.InputSchema)
	}
}
