package protocoltest

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const remoteToolName = "tingly_box_mcp__remote__echo"

func remoteMCPHarnessCases() []serverToolCase {
	var cases []serverToolCase
	for _, transport := range []string{"http", "sse"} {
		group := pairCases("TestMCPRemoteNotInjected", "remote_mcp_not_injected", crossPairs(toolLoopSources, toolLoopTargets), bothStreamModes, func(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
			return remoteMCPNotInjectedCase(t, source, target, streaming, transport)
		})
		for i := range group {
			group[i].sub += "/transport=" + transport
		}
		cases = append(cases, group...)
	}
	return cases
}

// A connected MCP is an ordinary MCP server for agents; the gateway never
// injects its tools into a model request, even with a stored gateway usage,
// and never runs a call to one. The upstream is a real SDK MCP server reached
// over HTTP/SSE; the model is a fixture that calls the tool anyway.
func remoteMCPNotInjectedCase(t flagTB, source, target protocol.APIType, streaming bool, transport string) ([]string, string) {
	var calls atomic.Int32
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "harness-remote", Version: "1"}, nil)
	server.AddTool(&sdkmcp.Tool{Name: "echo", Description: "Remote echo", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []string{"q"}}}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: ownedToolResultText}}}, nil
	})
	var handler http.Handler = sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	if transport == "sse" {
		handler = sdkmcp.NewSSEHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
	}
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{ID: "remote", Transport: transport, Endpoint: remote.URL, Tools: []string{"echo"}, Usage: &typ.MCPToolUsage{Client: true, Gateway: true}}, {ID: "webtools", Enabled: typ.BoolPtr(false), Command: "tingly-box", Transport: "stdio"}}}
	env := newCaseEnv(t, NewTestEnvOptionWithMCPConfig(cfg))
	scenario := OwnedToolScenario()
	scenario.Name = "remote_mcp_not_injected"
	for format, builder := range scenario.MockResponses {
		original := builder
		if original.NonStreamFor != nil {
			builder.NonStreamFor = func(request []byte) (int, []byte) {
				code, body := original.NonStreamFor(request)
				return code, bytes.ReplaceAll(body, []byte(OwnedToolWireName), []byte(remoteToolName))
			}
		}
		if original.StreamFor != nil {
			builder.StreamFor = func(request []byte) []string {
				events := original.StreamFor(request)
				out := make([]string, len(events))
				for i, event := range events {
					out[i] = strings.ReplaceAll(event, OwnedToolWireName, remoteToolName)
				}
				return out
			}
		}
		scenario.MockResponses[format] = builder
	}
	path, body, _ := routedRequest(env, source, target, scenario, streaming)
	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != 200 {
		failures = append(failures, fmt.Sprintf("status=%d", status))
	}
	upstream := string(requireLastRequest(t, env, target, "remote MCP not injected").Body)
	if strings.Contains(upstream, remoteToolName) {
		failures = append(failures, "connected MCP tool injected into the model request")
	}
	if calls.Load() != 0 {
		failures = append(failures, fmt.Sprintf("gateway ran a connected MCP tool: calls=%d", calls.Load()))
	}
	if env.VirtualCallCount() != 1 {
		failures = append(failures, fmt.Sprintf("model calls=%d, want 1 (no gateway tool round)", env.VirtualCallCount()))
	}
	return failures, "client:\n" + raw + "\nupstream:\n" + upstream
}
