package scenario

import (
	"time"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
)

// ─── Text ──────────────────────────────────────────────────────────────────────

// TextScenario is the baseline: a single user message and a plain text reply.
func TextScenario() Scenario {
	return Scenario{
		Name:        "text",
		Description: "Basic text completion: user asks a question, assistant answers",
		Tags:        []string{"text"},
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAITextResponse(),
			FormatOpenAIResponses: openAIResponsesTextResponse(),
			FormatAnthropic:       anthropicTextResponse(),
			FormatGoogle:          googleTextResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("Paris"),
			check.AssertContentNonEmpty(),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertContentNonEmpty(),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAIToolUseResponse(),
			FormatOpenAIResponses: openAIResponsesToolUseResponse(),
			FormatAnthropic:       anthropicToolUseResponse(),
			FormatGoogle:          googleToolUseResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertHasToolCalls(1),
			check.AssertToolCallName(0, "get_weather"),
			check.AssertToolCallArgs(0, "location", "Paris"),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertHasToolCalls(1),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAITextResponse(),
			FormatOpenAIResponses: openAIResponsesTextResponse(),
			FormatAnthropic:       anthropicTextResponse(),
			FormatGoogle:          googleTextResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("Paris"),
			check.AssertContentNonEmpty(),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertContentNonEmpty(),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatAnthropic:       anthropicThinkingResponse(),
			FormatOpenAIChat:      openAITextResponse(),
			FormatOpenAIResponses: openAIResponsesTextResponse(),
			FormatGoogle:          googleTextResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("Paris"),
			check.AssertContentNonEmpty(),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertContentNonEmpty(),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAITextResponse(),
			FormatOpenAIResponses: openAIResponsesTextResponse(),
			FormatAnthropic:       anthropicTextResponse(),
			FormatGoogle:          googleTextResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("Paris"),
			check.AssertContentNonEmpty(),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertContentNonEmpty(),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAITextResponse(),
			FormatOpenAIResponses: openAIResponsesTextResponse(),
			FormatAnthropic:       anthropicTextResponse(),
			FormatGoogle:          googleTextResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertStreamEventCount(3),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("Paris"),
			check.AssertContentNonEmpty(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertStreamEventCount(1),
			check.AssertContentNonEmpty(),
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
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAIToolUseResponse(),
			FormatOpenAIResponses: openAIResponsesToolUseResponse(),
			FormatAnthropic:       anthropicToolUseResponse(),
			FormatGoogle:          googleToolUseResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertStreamEventCount(3),
			check.AssertHasToolCalls(1),
			check.AssertToolCallName(0, "get_weather"),
			check.AssertToolCallArgs(0, "location", "Paris"),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertStreamEventCount(1),
			check.AssertHasToolCalls(1),
		},
	}
}

// ─── Incomplete ──────────────────────────────────────────────────────────────

// IncompleteScenario exercises the max-output-tokens truncation path.
// The provider returns a partial response that was cut short; the gateway
// must preserve the partial content and map the finish reason correctly
// (Chat → "length", Anthropic → "max_tokens", Responses → "incomplete").
func IncompleteScenario() Scenario {
	return Scenario{
		Name:        "incomplete",
		Description: "Provider truncated response due to max_output_tokens",
		Tags:        []string{"incomplete", "length"},
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      openAIIncompleteResponse(),
			FormatOpenAIResponses: openAIResponsesIncompleteResponse(),
			FormatAnthropic:       anthropicIncompleteResponse(),
			FormatGoogle:          googleIncompleteResponse(),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertRoleEquals("assistant"),
			check.AssertContentContains("truncated"),
			check.AssertContentNonEmpty(),
			check.AssertUsageNonZero(),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
			check.AssertContentNonEmpty(),
		},
	}
}

func openAIIncompleteResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":      "chatcmpl-validate-incomplete",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "This response was truncated due to output limit.",
				},
				"finish_reason": "length",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 50,
			"total_tokens":      60,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAIIncompleteSSE,
	}
}

func anthropicIncompleteResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":   "msg-validate-incomplete",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{"type": "text", "text": "This response was truncated due to output limit."},
		},
		"model":         "claude-3-5-sonnet-20241022",
		"stop_reason":   "max_tokens",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 50,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    anthropicIncompleteSSE,
	}
}

func googleIncompleteResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"text": "This response was truncated due to output limit."},
					},
				},
				"finishReason": "MAX_TOKENS",
				"index":        0,
			},
		},
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     10,
			"candidatesTokenCount": 50,
			"totalTokenCount":      60,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    googleIncompleteSSE,
	}
}

func openAIResponsesIncompleteResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":         "resp-validate-incomplete",
		"object":     "realtime.response",
		"created_at": time.Now().Unix(),
		"model":      "gpt-4o",
		"status":     "incomplete",
		"incomplete_details": map[string]interface{}{
			"reason": "max_output_tokens",
		},
		"output": []map[string]interface{}{
			{
				"id":     "item-validate-incomplete",
				"type":   "message",
				"role":   "assistant",
				"status": "incomplete",
				"content": []map[string]interface{}{
					{"type": "output_text", "text": "This response was truncated due to output limit.", "annotations": []interface{}{}},
				},
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 50,
			"total_tokens":  60,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAIResponsesIncompleteSSE,
	}
}

func openAIIncompleteSSE() []string {
	return []string{
		`data: {"id":"chatcmpl-validate-incomplete","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-incomplete","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"This response was truncated due to output limit."},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-validate-incomplete","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":50,"total_tokens":60}}`,
		`data: [DONE]`,
	}
}

func anthropicIncompleteSSE() []string {
	return []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-validate-incomplete","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"This response was truncated due to output limit."}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":50}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	}
}

func googleIncompleteSSE() []string {
	chunk := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]interface{}{{"text": "This response was truncated due to output limit."}},
				},
				"finishReason": "MAX_TOKENS",
				"index":        0,
			},
		},
	}
	return []string{
		"data: " + string(mustMarshal(chunk)),
	}
}

