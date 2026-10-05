package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/mcp/local"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func managementFixture(t *testing.T) (*Handler, *gin.Engine, *config.Config) {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, cfg.SetToolConfig(config.ToolTypeMCPRuntime, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}}))
	h := NewHandler(cfg)
	t.Cleanup(h.runtime.Close)
	r := gin.New()
	r.GET("/config", h.GetMCPRuntimeConfig)
	r.PUT("/config", h.SetMCPRuntimeConfig)
	r.POST("/sources", h.CreateSource)
	r.PATCH("/sources/:source_id", h.PatchSource)
	r.DELETE("/sources/:source_id", h.DeleteSource)
	r.GET("/catalog", h.GetCatalog)
	r.POST("/call", h.CallTool)
	r.PUT("/profiles/:profile_id", h.SaveClientProfile)
	r.DELETE("/profiles/:profile_id", h.DeleteClientProfile)
	r.POST("/transport/:client_name", h.transportHandler.HandleMCP)
	r.GET("/transport/:client_name", h.transportHandler.HandleMCP)
	r.DELETE("/transport/:client_name", h.transportHandler.HandleMCP)
	t.Cleanup(h.transportHandler.ResetAll)
	return h, r, cfg
}

func managementRequest(t *testing.T, r http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, status, w.Code, w.Body.String())
	return w
}

func TestManagementCRUDPersistsAndPreservesOmittedFields(t *testing.T) {
	h, r, cfg := managementFixture(t)
	require.False(t, h.IsMCPEnabled())
	managementRequest(t, r, "GET", "/config", "", 200)
	managementRequest(t, r, "POST", "/sources", `{"id":"remote","transport":"http","endpoint":"https://example.test/mcp","headers":{"Authorization":"retained"},"usage":{"client":true,"gateway":true}}`, 201)
	managementRequest(t, r, "POST", "/sources", `{"id":"remote","transport":"http","endpoint":"https://example.test"}`, 400)
	managementRequest(t, r, "PATCH", "/sources/remote", `{"id":"changed"}`, 400)
	managementRequest(t, r, "PATCH", "/sources/missing", `{"enabled":false}`, 404)
	managementRequest(t, r, "PATCH", "/sources/remote", `{"name":"New name","tool_policies":{"echo":{"enabled":false}}}`, 200)
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":["remote"],"tools":["tingly_box_mcp__remote__echo"]}`, 200)
	managementRequest(t, r, "PUT", "/config", `{"request_timeout":12,"strip_disabled_mcp_tools":false}`, 200)
	got := cfg.GetMCPRuntimeConfig()
	require.Len(t, got.Sources, 1)
	require.Equal(t, "retained", got.Sources[0].Headers["Authorization"])
	require.NotNil(t, got.Sources[0].ToolPolicies["echo"].Enabled)
	require.Len(t, got.ClientProfiles, 1)
	require.Equal(t, 12, got.RequestTimeout)
	reloaded, err := config.NewConfig(config.WithConfigDir(cfg.ConfigDir))
	require.NoError(t, err)
	require.Equal(t, got, reloaded.GetMCPRuntimeConfig())
	managementRequest(t, r, "POST", "/call", `{"source_id":"remote","tool_name":"echo"}`, 403)
	managementRequest(t, r, "DELETE", "/sources/remote", "", 200)
	require.Empty(t, cfg.GetMCPRuntimeConfig().ClientProfiles[0].Sources)
	require.Empty(t, cfg.GetMCPRuntimeConfig().ClientProfiles[0].Tools)
	managementRequest(t, r, "DELETE", "/profiles/reader", "", 200)
	require.False(t, h.runtime.ClientExists("tb"))
	require.True(t, cfg.GetMCPRuntimeConfig().ClientProfilesConfigured)
}

func TestSourceValidationAcceptsEnvironmentURLsAndRejectsBrokenConfig(t *testing.T) {
	_, r, cfg := managementFixture(t)
	t.Setenv("MCP_TEST_ENDPOINT", "https://example.test/mcp")
	managementRequest(t, r, "POST", "/sources", `{"id":"env","transport":"sse","endpoint":"${MCP_TEST_ENDPOINT}","env":{"MCP_TEST_ENDPOINT":"${MCP_TEST_ENDPOINT}"}}`, 201)
	require.Equal(t, "${MCP_TEST_ENDPOINT}", cfg.GetMCPRuntimeConfig().Sources[0].Endpoint)
	for _, body := range []string{`{"id":"bad__id","transport":"http","endpoint":"https://example.test"}`, `{"id":"bad","transport":"http","endpoint":"file:///tmp/x"}`, `{"id":"bad","transport":"http","endpoint":"${MCP_MISSING_TEST_VAR}"}`, `{"id":"bad","transport":"stdio"}`, `{"id":"bad","transport":"other"}`} {
		managementRequest(t, r, "POST", "/sources", body, 400)
	}
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":["missing"],"tools":["*"]}`, 400)
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":null,"tools":null}`, 200)
	require.Empty(t, cfg.GetMCPRuntimeConfig().ClientProfiles[0].Sources)
}

func TestDownstreamBridgePreservesSchemasResultsAndRevokesAccess(t *testing.T) {
	h, r, cfg := managementFixture(t)
	upstream := sdk.NewServer(&sdk.Implementation{Name: "remote", Version: "1"}, nil)
	upstream.AddTool(&sdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "failure"}, &sdk.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"}, &sdk.ResourceLink{Name: "report", URI: "https://example.test/report"}}, StructuredContent: map[string]any{"retained": true}}, nil
	})
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return upstream }, &sdk.StreamableHTTPOptions{JSONResponse: true}))
	defer remote.Close()
	body, _ := json.Marshal(typ.MCPSourceConfig{ID: "remote", Transport: "http", Endpoint: remote.URL, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}})
	managementRequest(t, r, "POST", "/sources", string(body), 201)
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":["remote"],"tools":["*"]}`, 200)
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP, true))
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "reader", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: gateway.URL + "/transport/reader"}, nil)
	require.NoError(t, err)
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
	require.NotNil(t, tools.Tools[0].OutputSchema)
	require.True(t, tools.Tools[0].Annotations.ReadOnlyHint)
	name := tools.Tools[0].Name
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 3)
	require.Equal(t, map[string]any{"retained": true}, result.StructuredContent)
	w := managementRequest(t, r, "POST", "/call", `{"source_id":"remote","tool_name":"echo","arguments":{}}`, 200)
	require.Contains(t, w.Body.String(), `"isError":true`)
	require.Contains(t, w.Body.String(), `"resource_link"`)
	// Keep an already-created adapter, then revoke its profile: calls must check current policy.
	adapter := local.NewMCPRuntimeAdapterForClient(h.runtime, "reader")
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":[],"tools":[]}`, 200)
	_, err = adapter.CallTool(ctx, name, map[string]any{})
	require.ErrorContains(t, err, "not allowed")
	result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: map[string]any{}})
	if err == nil {
		require.True(t, result.IsError)
	}
	tools, err = session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, tools.Tools)
	managementRequest(t, r, "POST", "/transport/typo", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, 404)
}
