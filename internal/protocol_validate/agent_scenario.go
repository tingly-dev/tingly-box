package protocol_validate

import (
	"github.com/tingly-dev/tingly-box/internal/server_validate"
)

// AgentScenario defines a test scenario for profile testing
// Unlike matrix scenarios which test protocol transformations,
// profile scenarios test client implementations (OAuth, path rewriting, etc.)
type AgentScenario struct {
	// Name is the scenario name
	Name string

	// Description describes what the scenario tests
	Description string

	// MockResponses are the mock responses keyed by API style
	MockResponses map[server_validate.ResponseFormat]server_validate.MockResponseBuilder
}

// AgentScenarios returns all built-in profile test scenarios
func AgentScenarios() []AgentScenario {
	return []AgentScenario{
		AgentTextScenario(),
		AgentStreamingTextScenario(),
		AgentToolUseScenario(),
	}
}

// AgentTextScenario returns a scenario for basic text request/response
func AgentTextScenario() AgentScenario {
	return AgentScenario{
		Name:        "text",
		Description: "Basic text message request and response",
		MockResponses: map[server_validate.ResponseFormat]server_validate.MockResponseBuilder{
			server_validate.FormatAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"model":"claude-3-5-sonnet-20241022","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":5}}`)
				},
				Stream: func() []string {
					return []string{
						"event: message_start",
						`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
						"",
						"event: content_block_start",
						`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
						"",
						"event: ping",
						`data: {"type":"ping"}`,
						"",
						"event: content_block_delta",
						`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
						"",
						"event: content_block_stop",
						`data: {"type":"content_block_stop","index":0}`,
						"",
						"event: message_delta",
						`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
						"",
						"event: message_stop",
						`data: {"type":"message_stop"}`,
						"",
					}
				},
			},
			server_validate.FormatOpenAIChat: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"chatcmpl-123","object":"chat.completion","created":1234567890,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hello, world!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
				},
				Stream: func() []string {
					return []string{
						`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
						"",
						`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
						"",
						"data: [DONE]",
						"",
					}
				},
			},
		},
	}
}

// AgentStreamingTextScenario returns a scenario for streaming text response
func AgentStreamingTextScenario() AgentScenario {
	scenario := AgentTextScenario()
	scenario.Name = "streaming"
	scenario.Description = "Streaming text message response"
	return scenario
}

// AgentToolUseScenario returns a scenario for tool use request/response
func AgentToolUseScenario() AgentScenario {
	return AgentScenario{
		Name:        "tool_use",
		Description: "Tool use request and response",
		MockResponses: map[server_validate.ResponseFormat]server_validate.MockResponseBuilder{
			server_validate.FormatAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"msg_456","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"bash","input":{"command":"echo 'hello'"}}],"model":"claude-3-5-sonnet-20241022","stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":20}}`)
				},
				Stream: func() []string {
					return []string{
						"event: message_start",
						`data: {"type":"message_start","message":{"id":"msg_456","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
						"",
						"event: content_block_start",
						`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"bash","input":{}}}`,
						"",
						"event: content_block_delta",
						`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"echo"}}`,
						"",
						"event: content_block_delta",
						`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" 'hello'\"}"}}`,
						"",
						"event: content_block_stop",
						`data: {"type":"content_block_stop","index":0}`,
						"",
						"event: message_delta",
						`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":20}}`,
						"",
						"event: message_stop",
						`data: {"type":"message_stop"}`,
						"",
					}
				},
			},
			server_validate.FormatOpenAIChat: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"chatcmpl-456","object":"chat.completion","created":1234567890,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"bash","arguments":"{\"command\":\"echo 'hello'\"}"}}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
				},
				Stream: func() []string {
					return []string{
						`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}`,
						"",
						`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"echo"}}]},"finish_reason":null}]}`,
						"",
						`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" 'hello'\""}}]},"finish_reason":null}]}`,
						"",
						`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
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
func GetScenarioByName(name string) (AgentScenario, bool) {
	for _, s := range AgentScenarios() {
		if s.Name == name {
			return s, true
		}
	}
	return AgentScenario{}, false
}
