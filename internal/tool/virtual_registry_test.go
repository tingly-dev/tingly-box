package tool

import (
	"context"
	"testing"
)

func TestVirtualRegistry_RegisterAndGet(t *testing.T) {
	reg := NewVirtualToolRegistry()
	vt := VirtualTool{
		Name:        "advisor",
		Description: "strategic guidance",
		Handler: func(ctx context.Context, call ToolCall) (ToolResult, error) {
			return TextToolResult("ok"), nil
		},
		Visibility: ToolVisibilityServer,
	}
	reg.Register(vt)

	got, ok := reg.Get("advisor")
	if !ok || got.Name != "advisor" {
		t.Fatal("expected advisor virtual tool")
	}

	if got.Visibility == ToolVisibilityClient {
		t.Fatal("expected advisor to be a server tool (Visibility=server)")
	}

	virtualTools := reg.ListVirtualTools()
	if len(virtualTools) != 1 || virtualTools[0].Name != "advisor" {
		t.Fatalf("expected 1 tool, got %d", len(virtualTools))
	}
}

func TestVirtualRegistry_ListVirtualTools(t *testing.T) {
	reg := NewVirtualToolRegistry()

	// Register server tool (not exposed to clients)
	serverTool := VirtualTool{
		Name:        "adviser",
		Description: "Server-side advisor",
		Handler:     nil,
		Visibility:  ToolVisibilityServer,
	}
	reg.Register(serverTool)

	// Register client tool (exposed to clients)
	clientTool := VirtualTool{
		Name:        "websearch",
		Description: "Client-side web search",
		Handler:     nil,
		Visibility:  ToolVisibilityClient,
	}
	reg.Register(clientTool)

	virtualTools := reg.ListVirtualTools()
	if len(virtualTools) != 2 {
		t.Fatalf("expected 2 virtual tools, got %d", len(virtualTools))
	}

	clientCount := 0
	serverCount := 0
	for _, vt := range virtualTools {
		if vt.Visibility == ToolVisibilityClient {
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
