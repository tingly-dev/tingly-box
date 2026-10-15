package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type remoteRuntimeE2EArgs struct {
	Q string `json:"q"`
}

func remoteRuntimeE2ETool(ctx context.Context, req *sdkmcp.CallToolRequest, args remoteRuntimeE2EArgs) (*sdkmcp.CallToolResult, any, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: `{"ok":true,"q":"` + args.Q + `"}`},
		},
	}, nil, nil
}

func newRemoteRuntimeE2EServer(t *testing.T, transport string) *httptest.Server {
	t.Helper()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "remote-runtime-e2e", Version: "v0.0.1"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: "echo", Description: "echo q"}, remoteRuntimeE2ETool)

	switch transport {
	case "http":
		return httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
			return server
		}, &sdkmcp.StreamableHTTPOptions{JSONResponse: true}))
	case "sse":
		return httptest.NewServer(sdkmcp.NewSSEHandler(func(*http.Request) *sdkmcp.Server {
			return server
		}, nil))
	default:
		t.Fatalf("unsupported transport %q", transport)
		return nil
	}
}

func TestMCPRemoteHTTPRuntimeE2E(t *testing.T) {
	runMCPRemoteRuntimeE2E(t, "http")
}

func TestMCPRemoteSSERuntimeE2E(t *testing.T) {
	runMCPRemoteRuntimeE2E(t, "sse")
}

func runMCPRemoteRuntimeE2E(t *testing.T, transport string) {
	t.Helper()

	remote := newRemoteRuntimeE2EServer(t, transport)
	t.Cleanup(remote.Close)

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{
		ID:         "remote-" + transport,
		Name:       "Remote " + transport,
		Transport:  transport,
		Endpoint:   remote.URL,
		Enabled:    typ.BoolPtr(true),
		Tools:      []string{"*"},
		Visibility: typ.ToolVisibilityClient,
	}}})

	ctx := context.Background()
	tools, err := s.mcpRuntime.ListSourceTools(ctx)
	require.NoError(t, err)

	sourceTools := tools["remote-"+transport]
	require.Len(t, sourceTools, 1)
	require.Equal(t, "echo", sourceTools[0].Name)
	normalizedName := runtime.NormalizeToolName("remote-"+transport, "echo")

	result, err := s.mcpRuntime.CallTool(ctx, normalizedName, `{"q":"hello"}`)
	require.NoError(t, err)

	require.Len(t, result.Contents, 1)
	require.Equal(t, `{"ok":true,"q":"hello"}`, result.FirstText())

	enabled := s.mcpRuntime.ListEnabledServerToolNames(ctx)
	_, ok := enabled[normalizedName]
	require.True(t, ok, "client tools remain known to the MCP declaration filter")
	for _, injected := range s.mcpRuntime.ListServerToolsForInjection(ctx) {
		require.NotEqual(t, normalizedName, injected.OfFunction.Function.Name, "client-only tools must not be injected into model calls")
	}

	s.mcpRuntime.Close()
}
