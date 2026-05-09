package ops

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestDeepSeekTransformWritesReasoningContentToAssistantVariant(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// The SDK marshals the concrete OfAssistant variant, so union-level extras
	// are ignored when OfAssistant is present.
	msg.SetExtraFields(map[string]any{"reasoning_content": "ignored"})
	raw := marshalMessage(t, msg)
	_, hasReasoning := raw["reasoning_content"]
	require.False(t, hasReasoning, "union-level reasoning_content should not be serialized")

	// Anthropic -> OpenAI conversion preserves thinking on the union, which the
	// DeepSeek transform must move to the assistant variant before forwarding.
	msg.OfAssistant.SetExtraFields(map[string]any{"vendor_marker": "keep"})
	msg.SetExtraFields(map[string]any{"x_thinking": "tool planning"})
	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("deepseek-v4-flash"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://opencode.ai/zen/go/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw = marshalMessage(t, req.Messages[0])
	assert.Equal(t, "tool planning", raw["reasoning_content"])
	assert.Equal(t, "keep", raw["vendor_marker"])
	assert.NotContains(t, raw, "x_thinking")
}

func assistantToolCallMessage(t *testing.T) openai.ChatCompletionMessageParamUnion {
	t.Helper()

	msgMap := map[string]any{
		"role":    "assistant",
		"content": "",
		"tool_calls": []map[string]any{
			{
				"id":   "call_123",
				"type": "function",
				"function": map[string]any{
					"name":      "lookup",
					"arguments": `{"q":"hello"}`,
				},
			},
		},
	}

	msgBytes, err := json.Marshal(msgMap)
	require.NoError(t, err)

	var msg openai.ChatCompletionMessageParamUnion
	require.NoError(t, json.Unmarshal(msgBytes, &msg))
	require.NotNil(t, msg.OfAssistant)
	return msg
}

func marshalMessage(t *testing.T, msg openai.ChatCompletionMessageParamUnion) map[string]any {
	t.Helper()

	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(msgBytes, &raw))
	return raw
}
