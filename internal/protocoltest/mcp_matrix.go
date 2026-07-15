package protocoltest

import (
	"context"
	"net/http"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

const (
	MCPStageOwnedToolScenarioName = "mcp_owned_tool"
	matrixOwnedToolName           = "tingly_box_mcp__builtin__echo"
)

type matrixEchoServertoolProvider struct{}

func newMatrixEchoServertoolProvider() servertool.ToolProvider {
	return matrixEchoServertoolProvider{}
}

func (matrixEchoServertoolProvider) Descriptor() coretool.VirtualTool {
	return coretool.VirtualTool{
		Name:        "echo",
		Description: "Echo a value for protocol-stage matrix validation",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
			"required": []string{"q"},
		},
		Handler: func(context.Context, coretool.ToolCall) (coretool.ToolResult, error) {
			return coretool.TextToolResult("echo-result"), nil
		},
	}
}

func (matrixEchoServertoolProvider) Hook() servertool.Hook { return nil }

// newMCPStageOwnedToolScenario returns alternating first-round tool calls and
// final text responses. Matrix execution is sequential per scenario, and each
// case makes exactly two calls to its target response format.
func newMCPStageOwnedToolScenario() Scenario {
	var mu sync.Mutex
	nonStreamCalls := make(map[ResponseFormat]int)
	streamCalls := make(map[ResponseFormat]int)

	nonStream := func(format ResponseFormat, first, final any) func() (int, []byte) {
		return func() (int, []byte) {
			mu.Lock()
			defer mu.Unlock()
			nonStreamCalls[format]++
			if nonStreamCalls[format]%2 == 1 {
				return http.StatusOK, mustMarshal(first)
			}
			return http.StatusOK, mustMarshal(final)
		}
	}
	stream := func(format ResponseFormat, first, final []string) func() []string {
		return func() []string {
			mu.Lock()
			defer mu.Unlock()
			streamCalls[format]++
			if streamCalls[format]%2 == 1 {
				return first
			}
			return final
		}
	}

	return Scenario{
		Name:        MCPStageOwnedToolScenarioName,
		Description: "Stage executes a server-owned tool and returns the provider's second-round answer",
		Tags:        []string{"mcp", "servertool", "stage"},
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatAnthropic: {
				NonStream: nonStream(FormatAnthropic, matrixAnthropicOwnedTool(), matrixAnthropicOwnedToolFinal()),
				Stream:    stream(FormatAnthropic, matrixAnthropicOwnedToolStream(), matrixAnthropicOwnedToolFinalStream()),
			},
			FormatOpenAIChat: {
				NonStream: nonStream(FormatOpenAIChat, matrixChatOwnedTool(), matrixChatOwnedToolFinal()),
				Stream:    stream(FormatOpenAIChat, matrixChatOwnedToolStream(), matrixChatOwnedToolFinalStream()),
			},
			FormatOpenAIResponses: {
				NonStream: nonStream(FormatOpenAIResponses, matrixResponsesOwnedTool(), matrixResponsesOwnedToolFinal()),
				Stream:    stream(FormatOpenAIResponses, matrixResponsesOwnedToolStream(), matrixResponsesOwnedToolFinalStream()),
			},
		},
		Assertions: []Assertion{
			AssertHTTPStatus(http.StatusOK),
			AssertContentEquals("owned-tool-final"),
		},
	}
}

func matrixAnthropicOwnedTool() map[string]any {
	return map[string]any{
		"id": "msg-owned-tool", "type": "message", "role": "assistant", "model": "worker-model",
		"content":     []map[string]any{{"type": "tool_use", "id": "toolu-owned-tool", "name": matrixOwnedToolName, "input": map[string]any{"q": "x"}}},
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 8, "output_tokens": 3},
	}
}

func matrixAnthropicOwnedToolFinal() map[string]any {
	return map[string]any{
		"id": "msg-owned-tool-final", "type": "message", "role": "assistant", "model": "worker-model",
		"content":     []map[string]any{{"type": "text", "text": "owned-tool-final"}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 12, "output_tokens": 5},
	}
}

func matrixAnthropicOwnedToolStream() []string {
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

func matrixAnthropicOwnedToolFinalStream() []string {
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

func matrixChatOwnedTool() map[string]any {
	return map[string]any{
		"id": "chatcmpl-owned-tool", "object": "chat.completion", "created": 1, "model": "worker-model",
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{
				"id": "call-owned-tool", "type": "function", "function": map[string]any{"name": matrixOwnedToolName, "arguments": `{"q":"x"}`},
			}}},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11},
	}
}

