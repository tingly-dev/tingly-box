package protocoltest

import (
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Request-shape cases for Responses `namespace` tools (issue #2305), the way
// Codex sends MCP tools. They plug into contentShapeCases(), so they run in
// TestContentShapes and in `harness matrix --mode=content_shapes` / `--mode=all`;
// they live in their own file only to keep content_shapes.go from growing with
// every topic.

// The namespace and member are chosen so that they flatten to "get_weather",
// the tool name the shared tool-use mock answers with: the upstream then
// "calls" exactly the function it was offered, with no mock of our own.
const (
	namespaceToolNamespace = "get_"
	namespaceToolMember    = "weather"
	namespaceToolFlat      = "get_weather"
)

// namespaceToolBody is a Codex request: a plain function next to a namespace
// tool, and an earlier call to the namespaced one replayed with its namespace.
func namespaceToolBody() map[string]any {
	params := map[string]any{"type": "object", "properties": map[string]any{"location": map[string]any{"type": "string"}}}
	return map[string]any{
		"tools": []map[string]any{
			{"type": "function", "name": "shell", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
			{"type": "namespace", "name": namespaceToolNamespace, "description": "Weather MCP server", "tools": []map[string]any{
				{"type": "function", "name": namespaceToolMember, "description": "Get the weather", "parameters": params},
			}},
		},
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": "What's the weather in Paris?"},
			{"type": "function_call", "call_id": "call_ns_1", "namespace": namespaceToolNamespace, "name": namespaceToolMember, "arguments": `{"location":"Paris"}`},
			{"type": "function_call_output", "call_id": "call_ns_1", "output": "Sunny"},
		},
	}
}

// forwardedTools reads a captured Chat or Anthropic request as
// "<offered tool names>|<name of the replayed tool call>".
func forwardedTools(body map[string]any) (string, bool) {
	tools, ok := body["tools"].([]any)
	if !ok {
		return "", false
	}
	var names []string
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _ := tool["name"].(string) // Anthropic
		if fn, ok := tool["function"].(map[string]any); ok {
			name, _ = fn["name"].(string) // Chat
		}
		names = append(names, name)
	}
	var call string
	msgs, _ := body["messages"].([]any)
	for _, raw := range msgs {
		msg, _ := raw.(map[string]any)
		calls, _ := msg["tool_calls"].([]any) // Chat
		for _, c := range calls {
			fn, _ := c.(map[string]any)["function"].(map[string]any)
			call, _ = fn["name"].(string)
		}
		blocks, _ := msg["content"].([]any) // Anthropic
		for _, b := range blocks {
			if block, _ := b.(map[string]any); block["type"] == "tool_use" {
				call, _ = block["name"].(string)
			}
		}
	}
	return strings.Join(names, ",") + "|" + call, true
}

// assertResponsesNamespaceTools sends the Codex request to target and checks
// both directions: the upstream is offered the member as a flat function (and
// the replayed call uses the same name), and the call it answers with reaches
// the client as a function_call with its namespace restored.
func assertResponsesNamespaceTools(t flagTB, env *TestEnv, target protocol.APIType, endpoint EndpointKind, streaming bool) {
	t.Helper()
	res := sendResponsesWithScenario(t, env, target, toolUseShapeScenario(), namespaceToolBody(), streaming)
	if res.HTTPStatus != 200 {
		t.Fatalf("http status = %d, body %s", res.HTTPStatus, string(res.RawBody))
	}

	want := "shell," + namespaceToolFlat + "|" + namespaceToolFlat
	if got, ok := forwardedTools(env.virtual.LastRequest(endpoint).JSON()); !ok || got != want {
		t.Errorf("upstream %s tools|call = %q (ok=%v), want %q", endpoint, got, ok, want)
	}

	calls := responsesFunctionCallItems(res)
	if len(calls) != 1 || calls[0]["namespace"] != namespaceToolNamespace || calls[0]["name"] != namespaceToolMember {
		t.Errorf("function_call items = %v, want one named %q in namespace %q", calls, namespaceToolMember, namespaceToolNamespace)
	}
}

// namespaceToolCases pin that a Responses client's `namespace` tools survive a
// Chat or Anthropic upstream. They were dropped silently before, so the model
// never saw, let alone called, those tools.
func namespaceToolCases() []contentShapeCase {
	targets := []struct {
		name     string
		target   protocol.APIType
		endpoint EndpointKind
	}{
		{"responses_to_chat", protocol.TypeOpenAIChat, EndpointChat},
		{"responses_to_anthropic", protocol.TypeAnthropicBeta, EndpointAnthropic},
	}
	var cases []contentShapeCase
	for _, tc := range targets {
		for _, streaming := range []bool{false, true} {
			name := tc.name + "/namespace_tools"
			if streaming {
				name += "_stream"
			}
			cases = append(cases, contentShapeCase{name: name, run: func(t flagTB, env *TestEnv) {
				assertResponsesNamespaceTools(t, env, tc.target, tc.endpoint, streaming)
			}})
		}
	}
	return cases
}
