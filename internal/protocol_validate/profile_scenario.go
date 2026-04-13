package protocol_validate

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server_validate"
)

// ProfileScenario defines a test scenario for profile testing
// Unlike matrix scenarios which test protocol transformations,
// profile scenarios test client implementations (OAuth, path rewriting, etc.)
type ProfileScenario struct {
	// Name is the scenario name
	Name string

	// Description describes what the scenario tests
	Description string

	// MockResponses are the mock responses keyed by API style
	MockResponses map[server_validate.APIStyle]server_validate.MockResponseBuilder
}

// ProfileScenarios returns all built-in profile test scenarios
func ProfileScenarios() []ProfileScenario {
	return []ProfileScenario{
		ProfileTextScenario(),
		ProfileStreamingTextScenario(),
		ProfileToolUseScenario(),
	}
}

// ProfileTextScenario returns a scenario for basic text request/response
func ProfileTextScenario() ProfileScenario {
	return ProfileScenario{
		Name:        "text",
		Description: "Basic text message request and response",
		MockResponses: map[server_validate.APIStyle]server_validate.MockResponseBuilder{
			protocol.APIStyleAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{
						"id": "msg_123",
						"type": "message",
						"role": "assistant",
						"content": [
							{"type": "text", "text": "Hello, world!"}
						],
						"model": "claude-3-5-sonnet-20241022",
						"stop_reason": "end_turn"
					}`)
				},
				Stream: func() []string {
					return []string{
						"event: message_start",
						"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"role\":\"assistant\"}}",
						"",
						"event: content_block_start",
						"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}",
						"",
						"event: content_block_delta",
						"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}",
						"",
						"event: content_block_stop",
						"data: {\"type\":\"content_block_stop\",\"index\":0}",
						"",
						"event: message_stop",
						"data: {\"type\":\"message_stop\"}",
						"",
					}
				},
			},
			protocol.APIStyleOpenAI: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{
						"id": "chatcmpl-123",
						"object": "chat.completion",
						"created": 1234567890,
						"model": "gpt-4",
						"choices": [{
							"index": 0,
							"message": {"role": "assistant", "content": "Hello, world!"},
							"finish_reason": "stop"
						}],
						"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
					}`)
				},
				Stream: func() []string {
					return []string{
						"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}",
						"",
						"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}",
						"",
						"data: [DONE]",
						"",
					}
				},
			},
		},
	}
}

// ProfileStreamingTextScenario returns a scenario for streaming text response
func ProfileStreamingTextScenario() ProfileScenario {
	scenario := ProfileTextScenario()
	scenario.Name = "streaming"
	scenario.Description = "Streaming text message response"
	return scenario
}

// ProfileToolUseScenario returns a scenario for tool use request/response
func ProfileToolUseScenario() ProfileScenario {
	return ProfileScenario{
		Name:        "tool_use",
		Description: "Tool use request and response",
		MockResponses: map[server_validate.APIStyle]server_validate.MockResponseBuilder{
			protocol.APIStyleAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{
						"id": "msg_456",
						"type": "message",
						"role": "assistant",
						"content": [
							{
								"type": "tool_use",
								"id": "toolu_123",
								"name": "bash",
								"input": {"command": "echo 'hello'"}
							}
						],
						"model": "claude-3-5-sonnet-20241022",
						"stop_reason": "tool_use"
					}`)
				},
				Stream: func() []string {
					return []string{
						"event: message_start",
						"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_456\",\"role\":\"assistant\"}}",
						"",
						"event: content_block_start",
						"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_123\",\"name\":\"bash\"}}",
						"",
						"event: content_block_delta",
						"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"command\\\":\\\"echo}}}",
						"",
						"event: content_block_delta",
						"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\" 'hello'\\\"}\"}}",
						"",
						"event: content_block_stop",
						"data: {\"type\":\"content_block_stop\",\"index\":0}",
						"",
						"event: message_stop",
						"data: {\"type\":\"message_stop\"}",
						"",
					}
				},
			},
			protocol.APIStyleOpenAI: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{
						"id": "chatcmpl-456",
						"object": "chat.completion",
						"created": 1234567890,
						"model": "gpt-4",
						"choices": [{
							"index": 0,
							"message": {
								"role": "assistant",
								"tool_calls": [{
									"id": "call_123",
									"type": "function",
									"function": {
										"name": "bash",
										"arguments": "{\"command\":\"echo 'hello'\"}"
									}
								}]
							},
							"finish_reason": "tool_calls"
						}],
						"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
					}`)
				},
				Stream: func() []string {
					return []string{
						"data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_123\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}",
						"",
						"data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"command\\\":\\\"echo\"}}}]},\"finish_reason\":null}]}",
						"",
						"data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\" 'hello'\\\"}\"}}}],\"finish_reason\":null}]}",
						"",
						"data: {\"id\":\"chatcmpl-456\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}",
						"",
						"data: [DONE]",
						"",
					}
				},
			},
		},
	}
}

// GetScenarioByName returns a profile scenario by name
func GetScenarioByName(name string) (ProfileScenario, bool) {
	for _, s := range ProfileScenarios() {
		if s.Name == name {
			return s, true
		}
	}
	return ProfileScenario{}, false
}
