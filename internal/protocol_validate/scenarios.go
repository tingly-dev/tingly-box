package protocol_validate

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/server_validate"
)

// MockResponseBuilder re-exports server_validate.MockResponseBuilder for convenience.
type MockResponseBuilder = server_validate.MockResponseBuilder

// Scenario is a named test scenario describing:
//   - What the mock provider should return (MockResponses per APIStyle)
//   - What assertions to run on the round-trip result
type Scenario struct {
	Name        string
	Description string
	Tags        []string

	// MockResponses keyed by provider APIStyle ("openai", "anthropic", "google").
	MockResponses map[server_validate.APIStyle]MockResponseBuilder

	// Assertions run after every round-trip for this scenario.
	Assertions []Assertion
}

// toVirtualServerScenario converts to a server_validate.Scenario (strips assertions).
func (s Scenario) toVirtualServerScenario() server_validate.Scenario {
	return server_validate.Scenario{
		Name:          s.Name,
		Description:   s.Description,
		Tags:          s.Tags,
		MockResponses: s.MockResponses,
	}
}

// AllScenarios returns the full set of built-in validation scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		TextScenario(),
		ToolUseScenario(),
		ToolResultScenario(),
		ThinkingScenario(),
		MultiTurnScenario(),
		StreamingTextScenario(),
		StreamingToolUseScenario(),
		ErrorScenario(),
	}
}

// ─── Text ──────────────────────────────────────────────────────────────────────

// TextScenario is the baseline: a single user message and a plain text reply.
func TextScenario() Scenario {
	return Scenario{
		Name:        "text",
		Description: "Basic text completion: user asks a question, assistant answers",
		Tags:        []string{"text"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAITextResponse(),
			server_validate.StyleAnthropic: anthropicTextResponse(),
			server_validate.StyleGoogle:    googleTextResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertRoleEquals("assistant"),
			AssertContentContains("Paris"),
		},
	}
}

func openAITextResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":      "chatcmpl-validate-text",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "The capital of France is Paris.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 8,
			"total_tokens":      18,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAITextSSE,
	}
}

func anthropicTextResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":   "msg-validate-text",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{"type": "text", "text": "The capital of France is Paris."},
		},
		"model":         "claude-3-5-sonnet-20241022",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 8,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    anthropicTextSSE,
	}
}

func googleTextResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"text": "The capital of France is Paris."},
					},
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     10,
			"candidatesTokenCount": 8,
			"totalTokenCount":      18,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    googleTextSSE,
	}
}

// ─── Tool Use ─────────────────────────────────────────────────────────────────

// ToolUseScenario exercises a single tool/function call.
func ToolUseScenario() Scenario {
	return Scenario{
		Name:        "tool_use",
		Description: "Single tool call: assistant calls get_weather with location arg",
		Tags:        []string{"tool_use"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAIToolUseResponse(),
			server_validate.StyleAnthropic: anthropicToolUseResponse(),
			server_validate.StyleGoogle:    googleToolUseResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertHasToolCalls(1),
			AssertToolCallName(0, "get_weather"),
			AssertToolCallArgs(0, "location", "Paris"),
		},
	}
}

func openAIToolUseResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":      "chatcmpl-validate-tool",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]interface{}{
						{
							"id":   "call_validate_weather_1",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"Paris","unit":"celsius"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     15,
			"completion_tokens": 20,
			"total_tokens":      35,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAIToolUseSSE,
	}
}

func anthropicToolUseResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":   "msg-validate-tool",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type":  "tool_use",
				"id":    "toolu_validate_weather_1",
				"name":  "get_weather",
				"input": map[string]interface{}{"location": "Paris", "unit": "celsius"},
			},
		},
		"model":       "claude-3-5-sonnet-20241022",
		"stop_reason": "tool_use",
		"usage": map[string]interface{}{
			"input_tokens":  15,
			"output_tokens": 20,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    anthropicToolUseSSE,
	}
}

func googleToolUseResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{
							"functionCall": map[string]interface{}{
								"name": "get_weather",
								"args": map[string]interface{}{"location": "Paris", "unit": "celsius"},
							},
						},
					},
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     15,
			"candidatesTokenCount": 20,
			"totalTokenCount":      35,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    googleToolUseSSE,
	}
}

// ─── Tool Result ──────────────────────────────────────────────────────────────

// ToolResultScenario tests a multi-turn conversation including a tool result message.
func ToolResultScenario() Scenario {
	return Scenario{
		Name:        "tool_result",
		Description: "Multi-turn with tool result: user→assistant(tool_use)→user(tool_result)→assistant",
		Tags:        []string{"tool_use", "multi_turn"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAITextResponse(),
			server_validate.StyleAnthropic: anthropicTextResponse(),
			server_validate.StyleGoogle:    googleTextResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertRoleEquals("assistant"),
			AssertContentContains("Paris"),
		},
	}
}

// ─── Thinking ─────────────────────────────────────────────────────────────────

// ThinkingScenario tests that extended thinking blocks are present in Anthropic responses.
func ThinkingScenario() Scenario {
	return Scenario{
		Name:        "thinking",
		Description: "Extended thinking: response contains a thinking block before text",
		Tags:        []string{"thinking"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleAnthropic: anthropicThinkingResponse(),
			server_validate.StyleOpenAI:    openAITextResponse(),
			server_validate.StyleGoogle:    googleTextResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
		},
	}
}

func anthropicThinkingResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":   "msg-validate-thinking",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type":      "thinking",
				"thinking":  "Let me reason about this step by step...",
				"signature": "thinking_sig_validate",
			},
			{
				"type": "text",
				"text": "The capital of France is Paris.",
			},
		},
		"model":       "claude-opus-4-6-20250514",
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 30,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    anthropicThinkingSSE,
	}
}

// ─── Multi-turn ───────────────────────────────────────────────────────────────

// MultiTurnScenario tests a conversation with system prompt + 2 turns of history.
func MultiTurnScenario() Scenario {
	return Scenario{
		Name:        "multi_turn",
		Description: "Multi-turn conversation: system + user/assistant history + final user message",
		Tags:        []string{"multi_turn"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAITextResponse(),
			server_validate.StyleAnthropic: anthropicTextResponse(),
			server_validate.StyleGoogle:    googleTextResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertRoleEquals("assistant"),
			AssertContentContains("Paris"),
		},
	}
}

// ─── Streaming Text ───────────────────────────────────────────────────────────

// StreamingTextScenario tests SSE streaming for a plain text response.
func StreamingTextScenario() Scenario {
	return Scenario{
		Name:        "streaming_text",
		Description: "Streaming text: SSE chunks assembling to a complete text response",
		Tags:        []string{"text", "streaming"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAITextResponse(),
			server_validate.StyleAnthropic: anthropicTextResponse(),
			server_validate.StyleGoogle:    googleTextResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertStreamEventCount(3),
			AssertContentContains("Paris"),
		},
	}
}

// ─── Streaming Tool Use ───────────────────────────────────────────────────────

// StreamingToolUseScenario tests SSE streaming for a tool call response.
func StreamingToolUseScenario() Scenario {
	return Scenario{
		Name:        "streaming_tool_use",
		Description: "Streaming tool use: SSE chunks with tool call deltas",
		Tags:        []string{"tool_use", "streaming"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAIToolUseResponse(),
			server_validate.StyleAnthropic: anthropicToolUseResponse(),
			server_validate.StyleGoogle:    googleToolUseResponse(),
		},
		Assertions: []Assertion{
			AssertHTTPStatus(200),
			AssertStreamEventCount(3),
		},
	}
}

// ─── Error ────────────────────────────────────────────────────────────────────

// ErrorScenario tests that provider error responses are forwarded to the client.
func ErrorScenario() Scenario {
	return Scenario{
		Name:        "error",
		Description: "Provider rate limit error (429) propagated to client",
		Tags:        []string{"error"},
		MockResponses: map[server_validate.APIStyle]MockResponseBuilder{
			server_validate.StyleOpenAI:    openAIErrorResponse(),
			server_validate.StyleAnthropic: anthropicErrorResponse(),
			server_validate.StyleGoogle:    googleErrorResponse(),
		},
		Assertions: []Assertion{},
	}
}

func openAIErrorResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"error": map[string]interface{}{
			"message": "Rate limit exceeded",
			"type":    "rate_limit_error",
			"code":    "rate_limit_exceeded",
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 429, mustMarshal(body) },
		Stream: func() []string {
			return []string{`data: {"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`}
		},
	}
}

func anthropicErrorResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "rate_limit_error",
			"message": "Rate limit exceeded",
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 429, mustMarshal(body) },
		Stream: func() []string {
			return []string{`event: error`, `data: {"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`}
		},
	}
}

func googleErrorResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    429,
			"message": "Rate limit exceeded",
			"status":  "RESOURCE_EXHAUSTED",
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 429, mustMarshal(body) },
		Stream:    func() []string { return []string{`data: {"error":{"code":429,"message":"Rate limit exceeded"}}`} },
	}
}

// ─── SSE event helpers ────────────────────────────────────────────────────────

func openAITextSSE() []string {
	return []string{
		`data: {"id":"chatcmpl-validate-text","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-text","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"The capital of France is Paris."},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-text","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}
}

func anthropicTextSSE() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-validate-text","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The capital of France is Paris."}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":8}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

func googleTextSSE() []string {
	chunk := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]interface{}{{"text": "The capital of France is Paris."}},
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
	}
	return []string{
		"data: " + string(mustMarshal(chunk)),
	}
}

func openAIToolUseSSE() []string {
	return []string{
		`data: {"id":"chatcmpl-validate-tool","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_validate_weather_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-tool","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":\"Paris\","}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-tool","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"unit\":\"celsius\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
}

func anthropicToolUseSSE() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-validate-tool","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"usage":{"input_tokens":15,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_validate_weather_1","name":"get_weather","input":{}}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"location\":\"Paris\",\"unit\":\"celsius\"}"}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":20}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

func googleToolUseSSE() []string {
	chunk := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"functionCall": map[string]interface{}{"name": "get_weather", "args": map[string]interface{}{"location": "Paris", "unit": "celsius"}}},
					},
				},
				"finishReason": "STOP",
			},
		},
	}
	return []string{
		"data: " + string(mustMarshal(chunk)),
	}
}

func anthropicThinkingSSE() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-validate-thinking","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6-20250514","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me reason about this step by step..."}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The capital of France is Paris."}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":30}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}
	return b
}
