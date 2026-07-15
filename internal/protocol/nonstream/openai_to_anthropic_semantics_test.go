package nonstream

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicUsesCallID(t *testing.T) {
	var response responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"resp_tool","object":"response","status":"completed","model":"provider-model",
		"output":[{"id":"fc_item","call_id":"call_weather","type":"function_call","name":"get_weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`), &response))

	beta := HandleResponsesToAnthropicBeta(&response, "public-model")
	require.Len(t, beta.Content, 1)
	assert.Equal(t, "tool_use", beta.Content[0].Type)
	assert.Equal(t, "call_weather", beta.Content[0].ID)

	v1 := HandleResponsesToAnthropicV1(&response, "public-model")
	require.Len(t, v1.Content, 1)
	assert.Equal(t, "tool_use", v1.Content[0].Type)
	assert.Equal(t, "call_weather", v1.Content[0].ID)
}

func TestCompleteConversionsPreserveTruncation(t *testing.T) {
	chat := &openai.ChatCompletion{
		ID: "chatcmpl-length",
		Choices: []openai.ChatCompletionChoice{{
			FinishReason: "length",
			Message:      openai.ChatCompletionMessage{Role: "assistant", Content: "partial"},
		}},
	}
	v1, err := ConvertOpenAIChatToAnthropicV1(chat, "public-model")
	require.NoError(t, err)
	assert.Equal(t, "max_tokens", string(v1.StopReason))
	beta, err := ConvertOpenAIChatToAnthropicBeta(chat, "public-model")
	require.NoError(t, err)
	assert.Equal(t, "max_tokens", string(beta.StopReason))

	var response responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"resp_incomplete","object":"response","status":"incomplete","model":"provider-model",
		"incomplete_details":{"reason":"max_output_tokens"},
		"output":[{"id":"msg_1","type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"partial","annotations":[]}]}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`), &response))
	responsesV1 := HandleResponsesToAnthropicV1(&response, "public-model")
	assert.Equal(t, "max_tokens", string(responsesV1.StopReason))
	responsesBeta := HandleResponsesToAnthropicBeta(&response, "public-model")
	assert.Equal(t, "max_tokens", string(responsesBeta.StopReason))
}

func TestChatToAnthropicSyntheticIDsAreUnique(t *testing.T) {
	chat := &openai.ChatCompletion{
		ID: "chatcmpl-source",
		Choices: []openai.ChatCompletionChoice{{
			FinishReason: "stop",
			Message:      openai.ChatCompletionMessage{Role: "assistant", Content: "hello"},
		}},
	}
	first, err := ConvertOpenAIChatToAnthropicV1(chat, "public-model")
	require.NoError(t, err)
	second, err := ConvertOpenAIChatToAnthropicV1(chat, "public-model")
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
}
