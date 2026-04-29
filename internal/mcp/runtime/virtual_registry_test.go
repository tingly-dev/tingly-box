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
		IsClientTool: false, // Server tool
	}
	reg.Register(vt)

	got, ok := reg.Get("advisor")
	if !ok || got.Name != "advisor" {
		t.Fatal("expected advisor virtual tool")
	}

	if got.IsClientTool {
		t.Fatal("expected advisor to be a server tool (IsClientTool=false)")
	}

	mcpTools := reg.List()
	if len(mcpTools) != 1 || mcpTools[0].Name != "advisor" {
		t.Fatalf("expected 1 tool, got %d", len(mcpTools))
	}
}

func TestVirtualRegistry_ListVirtualTools(t *testing.T) {
	reg := NewVirtualToolRegistry()

	// Register server tool (not exposed to clients)
	serverTool := VirtualTool{
		Name:        "adviser",
		Description: "Server-side advisor",
		Handler:     nil,
		IsClientTool: false,
	}
	reg.Register(serverTool)

	// Register client tool (exposed to clients)
	clientTool := VirtualTool{
		Name:        "websearch",
		Description: "Client-side web search",
		Handler:     nil,
		IsClientTool: true,
	}
	reg.Register(clientTool)

	virtualTools := reg.ListVirtualTools()
	if len(virtualTools) != 2 {
		t.Fatalf("expected 2 virtual tools, got %d", len(virtualTools))
	}

	// Count client and server tools
	clientCount := 0
	serverCount := 0
	for _, vt := range virtualTools {
		if vt.IsClientTool {
			clientCount++
		} else {
			serverCount++
		}
	}

	if clientCount != 1 {
		t.Errorf("expected 1 client tool, got %d", clientCount)
	}
	if serverCount != 1 {
		t.Errorf("expected 1 server tool, got %d", serverCount)
	}
}
