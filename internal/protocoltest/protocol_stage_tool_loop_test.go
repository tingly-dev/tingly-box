package protocoltest

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

const ownedToolName = "tingly_box_mcp__builtin__echo"

type echoServertoolProvider struct {
	mu        sync.Mutex
	calls     int
	arguments map[string]any
}

func (p *echoServertoolProvider) Descriptor() coretool.VirtualTool {
	return coretool.VirtualTool{
		Name:        "echo",
		Description: "Echo a value for protocol-stage integration tests",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
			"required": []string{"q"},
		},
		Handler: func(_ context.Context, call coretool.ToolCall) (coretool.ToolResult, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.calls++
			p.arguments = call.Arguments
			return coretool.TextToolResult("echo-result"), nil
		},
	}
}

func (p *echoServertoolProvider) Hook() servertool.Hook { return nil }

func (p *echoServertoolProvider) snapshot() (int, map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.arguments
}

func TestServerProtocolStageOwnedToolLoopHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source protocol.APIType
		target protocol.APIType
	}{
		{name: "beta_native", source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta},
		{name: "v1_promoted_to_beta_then_chat", source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, streaming := range []bool{false, true} {
				mode := "complete"
				if streaming {
					mode = "stream"
				}
				t.Run(mode, func(t *testing.T) {
					t.Parallel()
					provider := &echoServertoolProvider{}
					env := NewTestEnv(t,
						NewTestEnvOptionWithProtocolStage(),
						NewTestEnvOptionWithMCP(),
						NewTestEnvOptionWithServertoolProviders(provider),
					)
					scenario := ownedToolLoopScenario()
					env.SetupRoute(tt.source, tt.target, scenario)

					result := env.SendAs(t, tt.source, tt.target, scenario, streaming)
					if result.HTTPStatus != http.StatusOK {
						t.Fatalf("status = %d, body = %s", result.HTTPStatus, result.RawBody)
					}
					if result.Content != "owned-tool-final" {
						t.Fatalf("content = %q, want owned-tool-final; body = %s", result.Content, result.RawBody)
					}
					if len(result.ToolCalls) != 0 {
						t.Fatalf("final response leaked %d tool calls", len(result.ToolCalls))
					}
					if env.VirtualCallCount() != 2 {
						t.Fatalf("provider calls = %d, want 2", env.VirtualCallCount())
					}
					calls, arguments := provider.snapshot()
					if calls != 1 {
						t.Fatalf("local tool executions = %d, want 1", calls)
					}
					if arguments["q"] != "x" {
						t.Fatalf("local tool arguments = %#v, want q=x", arguments)
					}
				})
			}
		})
	}
}

func ownedToolLoopScenario() Scenario {
	var mu sync.Mutex
	nonStreamCalls := map[ResponseFormat]int{}
	streamCalls := map[ResponseFormat]int{}

	nextNonStream := func(format ResponseFormat, first, final any) func() (int, []byte) {
		return func() (int, []byte) {
			mu.Lock()
			defer mu.Unlock()
			nonStreamCalls[format]++
			if nonStreamCalls[format] == 1 {
				return http.StatusOK, mustMarshal(first)
			}
			return http.StatusOK, mustMarshal(final)
		}
	}
	nextStream := func(format ResponseFormat, first, final []string) func() []string {
		return func() []string {
			mu.Lock()
			defer mu.Unlock()
			streamCalls[format]++
			if streamCalls[format] == 1 {
				return first
			}
			return final
		}
	}

	return Scenario{
		Name:        "mcp_owned_tool",
		Description: "Provider requests a server-owned tool, then returns a final answer",
		Tags:        []string{"mcp", "servertool", "stage"},
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatAnthropic: {
				NonStream: nextNonStream(FormatAnthropic, anthropicOwnedToolResponse(), anthropicOwnedToolFinalResponse()),
				Stream:    nextStream(FormatAnthropic, anthropicOwnedToolStream(), anthropicOwnedToolFinalStream()),
			},
			FormatOpenAIChat: {
				NonStream: nextNonStream(FormatOpenAIChat, openAIOwnedToolResponse(), openAIOwnedToolFinalResponse()),
				Stream:    nextStream(FormatOpenAIChat, openAIOwnedToolStream(), openAIOwnedToolFinalStream()),
			},
		},
	}
}

func anthropicOwnedToolResponse() map[string]any {
	return map[string]any{
		"id": "msg-owned-tool", "type": "message", "role": "assistant", "model": "worker-model",
		"content":     []map[string]any{{"type": "tool_use", "id": "toolu-owned-tool", "name": ownedToolName, "input": map[string]any{"q": "x"}}},
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 8, "output_tokens": 3},
	}
}

func anthropicOwnedToolFinalResponse() map[string]any {
	return map[string]any{
		"id": "msg-owned-tool-final", "type": "message", "role": "assistant", "model": "worker-model",
		"content":     []map[string]any{{"type": "text", "text": "owned-tool-final"}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 12, "output_tokens": 5},
	}
}

func anthropicOwnedToolStream() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-owned-tool","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":8,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu-owned-tool","name":"tingly_box_mcp__builtin__echo","input":{}}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":3}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

func anthropicOwnedToolFinalStream() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-owned-tool-final","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"owned-tool-final"}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

func openAIOwnedToolResponse() map[string]any {
	return map[string]any{
		"id": "chatcmpl-owned-tool", "object": "chat.completion", "created": 1, "model": "worker-model",
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{
				"id": "call-owned-tool", "type": "function", "function": map[string]any{"name": ownedToolName, "arguments": `{"q":"x"}`},
			}}},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11},
	}
}

func openAIOwnedToolFinalResponse() map[string]any {
	return map[string]any{
		"id": "chatcmpl-owned-tool-final", "object": "chat.completion", "created": 2, "model": "worker-model",
		"choices": []map[string]any{{
			"index": 0, "message": map[string]any{"role": "assistant", "content": "owned-tool-final"}, "finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17},
	}
}

func openAIOwnedToolStream() []string {
	return []string{
		`data: {"id":"chatcmpl-owned-tool","object":"chat.completion.chunk","created":1,"model":"worker-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-owned-tool","type":"function","function":{"name":"tingly_box_mcp__builtin__echo","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-owned-tool","object":"chat.completion.chunk","created":1,"model":"worker-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
}

func openAIOwnedToolFinalStream() []string {
	return []string{
		`data: {"id":"chatcmpl-owned-tool-final","object":"chat.completion.chunk","created":2,"model":"worker-model","choices":[{"index":0,"delta":{"role":"assistant","content":"owned-tool-final"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-owned-tool-final","object":"chat.completion.chunk","created":2,"model":"worker-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}
}
