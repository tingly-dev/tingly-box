package server_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server_validate"
)

// newTextScenario returns a minimal text scenario for direct virtual server tests.
func newTextScenario() server_validate.Scenario {
	return server_validate.Scenario{
		Name: "text",
		MockResponses: map[server_validate.APIStyle]server_validate.MockResponseBuilder{
			server_validate.StyleOpenAI: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"The capital of France is Paris."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}}`)
				},
				Stream: func() []string {
					return []string{
						`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
						`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"The capital of France is Paris."},"finish_reason":null}]}`,
						`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
						`data: [DONE]`,
					}
				},
			},
			server_validate.StyleAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"msg-test","type":"message","role":"assistant","content":[{"type":"text","text":"The capital of France is Paris."}],"model":"claude-3-5-sonnet-20241022","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":8}}`)
				},
				Stream: func() []string {
					return []string{
						`event: message_start`,
						`data: {"type":"message_start","message":{"id":"msg-test","role":"assistant","model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":10,"output_tokens":0}}}`,
						`event: content_block_delta`,
						`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The capital of France is Paris."}}`,
						`event: message_stop`,
						`data: {"type":"message_stop"}`,
					}
				},
			},
			server_validate.StyleGoogle: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"The capital of France is Paris."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":8}}`)
				},
				Stream: func() []string {
					return []string{
						`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"The capital of France is Paris."}]},"finishReason":"STOP"}]}`,
					}
				},
			},
		},
	}
}

func newToolUseScenario() server_validate.Scenario {
	return server_validate.Scenario{
		Name: "tool_use",
		MockResponses: map[server_validate.APIStyle]server_validate.MockResponseBuilder{
			server_validate.StyleOpenAI: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"chatcmpl-tool","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":15,"completion_tokens":20}}`)
				},
			},
			server_validate.StyleAnthropic: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"id":"msg-tool","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"location":"Paris"}}],"model":"claude-3-5-sonnet-20241022","stop_reason":"tool_use","usage":{"input_tokens":15,"output_tokens":20}}`)
				},
			},
			server_validate.StyleGoogle: {
				NonStream: func() (int, []byte) {
					return 200, []byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"location":"Paris"}}}]},"finishReason":"STOP"}]}`)
				},
			},
		},
	}
}

func newErrorScenario() server_validate.Scenario {
	return server_validate.Scenario{
		Name: "error",
		MockResponses: map[server_validate.APIStyle]server_validate.MockResponseBuilder{
			server_validate.StyleOpenAI: {
				NonStream: func() (int, []byte) {
					return 429, []byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`)
				},
			},
		},
	}
}

func TestServerValidate_Lifecycle(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	require.NotNil(t, vs)
	defer vs.Close()

	url := vs.URL()
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "http://")
}

func TestServerValidate_OpenAI_ChatCompletions(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	result := vs.SendOpenAIChat(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "stop", result.FinishReason)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

func TestServerValidate_OpenAI_ChatCompletions_Streaming(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	result := vs.SendOpenAIChat(t, s, true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}

func TestServerValidate_Anthropic_Messages(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	result := vs.SendAnthropicV1(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
}

func TestServerValidate_Anthropic_Messages_Streaming(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	result := vs.SendAnthropicV1(t, s, true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}

func TestServerValidate_Google_GenerateContent(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	result := vs.SendGoogle(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.Content)
}

func TestServerValidate_ToolUse_OpenAI(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newToolUseScenario()
	result := vs.SendOpenAIChat(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestServerValidate_ToolUse_Anthropic(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newToolUseScenario()
	result := vs.SendAnthropicV1(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestServerValidate_ErrorResponse(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newErrorScenario()
	result := vs.SendOpenAIChat(t, s, false)
	assert.Equal(t, 429, result.HTTPStatus)
}

func TestServerValidate_CallCount(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	defer vs.Close()

	s := newTextScenario()
	vs.SendOpenAIChat(t, s, false)
	vs.SendOpenAIChat(t, s, false)
	assert.Equal(t, 2, vs.CallCount())
}
