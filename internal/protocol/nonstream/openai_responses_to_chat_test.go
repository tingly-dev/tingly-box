package nonstream

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesToChatToolCalls(t *testing.T) {
	raw := []byte(`{
		"id":"resp_123",
		"created_at":1710000000,
		"model":"gpt-4.1",
		"object":"response",
		"status":"completed",
		"parallel_tool_calls":true,
		"tool_choice":"auto",
		"tools":[],
		"temperature":1,
		"top_p":1,
		"text":{"format":{"type":"text"}},
		"output":[
			{
				"id":"msg_1",
				"type":"message",
				"role":"assistant",
				"status":"completed",
				"content":[{"type":"output_text","text":"Let me check.","annotations":[]}]
			},
			{
				"id":"fc_1",
				"type":"function_call",
				"call_id":"call_1",
				"name":"get_weather",
				"arguments":"{\"location\":\"Tokyo\"}",
				"status":"completed"
			}
		],
		"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	var resp responses.Response
	require.NoError(t, json.Unmarshal(raw, &resp))

	result := OpenAIResponsesToChat(&resp, "proxy-model")
	choices := result["choices"].([]map[string]any)
	message := choices[0]["message"].(map[string]any)
	toolCalls := message["tool_calls"].([]map[string]any)

	assert.Equal(t, "tool_calls", choices[0]["finish_reason"])
	assert.Equal(t, "assistant", message["role"])
	assert.Equal(t, "Let me check.", message["content"])
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0]["id"])
	assert.Equal(t, "function", toolCalls[0]["type"])

	function := toolCalls[0]["function"].(map[string]any)
	assert.Equal(t, "get_weather", function["name"])
	assert.Equal(t, `{"location":"Tokyo"}`, function["arguments"])
}

func TestOpenAIResponsesToChatIncompleteReasons(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		reason       string
		expectedStop string
	}{
		{name: "max output tokens", status: "incomplete", reason: "max_output_tokens", expectedStop: "length"},
		{name: "content filter", status: "incomplete", reason: "content_filter", expectedStop: "content_filter"},
		{name: "completed", status: "completed", reason: "", expectedStop: "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(`{
				"id":"resp_456",
				"created_at":1710000000,
				"model":"gpt-4.1",
				"object":"response",
				"status":"` + tt.status + `",
				"incomplete_details":{"reason":"` + tt.reason + `"},
				"parallel_tool_calls":false,
				"tool_choice":"auto",
				"tools":[],
				"temperature":1,
				"top_p":1,
				"text":{"format":{"type":"text"}},
				"output":[
					{
						"id":"msg_1",
						"type":"message",
						"role":"assistant",
						"status":"completed",
						"content":[{"type":"output_text","text":"Partial text","annotations":[]}]
					}
				],
				"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
			}`)

			var resp responses.Response
			require.NoError(t, json.Unmarshal(raw, &resp))

			result := OpenAIResponsesToChat(&resp, "proxy-model")
			choices := result["choices"].([]map[string]any)
			assert.Equal(t, tt.expectedStop, choices[0]["finish_reason"])
		})
	}
}
