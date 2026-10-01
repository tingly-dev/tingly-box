package nonstream

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	result, _, err := HandleResponsesToOpenAIChat(protocol.NewHandleContext(c, "proxy-model"), &resp)
	require.NoError(t, err)
	choices := asJSONMapSlice(t, result["choices"])
	message := choices[0]["message"].(map[string]any)
	toolCalls := asJSONMapSlice(t, message["tool_calls"])

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

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			result, _, err := HandleResponsesToOpenAIChat(protocol.NewHandleContext(c, "proxy-model"), &resp)
			require.NoError(t, err)
			choices := asJSONMapSlice(t, result["choices"])
			assert.Equal(t, tt.expectedStop, choices[0]["finish_reason"])
		})
	}
}

func TestConvertResponsesToOpenAIChatPreservesTypedExtensions(t *testing.T) {
	raw := []byte(`{
		"id":"resp_extensions","created_at":1710000000,"model":"gpt-4.1",
		"object":"response","status":"completed",
		"output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"cannot comply"}]}],
		"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}
	}`)
	var resp responses.Response
	require.NoError(t, json.Unmarshal(raw, &resp))

	converted := ConvertResponsesToOpenAIChat(&resp, "public-model")
	require.Len(t, converted.Choices, 1)
	assert.Equal(t, "cannot comply", converted.Choices[0].Message.Refusal)
	require.NotNil(t, converted.Usage.PromptTokensDetails)
	assert.EqualValues(t, 4, converted.Usage.PromptTokensDetails.CachedTokens)
	require.NotNil(t, converted.Usage.CompletionTokensDetails)
	assert.EqualValues(t, 3, converted.Usage.CompletionTokensDetails.ReasoningTokens)

	payload := converted.ToMap()
	choices := asJSONMapSlice(t, payload["choices"])
	message := choices[0]["message"].(map[string]any)
	assert.Equal(t, "cannot comply", message["refusal"])
	usage := payload["usage"].(map[string]any)
	assert.EqualValues(t, 3, usage["completion_tokens_details"].(map[string]any)["reasoning_tokens"])
}

func asJSONMapSlice(t *testing.T, value any) []map[string]any {
	t.Helper()
	items, ok := value.([]any)
	require.Truef(t, ok, "expected JSON array, got %T", value)

	result := make([]map[string]any, len(items))
	for i, item := range items {
		mapped, ok := item.(map[string]any)
		require.Truef(t, ok, "expected JSON object at index %d, got %T", i, item)
		result[i] = mapped
	}
	return result
}

// TestOpenAIResponsesToChatStripUsage proves StripUsage (skip_usage /
// cursor_compat for a Chat client) leaves usage out of the Chat answer while
// the returned token usage still feeds tracking.
func TestOpenAIResponsesToChatStripUsage(t *testing.T) {
	raw := []byte(`{
		"id":"resp_1","created_at":1710000000,"model":"gpt-4.1","object":"response","status":"completed",
		"output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed",
			"content":[{"type":"output_text","text":"hi","annotations":[]}]}],
		"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}
	}`)
	var resp responses.Response
	require.NoError(t, json.Unmarshal(raw, &resp))

	for _, strip := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		hc := protocol.NewHandleContext(c, "proxy-model")
		hc.StripUsage = strip
		_, usage, err := HandleResponsesToOpenAIChat(hc, &resp)
		require.NoError(t, err)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		_, has := body["usage"]
		assert.Equal(t, !strip, has, "strip=%v", strip)
		assert.Equal(t, 5, usage.OutputTokens, "strip=%v", strip)
	}
}
