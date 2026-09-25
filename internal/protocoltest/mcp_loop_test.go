package protocoltest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

var _ = registerKnownGaps(KnownGap{
	ID:     "M1",
	Reason: "Responses source: the server tool is injected upstream but its call is not intercepted, so it leaks to the client",
},
	"TestMCPOwnedToolLoop/openai_responses->anthropic_beta/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->anthropic_beta/stream=true",
	"TestMCPOwnedToolLoop/openai_responses->openai_chat/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->openai_chat/stream=true",
) && registerKnownGaps(KnownGap{
	ID:     "M3",
	Reason: "OpenAI Responses target: server tools are not offered to the model at all",
},
	"TestMCPOwnedToolLoop/anthropic_v1->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/anthropic_v1->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/anthropic_beta->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/anthropic_beta->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/openai_chat->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/openai_chat->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/openai_responses->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->openai_responses/stream=true",
)

// TestMCPOwnedToolLoop pins the server-owned tool loop through the real HTTP
// gateway for every protocol pair: the gateway offers its tool, executes the
// model's call exactly once with the model's arguments, continues the
// conversation, and returns only the final answer — the client never sees the
// server tool.
func TestMCPOwnedToolLoop(t *testing.T) {
	t.Parallel()

	for _, pair := range DefaultPairs() {
		for _, streaming := range []bool{false, true} {
			pair, streaming := pair, streaming
			name := fmt.Sprintf("%s->%s/stream=%v", pair.Source, pair.Target, streaming)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				echo := NewEchoServertoolProvider()
				env := NewTestEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
				scenario := OwnedToolScenario()
				env.SetupRoute(pair.Source, pair.Target, scenario)
				model := env.findRouteModel(pair.Source, pair.Target, scenario.Name)
				path, body := buildRequest(pair.Source, model, streaming)

				status, raw := sendRaw(t, env, path, body)

				var failures []string
				if status != 200 {
					failures = append(failures, fmt.Sprintf("status = %d", status))
				}
				calls := echo.Calls()
				if len(calls) != 1 {
					failures = append(failures, fmt.Sprintf("server tool executed %d times, want 1", len(calls)))
				} else if calls[0]["q"] != "x" {
					failures = append(failures, fmt.Sprintf("server tool args = %v, want q=x", calls[0]))
				}
				if got := env.VirtualCallCount(); got != 2 {
					failures = append(failures, fmt.Sprintf("upstream calls = %d, want 2 (tool round + final round)", got))
				}
				if !strings.Contains(raw, OwnedToolFinalText) {
					failures = append(failures, "final answer missing from client response")
				}
				if strings.Contains(raw, OwnedToolWireName) {
					failures = append(failures, "server tool call leaked to client")
				}
				checkCase(t, t.Name(), failures, "client response:\n"+raw)
			})
		}
	}
}

// TestMCPOwnedToolNotOfferedWhenDisabled pins that without the MCP extension
// the gateway neither offers nor executes server tools.
func TestMCPOwnedToolNotOfferedWhenDisabled(t *testing.T) {
	t.Parallel()

	env := NewTestEnv(t)
	scenario := OwnedToolScenario()
	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, scenario)
	model := env.findRouteModel(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, scenario.Name)
	path, body := buildRequest(protocol.TypeAnthropicBeta, model, false)

	status, raw := sendRaw(t, env, path, body)
	if status != 200 {
		t.Fatalf("status = %d: %s", status, raw)
	}
	if upstream := string(env.virtual.LastRequest(EndpointAnthropic).Body); strings.Contains(upstream, OwnedToolWireName) {
		t.Fatalf("server tool offered upstream with MCP disabled:\n%s", upstream)
	}
	if got := env.VirtualCallCount(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

// Pairs on which the server-tool loop runs today; Responses on either side is
// covered by gaps M1/M3 in TestMCPOwnedToolLoop.
var toolLoopSources = []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
var toolLoopTargets = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}