func matrixChatOwnedToolFinal() map[string]any {
	return map[string]any{
		"id": "chatcmpl-owned-tool-final", "object": "chat.completion", "created": 2, "model": "worker-model",
		"choices": []map[string]any{{
			"index": 0, "message": map[string]any{"role": "assistant", "content": "owned-tool-final"}, "finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17},
	}
}

func matrixChatOwnedToolStream() []string {
	return []string{
		`data: {"id":"chatcmpl-owned-tool","object":"chat.completion.chunk","created":1,"model":"worker-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-owned-tool","type":"function","function":{"name":"tingly_box_mcp__builtin__echo","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-owned-tool","object":"chat.completion.chunk","created":1,"model":"worker-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
}

func matrixChatOwnedToolFinalStream() []string {
	return []string{
		`data: {"id":"chatcmpl-owned-tool-final","object":"chat.completion.chunk","created":2,"model":"worker-model","choices":[{"index":0,"delta":{"role":"assistant","content":"owned-tool-final"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-owned-tool-final","object":"chat.completion.chunk","created":2,"model":"worker-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}
}

func matrixResponsesOwnedTool() map[string]any {
	return map[string]any{
		"id": "resp-owned-tool", "object": "response", "created_at": 1, "model": "worker-model", "status": "completed",
		"output": []map[string]any{{
			"id": "fc-owned-tool", "type": "function_call", "call_id": "call-owned-tool", "name": matrixOwnedToolName, "arguments": `{"q":"x"}`, "status": "completed",
		}},
		"usage": map[string]any{"input_tokens": 8, "output_tokens": 3, "total_tokens": 11},
	}
}

func matrixResponsesOwnedToolFinal() map[string]any {
	return map[string]any{
		"id": "resp-owned-tool-final", "object": "response", "created_at": 2, "model": "worker-model", "status": "completed",
		"output": []map[string]any{{
			"id": "item-owned-tool-final", "type": "message", "role": "assistant", "status": "completed",
			"content": []map[string]any{{"type": "output_text", "text": "owned-tool-final", "annotations": []any{}}},
		}},
		"usage": map[string]any{"input_tokens": 12, "output_tokens": 5, "total_tokens": 17},
	}
}

func matrixResponsesOwnedToolStream() []string {
	return []string{
		`data: {"type":"response.created","response":{"id":"resp-owned-tool","object":"response","created_at":1,"model":"worker-model","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.added","response_id":"resp-owned-tool","output_index":0,"item":{"id":"fc-owned-tool","type":"function_call","call_id":"call-owned-tool","name":"tingly_box_mcp__builtin__echo","status":"in_progress"}}`,
		`data: {"type":"response.function_call_arguments.delta","response_id":"resp-owned-tool","item_id":"fc-owned-tool","output_index":0,"delta":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.function_call_arguments.done","response_id":"resp-owned-tool","item_id":"fc-owned-tool","output_index":0,"arguments":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp-owned-tool","object":"response","created_at":1,"model":"worker-model","status":"completed","output":[{"id":"fc-owned-tool","type":"function_call","call_id":"call-owned-tool","name":"tingly_box_mcp__builtin__echo","arguments":"{\"q\":\"x\"}","status":"completed"}],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}}`,
		`data: [DONE]`,
	}
}

func matrixResponsesOwnedToolFinalStream() []string {
	return []string{
		`data: {"type":"response.created","response":{"id":"resp-owned-tool-final","object":"response","created_at":2,"model":"worker-model","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.added","response_id":"resp-owned-tool-final","output_index":0,"item":{"id":"item-owned-tool-final","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`data: {"type":"response.output_text.delta","response_id":"resp-owned-tool-final","item_id":"item-owned-tool-final","output_index":0,"content_index":0,"delta":"owned-tool-final"}`,
		`data: {"type":"response.output_text.done","response_id":"resp-owned-tool-final","item_id":"item-owned-tool-final","output_index":0,"content_index":0,"text":"owned-tool-final"}`,
		`data: {"type":"response.completed","response":{"id":"resp-owned-tool-final","object":"response","created_at":2,"model":"worker-model","status":"completed","output":[{"id":"item-owned-tool-final","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"owned-tool-final","annotations":[]}]}],"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17}}}`,
		`data: [DONE]`,
	}
}
