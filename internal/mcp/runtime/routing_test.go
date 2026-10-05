package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRoutingMatchesActualExposureAndKeepsOriginsIndependent(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "routes", Version: "1"}, nil)
	for _, name := range []string{"shared", "client_only", "server_only", "disabled", "excluded"} {
		server.AddTool(&sdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	}
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{JSONResponse: true}))
	defer remote.Close()
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{
		{ID: "remote", Origin: "external", Transport: "http", Endpoint: remote.URL + "?token=secret-query", Headers: map[string]string{"Authorization": "secret-header"}, Env: map[string]string{"TOKEN": "secret-env"}, Tools: []string{"shared", "client_only", "server_only", "disabled"}, Usage: &typ.MCPToolUsage{Client: true}, ToolPolicies: map[string]typ.MCPToolPolicy{
			"shared": {Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}, "server_only": {Usage: &typ.MCPToolUsage{Gateway: true}}, "disabled": {Enabled: typ.BoolPtr(false)},
		}},
		{ID: "advisor", Origin: "builtin", Transport: "advisor", Advisor: &typ.AdvisorConfig{ProviderUUID: "provider", Model: "model"}, Usage: &typ.MCPToolUsage{Gateway: true}},
	}, ClientProfilesConfigured: true, ClientProfiles: []typ.MCPClientProfile{
		{ID: "reader", Sources: []string{"remote"}, Tools: []string{NormalizeToolName("remote", "shared")}},
		{ID: "empty", Sources: []string{}, Tools: []string{}},
		{ID: "off", Enabled: typ.BoolPtr(false), Sources: []string{"*"}, Tools: []string{"*"}},
	}}
	rt := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer rt.Close()
	rt.VirtualRegistry().Register(coretool.VirtualTool{Name: "advisor", Visibility: typ.ToolVisibilityServer, InputSchema: map[string]any{"type": "object"}})
	ctx := context.Background()
	snapshot := rt.Routing(ctx)
	require.Len(t, snapshot.Sources, 2)
	require.Equal(t, "external", snapshot.Sources[0].Origin)
	require.Equal(t, "builtin", snapshot.Sources[1].Origin)
	require.Equal(t, "advisor", snapshot.Sources[1].Processing)
	require.Len(t, snapshot.Clients[0].Sources, 1)
	require.Equal(t, []string{NormalizeToolName("remote", "shared")}, routeToolNames(snapshot.Clients[0].Sources))
	require.Empty(t, snapshot.Clients[1].Sources)
	require.Empty(t, snapshot.Clients[2].Sources)
	injected := rt.ListServerToolsForInjection(ctx)
	var names []string
	for _, tool := range injected {
		names = append(names, tool.GetFunction().Name)
	}
	require.ElementsMatch(t, names, routeToolNames(snapshot.ServerTools), "graph must match actual server tool injection")
	clientTools, err := rt.ListClientSourceToolsForMCP(ctx)
	require.NoError(t, err)
	var clientNames []string
	for sourceID, tools := range clientTools {
		for _, tool := range tools {
			if rt.ClientAllows("reader", sourceID, tool.NormalizedName) {
				clientNames = append(clientNames, tool.NormalizedName)
			}
		}
	}
	require.ElementsMatch(t, clientNames, routeToolNames(snapshot.Clients[0].Sources), "graph must match actual bridge grants")
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	for _, secret := range []string{"secret-query", "secret-header", "secret-env", "Authorization"} {
		require.NotContains(t, string(raw), secret)
	}
	// Revocation changes the graph immediately, without a source reconnect.
	cfg.ClientProfiles[0].Tools = []string{}
	require.Empty(t, rt.Routing(ctx).Clients[0].Sources)
}
func TestRoutingPreservesFailedBranchesAndLegacyEndpoint(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{RequestTimeout: 1, Sources: []typ.MCPSourceConfig{{ID: "missing", Transport: "http", Endpoint: "http://user:secret-password@127.0.0.1:1?token=secret-query", Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}}}
	rt := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer rt.Close()
	snapshot := rt.Routing(context.Background())
	require.Len(t, snapshot.Clients, 1)
	require.Equal(t, "/api/v1/mcp/tb", snapshot.Clients[0].Endpoint)
	require.True(t, snapshot.Clients[0].Legacy)
	require.Len(t, snapshot.Clients[0].Sources, 1)
	require.Equal(t, "error", snapshot.Clients[0].Sources[0].State)
	require.Empty(t, snapshot.Clients[0].Sources[0].Tools)
	require.Len(t, snapshot.ServerTools, 1)
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-password")
	require.NotContains(t, string(raw), "secret-query")
	cfg.ClientProfilesConfigured = true
	cfg.ClientProfiles = []typ.MCPClientProfile{{ID: "reader", Sources: []string{"missing"}, Tools: []string{NormalizeToolName("other", "echo")}}}
	require.Empty(t, rt.Routing(context.Background()).Clients[0].Sources, "unrelated tool grants must not claim a failed source is reachable")
	cfg.ClientProfiles = nil
	require.Empty(t, rt.Routing(context.Background()).Clients, "deleting last profile must not restore legacy exposure")
}
func routeToolNames(sources []RouteSource) []string {
	names := []string{}
	for _, source := range sources {
		for _, tool := range source.Tools {
			names = append(names, tool.NormalizedName)
		}
	}
	return names
}

func TestRoutingAdvisorCannotGrantOrdinaryClientExecution(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{ID: "advisor", Transport: "advisor", Advisor: &typ.AdvisorConfig{ProviderUUID: "provider", Model: "model"}, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}, ToolPolicies: map[string]typ.MCPToolPolicy{"advisor": {Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}}}}, ClientProfilesConfigured: true, ClientProfiles: []typ.MCPClientProfile{{ID: "reader", Sources: []string{"*"}, Tools: []string{"*"}}}}
	rt := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer rt.Close()
	rt.VirtualRegistry().Register(coretool.VirtualTool{Name: "advisor", Visibility: typ.ToolVisibilityServer, InputSchema: map[string]any{"type": "object"}})
	snapshot := rt.Routing(context.Background())
	require.Empty(t, snapshot.Clients[0].Sources)
	actual, err := rt.ListClientSourceToolsForMCP(context.Background())
	require.NoError(t, err)
	require.Empty(t, actual)
	require.Len(t, snapshot.ServerTools, 1)
	require.Len(t, rt.ListServerToolsForInjection(context.Background()), 1)
}
