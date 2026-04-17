package runtime

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestVirtualRegistry_RegisterAndGet(t *testing.T) {
	reg := NewVirtualToolRegistry()
	vt := VirtualTool{
		Name:        "advisor",
		Description: "strategic guidance",
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("ok")}}, nil
		},
	}
	reg.Register(vt)

	got, ok := reg.Get("advisor")
	if !ok || got.Name != "advisor" {
		t.Fatal("expected advisor virtual tool")
	}

	mcpTools := reg.List()
	if len(mcpTools) != 1 || mcpTools[0].Name != "advisor" {
		t.Fatalf("expected 1 tool, got %d", len(mcpTools))
	}
}
