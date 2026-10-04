package protocoltest

import (
	"context"
	"encoding/json"
	"fmt"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

func routingMCPHarnessCases() []serverToolCase {
	var cases []serverToolCase
	for _, transport := range []string{"http", "sse"} {
		for _, state := range []string{"client_only", "server_only", "dual", "none", "revoked", "disabled_tool", "excluded_tool"} {
			cases = append(cases, serverToolCase{test: "TestMCPRoutingExposure", scenario: "mcp_routing_exposure", source: protocol.APIType("mcp"), target: protocol.APIType("mcp"), sub: "transport=" + transport + "/policy=" + state, run: func(t flagTB) ([]string, string) {
				return routingMCPCase(t, transport, state)
			}})
		}
	}
	cases = append(cases, serverToolCase{test: "TestMCPRoutingExposure", scenario: "mcp_routing_exposure", source: protocol.APIType("mcp"), target: protocol.APIType("mcp"), sub: "advisor_context_required", run: routingMCPAdvisorCase})
	return cases
}

// Compare the management graph with an authenticated SDK client using the actual
// gateway. A hidden or ungranted tool must never reach the upstream handler.
func routingMCPCase(t flagTB, transport, state string) ([]string, string) {
	var calls atomic.Int32
	upstream := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "routing-harness", Version: "1"}, nil)
	upstream.AddTool(&sdkmcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "route passed"}}, StructuredContent: map[string]any{"retained": true}}, nil
	})
	var handler http.Handler = sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return upstream }, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	if transport == "sse" {
		handler = sdkmcp.NewSSEHandler(func(*http.Request) *sdkmcp.Server { return upstream }, nil)
	}
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	clientUse, serverUse := state != "server_only" && state != "none", state != "client_only" && state != "none"
	source := typ.MCPSourceConfig{ID: "remote", Origin: "external", Transport: transport, Endpoint: remote.URL, Usage: &typ.MCPToolUsage{Client: clientUse, Gateway: serverUse}}
	grant := []string{"*"}
	if state == "revoked" {
		grant = []string{}
	}
	if state == "disabled_tool" {
		source.ToolPolicies = map[string]typ.MCPToolPolicy{"echo": {Enabled: typ.BoolPtr(false)}}
	}
	if state == "excluded_tool" {
		source.Tools = []string{"another"}
	}
	wantClient := clientUse && state != "revoked" && state != "disabled_tool" && state != "excluded_tool"
	wantServer := serverUse && state != "disabled_tool" && state != "excluded_tool"
	env := newCaseEnv(t, NewTestEnvOptionWithMCPConfig(&typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{source, {ID: "webtools", Enabled: typ.BoolPtr(false), Transport: "stdio", Command: "tingly-box"}}, ClientProfilesConfigured: true, ClientProfiles: []typ.MCPClientProfile{{ID: "reader", Sources: []string{"remote"}, Tools: grant}}}))
	token := env.appConfig.GetGlobalConfig().GetUserToken()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.GatewayURL()+"/api/v1/mcp/routing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{err.Error()}, ""
	}
	defer resp.Body.Close()
	var body struct {
		Success bool                       `json:"success"`
		Routing mcpruntime.RoutingSnapshot `json:"routing"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return []string{err.Error()}, ""
	}
	var failures []string
	if resp.StatusCode != 200 || !body.Success {
		failures = append(failures, "routing request failed")
	}
	count := func(routes []mcpruntime.RouteSource) int {
		n := 0
		for _, r := range routes {
			n += len(r.Tools)
		}
		return n
	}
	if (count(body.Routing.ServerTools) > 0) != wantServer {
		failures = append(failures, "server-tool graph differs from effective policy")
	}
	if len(body.Routing.Clients) != 1 {
		return append(failures, "expected explicit client profile"), ""
	}
	if (count(body.Routing.Clients[0].Sources) > 0) != wantClient {
		failures = append(failures, "client graph differs from grants")
	}
	sdk := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "harness-reader", Version: "1"}, nil)
	session, err := sdk.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: env.GatewayURL() + "/api/v1/mcp/reader", HTTPClient: &http.Client{Transport: routingAuth{token: token}}}, nil)
	if err != nil {
		return append(failures, "gateway initialize: "+err.Error()), ""
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return append(failures, "tools/list: "+err.Error()), ""
	}
	if (len(listed.Tools) > 0) != wantClient {
		failures = append(failures, "SDK tools/list differs from graph")
	}
	result, callErr := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: remoteToolName, Arguments: map[string]any{}})
	if wantClient {
		if callErr != nil || result == nil || result.IsError || calls.Load() != 1 {
			failures = append(failures, "authorized SDK call failed")
		}
		if result != nil {
			raw, _ := json.Marshal(result.StructuredContent)
			if string(raw) != "{\"retained\":true}" {
				failures = append(failures, "structured result lost")
			}
		}
	} else if calls.Load() != 0 || callErr == nil && (result == nil || !result.IsError) {
		failures = append(failures, "blocked tool reached upstream or falsely succeeded")
	}
	return failures, fmt.Sprintf("transport=%s policy=%s client=%v server=%v SDK-listed=%d upstream-calls=%d", transport, state, wantClient, wantServer, len(listed.Tools), calls.Load())
}

type routingAuth struct{ token string }

func (a routingAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(copy)
}

// Even a legacy dual-use setting cannot expose the context-dependent Advisor
// through a generic client endpoint. Its model-loop route remains available.
func routingMCPAdvisorCase(t flagTB) ([]string, string) {
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{ID: "advisor", Enabled: typ.BoolPtr(true), Transport: "advisor", Advisor: &typ.AdvisorConfig{ProviderUUID: "consultation", Model: "review-model"}, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}, {ID: "webtools", Enabled: typ.BoolPtr(false), Command: "tingly-box", Transport: "stdio"}}, ClientProfilesConfigured: true, ClientProfiles: []typ.MCPClientProfile{{ID: "reader", Sources: []string{"*"}, Tools: []string{"*"}}}}
	env := newCaseEnv(t, NewTestEnvOptionWithMCPConfig(cfg))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	token := env.appConfig.GetGlobalConfig().GetUserToken()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.GatewayURL()+"/api/v1/mcp/routing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{err.Error()}, ""
	}
	defer response.Body.Close()
	var body struct {
		Routing mcpruntime.RoutingSnapshot `json:"routing"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		return []string{err.Error()}, ""
	}
	var failures []string
	if len(body.Routing.Clients) != 1 || len(body.Routing.Clients[0].Sources) != 0 {
		failures = append(failures, "Advisor appeared in ordinary client route")
	}
	if len(body.Routing.ServerTools) != 1 || len(body.Routing.ServerTools[0].Tools) != 1 {
		failures = append(failures, "Advisor model-loop route missing")
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "context-check", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: env.GatewayURL() + "/api/v1/mcp/reader", HTTPClient: &http.Client{Transport: routingAuth{token: token}}}, nil)
	if err != nil {
		return append(failures, err.Error()), ""
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		return append(failures, err.Error()), ""
	}
	if len(tools.Tools) != 0 {
		failures = append(failures, "Advisor exposed through SDK tools/list")
	}
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "tingly_box_mcp__builtin__advisor", Arguments: map[string]any{}})
	if err == nil && (result == nil || !result.IsError) {
		failures = append(failures, "context-free Advisor call falsely succeeded")
	}
	return failures, "Advisor available only to model loop; generic client has zero exposed tools"
}