// TestMCPServerToolError pins that a failing server tool is reported back to
// the model (which then answers) rather than aborting the request, and that
// the failure is not retried.
func TestMCPServerToolError(t *testing.T) {
	t.Parallel()

	for _, source := range toolLoopSources {
		for _, target := range toolLoopTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				t.Run(fmt.Sprintf("%s->%s/stream=%v", source, target, streaming), func(t *testing.T) {
					t.Parallel()

					echo := &EchoServertoolProvider{Fail: true}
					env := NewTestEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
					scenario := OwnedToolScenario()
					env.SetupRoute(source, target, scenario)
					model := env.findRouteModel(source, target, scenario.Name)
					path, body := buildRequest(source, model, streaming)

					status, raw := sendRaw(t, env, path, body)
					var failures []string
					if status != 200 {
						failures = append(failures, fmt.Sprintf("status = %d", status))
					}
					if n := len(echo.Calls()); n != 1 {
						failures = append(failures, fmt.Sprintf("failing server tool executed %d times, want 1", n))
					}
					if last := env.virtual.LastRequest(cacheControlEndpoint(target)); last == nil || !strings.Contains(string(last.Body), ownedToolErrorText) {
						failures = append(failures, "tool failure was not reported to the model")
					}
					if !strings.Contains(raw, OwnedToolFinalText) {
						failures = append(failures, "final answer missing from client response")
					}
					if strings.Contains(raw, OwnedToolWireName) {
						failures = append(failures, "server tool call leaked to client")
					}
					checkCase(t, t.Name(), failures, "client response:\n"+raw)
				})
			}
		}
	}
}

var _ = registerKnownGaps(KnownGap{
	ID:     "M4",
	Reason: "Anthropic client -> Chat provider streaming loop fails the request (500) at the round limit instead of ending it like other paths",
},
	"TestMCPToolLoopBounded/anthropic_v1->openai_chat/stream=true",
	"TestMCPToolLoopBounded/anthropic_beta->openai_chat/stream=true",
)

// TestMCPToolLoopBounded pins that a model which keeps calling a server tool
// cannot hold the request forever, and that the loop's cut-off never hands
// the server tool call to the client.
func TestMCPToolLoopBounded(t *testing.T) {
	t.Parallel()

	const maxToolRounds = 3 // InterceptorConfig.MaxRounds at every call site
	for _, source := range toolLoopSources {
		for _, target := range toolLoopTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				t.Run(fmt.Sprintf("%s->%s/stream=%v", source, target, streaming), func(t *testing.T) {
					t.Parallel()

					echo := NewEchoServertoolProvider()
					env := NewTestEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
					scenario := AlwaysOwnedToolScenario()
					env.SetupRoute(source, target, scenario)
					model := env.findRouteModel(source, target, scenario.Name)
					path, body := buildRequest(source, model, streaming)

					status, raw := sendRaw(t, env, path, body)
					var failures []string
					if status != 200 {
						failures = append(failures, fmt.Sprintf("status = %d", status))
					}
					if n := len(echo.Calls()); n > maxToolRounds {
						failures = append(failures, fmt.Sprintf("server tool executed %d times, limit %d", n, maxToolRounds))
					}
					if n := env.VirtualCallCount(); n > maxToolRounds+1 {
						failures = append(failures, fmt.Sprintf("upstream calls = %d, limit %d", n, maxToolRounds+1))
					}
					if strings.Contains(raw, OwnedToolWireName) {
						failures = append(failures, "server tool call leaked to client at the round limit")
					}
					checkCase(t, t.Name(), failures, fmt.Sprintf("tool calls=%d upstream=%d client response:\n%s", len(echo.Calls()), env.VirtualCallCount(), raw))
				})
			}
		}
	}
}

