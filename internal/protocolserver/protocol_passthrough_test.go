package protocolserver

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chatResponseMapFrom mirrors the production path in nonstreamOpenAIChat: the
// SDK response object is re-serialized with encoding/json and post-processed
// as a generic map. That round-trip is where unknown message fields die — the
// extras-carrying JSON metadata struct is tagged json:"-" — which is the bug
// under test.
func chatResponseMapFrom(t *testing.T, resp *openai.ChatCompletion) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var responseMap map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &responseMap))
	return responseMap
}

// firstChatMessage digs the first choice's message map out of a re-serialized
// response map.
func firstChatMessage(t *testing.T, responseMap map[string]interface{}) map[string]interface{} {
	t.Helper()
	choices, ok := responseMap["choices"].([]interface{})
	require.True(t, ok, "choices must be a JSON array")
	require.NotEmpty(t, choices)
	message, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	require.True(t, ok, "choice must carry a message object")
	return message
}

// TestNormalizeOpenAIChatReasoningExtras: the non-streaming passthrough
// re-serializes the SDK ChatCompletion with plain json.Marshal, which drops
// every unknown message field, so upstream thinking text in either spelling
// never reaches the client (#1773). The normalizer must restore it as
// message.reasoning_content — without touching responses that carry no
// reasoning at all.
func TestNormalizeOpenAIChatReasoningExtras(t *testing.T) {
	t.Run("openai spelling is normalized", func(t *testing.T) {
		var resp openai.ChatCompletion
		require.NoError(t, json.Unmarshal([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1000,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "390",
					"reasoning": "Let me think about this carefully.",
					"reasoning_details": [{"type": "reasoning", "text": "Planning the arithmetic."}]
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30,
				"completion_tokens_details": {"reasoning_tokens": 15}
			}
		}`), &resp))

		responseMap := chatResponseMapFrom(t, &resp)

		// The bug being reproduced: the text survives the SDK decode
		// (captured into message extras)…
		require.Contains(t, resp.Choices[0].Message.JSON.ExtraFields, "reasoning")
		assert.Contains(t, resp.Choices[0].Message.JSON.ExtraFields["reasoning"].Raw(), "Let me think about this carefully.")
		// …but not the plain re-serialization that nonstreamOpenAIChat does.
		assert.NotContains(t, firstChatMessage(t, responseMap), "reasoning_content")

		normalizeOpenAIChatReasoningExtras(&resp, responseMap)

		// When both spellings are present, reasoning wins over
		// reasoning_details (full text beats a detail fragment).
		assert.Equal(t, "Let me think about this carefully.", firstChatMessage(t, responseMap)["reasoning_content"])
	})

	t.Run("reasoning_details only is concatenated", func(t *testing.T) {
		var resp openai.ChatCompletion
		require.NoError(t, json.Unmarshal([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1000,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "390",
					"reasoning_details": [
						{"type": "reasoning", "text": "Planning."},
						{"type": "reasoning", "text": "Executing."}
					]
				},
				"finish_reason": "stop"
			}]
		}`), &resp))

		responseMap := chatResponseMapFrom(t, &resp)
		normalizeOpenAIChatReasoningExtras(&resp, responseMap)

		assert.Equal(t, "Planning.Executing.", firstChatMessage(t, responseMap)["reasoning_content"])
	})

	t.Run("deepseek spelling passes through", func(t *testing.T) {
		var resp openai.ChatCompletion
		require.NoError(t, json.Unmarshal([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1000,
			"model": "deepseek-chat",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "390",
					"reasoning_content": "Let me think about this carefully."
				},
				"finish_reason": "stop"
			}]
		}`), &resp))

		responseMap := chatResponseMapFrom(t, &resp)
		assert.NotContains(t, firstChatMessage(t, responseMap), "reasoning_content")

		normalizeOpenAIChatReasoningExtras(&resp, responseMap)

		assert.Equal(t, "Let me think about this carefully.", firstChatMessage(t, responseMap)["reasoning_content"])
	})

	t.Run("no reasoning leaves the map untouched", func(t *testing.T) {
		var resp openai.ChatCompletion
		require.NoError(t, json.Unmarshal([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1000,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "390"},
				"finish_reason": "stop"
			}]
		}`), &resp))

		responseMap := chatResponseMapFrom(t, &resp)
		normalizeOpenAIChatReasoningExtras(&resp, responseMap)

		// The passthrough must not fabricate an empty reasoning_content on
		// responses that never carried one (only the DeepSeek vendor
		// transform does that, deliberately).
		assert.NotContains(t, firstChatMessage(t, responseMap), "reasoning_content")
	})
}
