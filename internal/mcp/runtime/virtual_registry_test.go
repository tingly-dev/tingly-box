package runtime

import (
	"context"
	"testing"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestVirtualRegistry_RegisterAndGet(t *testing.T) {
	reg := coretool.NewVirtualToolRegistry()
	vt := coretool.VirtualTool{
		Name:        "advisor",
		Description: "strategic guidance",
		Handler: func(ctx context.Context, call coretool.ToolCall) (coretool.ToolResult, error) {
			return coretool.TextToolResult("ok"), nil
		},
		Visibility: typ.ToolVisibilityServer, // Server tool
	}
	reg.Register(vt)

	got, ok := reg.Get("advisor")
	if !ok || got.Name != "advisor" {
		t.Fatal("expected advisor virtual tool")
	}

	if got.Visibility == typ.ToolVisibilityClient {
		t.Fatal("expected advisor to be a server tool (Visibility=server)")
	}

	virtualTools := reg.ListVirtualTools()
	if len(virtualTools) != 1 || virtualTools[0].Name != "advisor" {
		t.Fatalf("expected 1 tool, got %d", len(virtualTools))
	}
}

func TestVirtualRegistry_ListVirtualTools(t *testing.T) {
	reg := coretool.NewVirtualToolRegistry()

	// Register server tool (not exposed to clients)
	serverTool := coretool.VirtualTool{
		Name:        "adviser",
		Description: "Server-side advisor",
		Handler:     nil,
		Visibility:  typ.ToolVisibilityServer,
	}
	reg.Register(serverTool)

	// Register client tool (exposed to clients)
	clientTool := coretool.VirtualTool{
		Name:        "websearch",
		Description: "Client-side web search",
		Handler:     nil,
		Visibility:  typ.ToolVisibilityClient,
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
		if vt.Visibility == typ.ToolVisibilityClient {
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