// followUpWithClientToolResult builds the client's next request in its own
// protocol: the original question, the assistant turn carrying the client
// tool call it received (id toolID), and that tool's result.
func followUpWithClientToolResult(source protocol.APIType, model, toolID string, streaming bool) (string, []byte) {
	path, _ := buildRequest(source, model, streaming)
	switch source {
	case protocol.TypeOpenAIChat:
		return path, mustMarshal(map[string]any{
			"model": model, "stream": streaming,
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": nil, "tool_calls": []map[string]any{{
					"id": toolID, "type": "function", "function": map[string]any{"name": clientToolName, "arguments": `{"location":"Paris"}`},
				}}},
				{"role": "tool", "tool_call_id": toolID, "content": clientToolResultText},
			},
		})
	default: // Anthropic V1 / Beta share the message shape
		return path, mustMarshal(map[string]any{
			"model": model, "max_tokens": 1024, "stream": streaming,
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": []map[string]any{{
					"type": "tool_use", "id": toolID, "name": clientToolName, "input": map[string]any{"location": "Paris"},
				}}},
				{"role": "user", "content": []map[string]any{{
					"type": "tool_result", "tool_use_id": toolID, "content": clientToolResultText,
				}}},
			},
		})
	}
}

var _ = registerKnownGaps(KnownGap{
	ID:     "M5",
	Reason: "Chat client -> Anthropic provider streaming: the mixed round's server-tool result is not spliced into the follow-up",
},
	"TestMCPMixedToolContinuation/openai_chat->anthropic_beta/stream=true",
)

// TestMCPMixedToolContinuation pins the two-request mixed round: the server
// tool runs in the first request and is hidden, the client receives only its
// own tool call, and when it returns that result the gateway resumes the
// provider conversation with the stored server-tool result spliced back in.
func TestMCPMixedToolContinuation(t *testing.T) {
	t.Parallel()

	for _, source := range toolLoopSources {
		for _, target := range toolLoopTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				t.Run(fmt.Sprintf("%s->%s/stream=%v", source, target, streaming), func(t *testing.T) {
					t.Parallel()

					echo := NewEchoServertoolProvider()
					env := NewTestEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
					scenario := MixedToolScenario()
					env.SetupRoute(source, target, scenario)
					model := env.findRouteModel(source, target, scenario.Name)
					headers := map[string]string{"X-Tingly-Session-ID": "mixed-" + t.Name()}

					path, body := buildRequest(source, model, streaming)
					status, first := sendRawWithHeaders(t, env, path, body, headers)
					var failures []string
					if status != 200 {
						failures = append(failures, fmt.Sprintf("first request status = %d", status))
					}
					if n := len(echo.Calls()); n != 1 {
						failures = append(failures, fmt.Sprintf("server tool executed %d times in the mixed round, want 1", n))
					}
					if strings.Contains(first, OwnedToolWireName) {
						failures = append(failures, "server tool call leaked to client in the mixed round")
					}
					toolID := ""
					for _, id := range []string{"toolu-client-tool", "call-client-tool"} {
						if strings.Contains(first, id) {
							toolID = id
						}
					}
					if toolID == "" {
						failures = append(failures, "client tool call missing from the mixed round")
						checkCase(t, t.Name(), failures, "first response:\n"+first)
						return
					}

					path, body = followUpWithClientToolResult(source, model, toolID, streaming)
					status, second := sendRawWithHeaders(t, env, path, body, headers)
					if status != 200 {
						failures = append(failures, fmt.Sprintf("follow-up status = %d", status))
					}
					if last := env.virtual.LastRequest(cacheControlEndpoint(target)); last == nil || !strings.Contains(string(last.Body), ownedToolResultText) {
						failures = append(failures, "follow-up did not carry the stored server-tool result upstream")
					}
					if !strings.Contains(second, OwnedToolFinalText) {
						failures = append(failures, "final answer missing from follow-up response")
					}
					if n := len(echo.Calls()); n != 1 {
						failures = append(failures, fmt.Sprintf("server tool executed %d times in total, want 1", n))
					}
					checkCase(t, t.Name(), failures, "first response:\n"+first+"\nfollow-up response:\n"+second)
				})
			}
		}
	}
}