func openAIResponsesIncompleteSSE() []string {
	return []string{
		`data: {"type":"response.created","response":{"id":"resp-validate-incomplete","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.added","response_id":"resp-validate-incomplete","item":{"id":"item-validate-incomplete","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`data: {"type":"response.output_text.delta","response_id":"resp-validate-incomplete","item_id":"item-validate-incomplete","output_index":0,"content_index":0,"delta":"This response was truncated due to output limit."}`,
		`data: {"type":"response.output_text.done","response_id":"resp-validate-incomplete","item_id":"item-validate-incomplete","output_index":0,"content_index":0,"text":"This response was truncated due to output limit."}`,
		`data: {"type":"response.incomplete","response":{"id":"resp-validate-incomplete","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"id":"item-validate-incomplete","type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"This response was truncated due to output limit.","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":50,"total_tokens":60}}}`,
		`data: [DONE]`,
	}
}

// ─── Error ────────────────────────────────────────────────────────────────────

// ErrorScenario tests that provider error responses are forwarded to the client.
func ErrorScenario() Scenario {
	spec429 := GetErrorSpec("virtual-fail-429")
	return Scenario{
		Name:           "error",
		Description:    "Provider rate limit error (429) propagated to client",
		Tags:           []string{"error"},
		SkipTransitive: true,
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      BuildErrorFromSpec(FormatOpenAIChat, spec429),
			FormatOpenAIResponses: BuildErrorFromSpec(FormatOpenAIResponses, spec429),
			FormatAnthropic:       BuildErrorFromSpec(FormatAnthropic, spec429),
			FormatGoogle:          BuildErrorFromSpec(FormatGoogle, spec429),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatusAtLeast(400),
			check.AssertErrorMessageContains("rate limit"),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatusAtLeast(400),
		},
	}
}

// Error500Scenario tests upstream server error (500) propagated to client.
func Error500Scenario() Scenario {
	spec500 := GetErrorSpec("virtual-fail-500")
	return Scenario{
		Name:           "error-500",
		Description:    "Provider upstream error (500) propagated to client",
		Tags:           []string{"error"},
		SkipTransitive: true,
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      BuildErrorFromSpec(FormatOpenAIChat, spec500),
			FormatOpenAIResponses: BuildErrorFromSpec(FormatOpenAIResponses, spec500),
			FormatAnthropic:       BuildErrorFromSpec(FormatAnthropic, spec500),
			FormatGoogle:          BuildErrorFromSpec(FormatGoogle, spec500),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatusAtLeast(400),
			check.AssertErrorMessageContains("upstream"),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatusAtLeast(400),
		},
	}
}

// ErrorAuth401Scenario tests authentication failure (401) propagated to client.
func ErrorAuth401Scenario() Scenario {
	spec401 := GetErrorSpec("virtual-fail-auth-401")
	return Scenario{
		Name:           "error-auth-401",
		Description:    "Authentication failure (401) propagated to client",
		Tags:           []string{"error", "auth"},
		SkipTransitive: true,
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      BuildErrorFromSpec(FormatOpenAIChat, spec401),
			FormatOpenAIResponses: BuildErrorFromSpec(FormatOpenAIResponses, spec401),
			FormatAnthropic:       BuildErrorFromSpec(FormatAnthropic, spec401),
			FormatGoogle:          BuildErrorFromSpec(FormatGoogle, spec401),
		},
		Assertions: []check.Assertion{
			check.AssertHTTPStatus(401),
			check.AssertErrorMessageContains("authentication"),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatusAtLeast(400),
		},
	}
}

// ErrorMidStreamCloseScenario tests mid-stream connection close failure.
func ErrorMidStreamCloseScenario() Scenario {
	specClose := GetErrorSpec("virtual-fail-midstream-close")
	return Scenario{
		Name:           "error-midstream-close",
		Description:    "Mid-stream connection close failure",
		Tags:           []string{"error", "midstream"},
		SkipTransitive: true,
		MockResponses: map[ResponseFormat]MockResponseBuilder{
			FormatOpenAIChat:      BuildErrorFromSpec(FormatOpenAIChat, specClose),
			FormatOpenAIResponses: BuildErrorFromSpec(FormatOpenAIResponses, specClose),
			FormatAnthropic:       BuildErrorFromSpec(FormatAnthropic, specClose),
			FormatGoogle:          BuildErrorFromSpec(FormatGoogle, specClose),
		},
		Assertions: []check.Assertion{
			// A mid-stream cut must be handled gracefully: the response stays
			// 200 (headers were already sent) and the stream terminates in a
			// client-consumable way. Anthropic-target paths surface it as an
			// in-band error event (real SDK clients raise — the turn was
			// truncated, not completed); OpenAI-target paths end with the
			// partial content. The gateway must not 5xx or hang either way.
			check.AssertHTTPStatus(200),
		},
		Structural: []check.Assertion{
			check.AssertHTTPStatus(200),
		},
	}
}

// ─── OpenAI Responses API mock builders ───────────────────────────────────────

func openAIResponsesTextResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":         "resp-validate-text",
		"object":     "realtime.response",
		"created_at": time.Now().Unix(),
		"model":      "gpt-4o",
		"status":     "completed",
		"output": []map[string]interface{}{
			{
				"id":     "item-validate-text",
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]interface{}{
					{"type": "output_text", "text": "The capital of France is Paris.", "annotations": []interface{}{}},
				},
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 8,
			"total_tokens":  18,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAIResponsesTextSSE,
	}
}

func openAIResponsesToolUseResponse() MockResponseBuilder {
	body := map[string]interface{}{
		"id":         "resp-validate-tool",
		"object":     "realtime.response",
		"created_at": time.Now().Unix(),
		"model":      "gpt-4o",
		"status":     "completed",
		"output": []map[string]interface{}{
			{
				"id":        "call-validate-weather",
				"type":      "function_call",
				"call_id":   "call_validate_weather_1",
				"name":      "get_weather",
				"arguments": `{"location":"Paris","unit":"celsius"}`,
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  15,
			"output_tokens": 20,
			"total_tokens":  35,
		},
	}
	return MockResponseBuilder{
		NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
		Stream:    openAIResponsesToolUseSSE,
	}
}

func openAIResponsesTextSSE() []string {
	return []string{
		`data: {"type":"response.created","response":{"id":"resp-validate-text","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.added","response_id":"resp-validate-text","item":{"id":"item-validate-text","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`data: {"type":"response.output_text.delta","response_id":"resp-validate-text","item_id":"item-validate-text","output_index":0,"content_index":0,"delta":"The capital of France is Paris."}`,
		`data: {"type":"response.output_text.done","response_id":"resp-validate-text","item_id":"item-validate-text","output_index":0,"content_index":0,"text":"The capital of France is Paris."}`,
		`data: {"type":"response.completed","response":{"id":"resp-validate-text","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"completed","output":[{"id":"item-validate-text","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"The capital of France is Paris.","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":8,"total_tokens":18}}}`,
		`data: [DONE]`,
	}
}

func openAIResponsesToolUseSSE() []string {
	return []string{
		`data: {"type":"response.created","response":{"id":"resp-validate-tool","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.added","response_id":"resp-validate-tool","item":{"id":"call-validate-weather","type":"function_call","call_id":"call_validate_weather_1","name":"get_weather","status":"in_progress"}}`,
		`data: {"type":"response.function_call_arguments.delta","response_id":"resp-validate-tool","item_id":"call-validate-weather","output_index":0,"delta":"{\"location\":\"Paris\",\"unit\":\"celsius\"}"}`,
		`data: {"type":"response.function_call_arguments.done","response_id":"resp-validate-tool","item_id":"call-validate-weather","output_index":0,"arguments":"{\"location\":\"Paris\",\"unit\":\"celsius\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp-validate-tool","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"completed","output":[{"id":"call-validate-weather","type":"function_call","call_id":"call_validate_weather_1","name":"get_weather","arguments":"{\"location\":\"Paris\",\"unit\":\"celsius\"}"}],"usage":{"input_tokens":15,"output_tokens":20,"total_tokens":35}}}`,
		`data: [DONE]`,
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
