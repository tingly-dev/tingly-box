package protocoltest

import (
	"bytes"
	"context"
	"encoding/json"
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
		for _, failure := range []bool{false, true} {
			name, scenario := "TestMCPRemoteOwnedToolLoop", "remote_mcp_loop"
			if failure {
				name, scenario = "TestMCPRemoteToolError", "remote_mcp_error"
			}
			group := pairCases(name, scenario, crossPairs(toolLoopSources, toolLoopTargets), bothStreamModes, func(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
				return remoteMCPLoopCase(t, source, target, streaming, transport, failure)
			})
			for i := range group {
				group[i].sub += "/transport=" + transport
			}
			cases = append(cases, group...)
		}
	}
	return cases
}

// The upstream is a real SDK MCP server reached over HTTP/SSE. Only the model
// is a fixture; injection, ownership, dispatch, result conversion and the second
// model request all traverse production code.
func remoteMCPLoopCase(t flagTB, source, target protocol.APIType, streaming bool, transport string, failure bool) ([]string, string) {
	var calls atomic.Int32
	var received atomic.Bool
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "harness-remote", Version: "1"}, nil)
	server.AddTool(&sdkmcp.Tool{Name: "echo", Description: "Remote echo", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []string{"q"}}}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		received.Store(args["q"] == "x")
		text := ownedToolResultText
		if failure {
			text = ownedToolErrorText
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}}, IsError: failure, StructuredContent: map[string]any{"remote_structured_marker": "preserved"}}, nil
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
	scenario.Name = "remote_mcp_loop"
	for format, builder := range scenario.MockResponses {
		original := builder
		if original.NonStreamFor != nil {
			builder.NonStreamFor = func(request []byte) (int, []byte) {
				code, body := original.NonStreamFor(bytes.ReplaceAll(request, []byte(remoteToolName), []byte(OwnedToolWireName)))
				return code, bytes.ReplaceAll(body, []byte(OwnedToolWireName), []byte(remoteToolName))
			}
		}
		if original.StreamFor != nil {
			builder.StreamFor = func(request []byte) []string {
				events := original.StreamFor(bytes.ReplaceAll(request, []byte(remoteToolName), []byte(OwnedToolWireName)))
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
	if calls.Load() != 1 || !received.Load() {
		failures = append(failures, fmt.Sprintf("remote calls=%d q=x:%v", calls.Load(), received.Load()))
	}
	if env.VirtualCallCount() != 2 {
		failures = append(failures, fmt.Sprintf("model calls=%d, want 2", env.VirtualCallCount()))
	}
	if !strings.Contains(raw, OwnedToolFinalText) {
		failures = append(failures, "final answer missing")
	}
	if strings.Contains(raw, remoteToolName) {
		failures = append(failures, "remote gateway tool leaked to client")
	}
	last := string(requireLastRequest(t, env, target, "remote MCP continuation").Body)
	if !strings.Contains(last, "remote_structured_marker") {
		failures = append(failures, "structured result missing from continuation")
	}
	if failure {
		if !strings.Contains(last, ownedToolErrorText) {
			failures = append(failures, "tool failure missing from continuation")
		}
		if target == protocol.TypeAnthropicBeta && !strings.Contains(last, `"is_error":true`) {
			failures = append(failures, "tool error flag missing from Anthropic continuation")
		}
	}
	return failures, "client:\n" + raw + "\ncontinuation:\n" + last
}
