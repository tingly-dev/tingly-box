package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func catalogRemote(t *testing.T, transport, label, token string) *httptest.Server {
	t.Helper()
	s := sdk.NewServer(&sdk.Implementation{Name: label, Version: "1"}, nil)
	s.AddTool(&sdk.Tool{Name: "echo", Description: label, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}, OutputSchema: map[string]any{"type": "object"}, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		return &sdk.CallToolResult{IsError: args["q"] == "fail", Content: []sdk.Content{&sdk.TextContent{Text: label}, &sdk.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"}}, StructuredContent: map[string]any{"label": label}}, nil
	})
	var handler http.Handler = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s }, &sdk.StreamableHTTPOptions{JSONResponse: true})
	if transport == "sse" {
		handler = sdk.NewSSEHandler(func(*http.Request) *sdk.Server { return s }, nil)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	return remote
}

func TestCatalogPolicyAndStructuredResults(t *testing.T) {
	for _, transport := range []string{"http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			remote := catalogRemote(t, transport, "first", "test-token")
			cfg := &typ.MCPRuntimeConfig{RequestTimeout: 2, Sources: []typ.MCPSourceConfig{{ID: "remote", Transport: transport, Endpoint: remote.URL, Headers: map[string]string{"Authorization": "test-token"}, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}}}
			r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
			defer r.Close()
			ctx := context.Background()
			status, err := r.DiscoverSource(ctx, "remote")
			require.NoError(t, err)
			require.Equal(t, "connected", status.State)
			require.Len(t, status.Tools, 1)
			tool := status.Tools[0]
			require.NotEmpty(t, tool.OutputSchema)
			require.Contains(t, string(tool.Annotations), "readOnlyHint")
			require.True(t, tool.Usage.Client)
			require.True(t, tool.Usage.Gateway)
			require.Len(t, r.ListServerToolsForInjection(ctx), 1)
			result, err := r.CallTool(ctx, tool.NormalizedName, `{"q":"fail"}`)
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Equal(t, "first", result.FirstText())
			require.Len(t, result.Contents, 2)
			require.Equal(t, "AQID", result.Contents[1].Data)
			require.Equal(t, map[string]any{"label": "first"}, result.StructuredContent)
			previous := r.activeSources["remote"]
			cfg.Sources[0].ToolPolicies = map[string]typ.MCPToolPolicy{"echo": {Enabled: typ.BoolPtr(false)}}
			r.Reconcile(ctx)
			require.Same(t, previous, r.activeSources["remote"], "policy edit must keep connection")
			require.Empty(t, r.ListServerToolsForInjection(ctx))
			_, err = r.CallTool(ctx, tool.NormalizedName, `{}`)
			require.Error(t, err)
			cfg.Sources[0].ToolPolicies = nil
			second := catalogRemote(t, transport, "second", "changed-token")
			cfg.Sources[0].Endpoint = second.URL
			cfg.Sources[0].Headers = map[string]string{"Authorization": "changed-token"}
			r.Reconcile(ctx)
			require.Error(t, previous.Connect(ctx), "a replaced source must never resurrect its old connection")
			result, err = r.CallTool(ctx, tool.NormalizedName, `{}`)
			require.NoError(t, err)
			require.Equal(t, "second", result.FirstText())
			status, err = r.ReconnectSource(ctx, "remote")
			require.NoError(t, err)
			require.Equal(t, "connected", status.State)
			cfg.Sources = nil
			r.Reconcile(ctx)
			_, err = r.CallTool(ctx, tool.NormalizedName, `{}`)
			require.Error(t, err)
		})
	}
}

func TestCatalogPartialFailureAndClientGrants(t *testing.T) {
	remote := catalogRemote(t, "http", "working", "")
	cfg := &typ.MCPRuntimeConfig{RequestTimeout: 1, Sources: []typ.MCPSourceConfig{{ID: "working", Transport: "http", Endpoint: remote.URL}, {ID: "failed", Transport: "http", Endpoint: "http://127.0.0.1:1"}}, ClientProfiles: []typ.MCPClientProfile{{ID: "reader", Sources: []string{"working"}, Tools: []string{NormalizeToolName("working", "echo")}}}, ClientProfilesConfigured: true}
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	start := time.Now()
	catalog := r.Catalog(context.Background(), "")
	require.Less(t, time.Since(start), 3*time.Second)
	require.Len(t, catalog, 2)
	require.Equal(t, "connected", catalog[0].State)
	require.Equal(t, "error", catalog[1].State)
	require.NotEmpty(t, catalog[1].Error)
	require.True(t, r.ClientAllows("reader", "working", NormalizeToolName("working", "echo")))
	require.False(t, r.ClientAllows("reader", "failed", NormalizeToolName("failed", "echo")))
	require.False(t, r.ClientAllows("typo", "working", NormalizeToolName("working", "echo")))
	cfg.ClientProfiles = nil
	require.False(t, r.ClientExists("tb"), "deleting the last explicit profile must not restore aggregate access")
}

func TestStdioCatalogResultAndHotUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real MCP subprocess")
	}
	command := buildFakeMCP(t)
	cfg := &typ.MCPRuntimeConfig{RequestTimeout: 3, Sources: []typ.MCPSourceConfig{{ID: "stdio", Transport: "stdio", Command: command, Env: map[string]string{"FAKE_MCP_LABEL": "first"}, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}}}
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	status, err := r.DiscoverSource(context.Background(), "stdio")
	require.NoError(t, err)
	require.Len(t, status.Tools, 1)
	source := r.activeSources["stdio"].(*StdioToolSource)
	originalPID := source.killCmd.Load().Process.Pid
	result, err := r.CallTool(context.Background(), status.Tools[0].NormalizedName, `{}`)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, "first", result.FirstText())
	require.Equal(t, map[string]any{"label": "first"}, result.StructuredContent)
	cfg.Sources[0].Env["FAKE_MCP_LABEL"] = "second"
	r.Reconcile(context.Background())
	result, err = r.CallTool(context.Background(), status.Tools[0].NormalizedName, `{}`)
	require.NoError(t, err)
	require.Equal(t, "second", result.FirstText())
	require.NotEqual(t, originalPID, r.activeSources["stdio"].(*StdioToolSource).killCmd.Load().Process.Pid)
	cfg.Sources[0].Enabled = typ.BoolPtr(false)
	r.Reconcile(context.Background())
	require.Empty(t, r.activeSources)
	_, err = r.CallTool(context.Background(), status.Tools[0].NormalizedName, `{}`)
	require.Error(t, err)
}
