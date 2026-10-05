package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRoutingAndClientProbeTraverseTheAuthenticatedGateway(t *testing.T) {
	h, r, cfg := managementFixture(t)
	r.GET("/routing", h.GetRouting)
	r.POST("/probe/:profile_id", h.ProbeClient)
	// Use the production endpoint, not an adapter shortcut. Its auth middleware
	// checks that the loopback SDK connection has the actual user's token.
	r.POST("/api/v1/mcp/:client_name", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+cfg.GetUserToken() {
			c.AbortWithStatus(401)
			return
		}
		h.transportHandler.HandleMCP(c)
	})
	r.GET("/api/v1/mcp/:client_name", h.transportHandler.HandleMCP)
	r.DELETE("/api/v1/mcp/:client_name", h.transportHandler.HandleMCP)
	upstream := sdk.NewServer(&sdk.Implementation{Name: "probe", Version: "1"}, nil)
	var calls atomic.Int32
	upstream.AddTool(&sdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		calls.Add(1)
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "preserved failure"}}, StructuredContent: map[string]any{"retained": true}}, nil
	})
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return upstream }, &sdk.StreamableHTTPOptions{JSONResponse: true}))
	defer remote.Close()
	body, _ := json.Marshal(typ.MCPSourceConfig{ID: "remote", Origin: "external", Transport: "http", Endpoint: remote.URL, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}})
	managementRequest(t, r, "POST", "/sources", string(body), 201)
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":["remote"],"tools":["*"]}`, 200)
	managementRequest(t, r, "GET", "/routing", "", 200)
	managementRequest(t, r, "POST", "/probe/reader", `{}`, 403)
	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP, true))
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	u, _ := url.Parse(gateway.URL)
	port, _ := strconv.Atoi(u.Port())
	require.NoError(t, cfg.SetServerPort(port))
	name := "tingly_box_mcp__remote__echo"
	listed := managementRequest(t, r, "POST", "/probe/reader", `{}`, 200)
	require.Contains(t, listed.Body.String(), name)
	require.Equal(t, int32(0), calls.Load(), "discovery probe must not execute a tool")
	result := managementRequest(t, r, "POST", "/probe/reader", `{"tool_name":"`+name+`","arguments":{}}`, 200)
	require.Contains(t, result.Body.String(), `"success":true`)
	require.Contains(t, result.Body.String(), `"isError":true`)
	require.Contains(t, result.Body.String(), `"retained":true`)
	require.Equal(t, int32(1), calls.Load())
	managementRequest(t, r, "PUT", "/profiles/reader", `{"id":"reader","sources":[],"tools":[]}`, 200)
	denied := managementRequest(t, r, "POST", "/probe/reader", `{"tool_name":"`+name+`"}`, 200)
	require.Contains(t, denied.Body.String(), `"success":false`)
	require.Equal(t, int32(1), calls.Load(), "revoked tool must not reach upstream")
	managementRequest(t, r, "POST", "/probe/missing", `{}`, 404)
}
