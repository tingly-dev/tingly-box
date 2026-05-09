package ops

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestUnionLevelExtraFieldsSerialized proves that ExtraFields set on the
// ChatCompletionMessageParamUnion (not on OfAssistant) are included in the
// final JSON output.  This validates that the DeepSeek transform's current
// approach of writing reasoning_content to the union level is correct.
func TestUnionLevelExtraFieldsSerialized(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// Set reasoning_content on the UNION level (not OfAssistant)
	msg.SetExtraFields(map[string]any{"reasoning_content": "planning the tool call"})

	raw := marshalMessage(t, msg)
	assert.Equal(t, "planning the tool call", raw["reasoning_content"],
		"union-level ExtraFields must appear in serialized JSON")
}

// TestOfAssistantLevelExtraFieldsSerialized proves that ExtraFields set on
// the OfAssistant variant are also included in the final JSON output.
func TestOfAssistantLevelExtraFieldsSerialized(t *testing.T) {
	msg := assistantToolCallMessage(t)

	msg.OfAssistant.SetExtraFields(map[string]any{"reasoning_content": "variant-level thinking"})

	raw := marshalMessage(t, msg)
	assert.Equal(t, "variant-level thinking", raw["reasoning_content"],
		"OfAssistant-level ExtraFields must appear in serialized JSON")
}

// TestUnionExtrasOverrideVariantExtras proves that when both levels have
// overlapping keys, the union-level value wins (because MarshalWithExtras
// merges union extras AFTER marshaling the variant).
func TestUnionExtrasOverrideVariantExtras(t *testing.T) {
	msg := assistantToolCallMessage(t)

	msg.OfAssistant.SetExtraFields(map[string]any{"reasoning_content": "from variant"})
	msg.SetExtraFields(map[string]any{"reasoning_content": "from union"})

	raw := marshalMessage(t, msg)
	assert.Equal(t, "from union", raw["reasoning_content"],
		"union-level extras should override variant-level extras")
}

// TestDeepSeekTransformReadsUnionLevelXThinking proves that the DeepSeek
// transform correctly reads x_thinking from the union level (where
// Anthropic→OpenAI conversion stores it) and writes reasoning_content back
// to the union level.
func TestDeepSeekTransformReadsUnionLevelXThinking(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// Simulate what anthropic_v1_to_openai.go does: store x_thinking on union
	msg.SetExtraFields(map[string]any{"x_thinking": "I need to call a tool"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("deepseek-v4-flash"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://api.deepseek.com/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "I need to call a tool", raw["reasoning_content"],
		"x_thinking should be converted to reasoning_content")
	assert.NotContains(t, raw, "x_thinking",
		"x_thinking should be removed after conversion")
}

// TestDeepSeekTransformFallbackEmptyReasoningContent proves that when no
// x_thinking is present, the transform sets an empty reasoning_content
// string on the union level, and it appears in the serialized output.
func TestDeepSeekTransformFallbackEmptyReasoningContent(t *testing.T) {
	msg := assistantToolCallMessage(t)
	// No x_thinking set — simulate assistant message that never had thinking

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("deepseek-v4-flash"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://api.deepseek.com/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	_, hasKey := raw["reasoning_content"]
	assert.True(t, hasKey, "reasoning_content must be present in JSON output")
	assert.Equal(t, "", raw["reasoning_content"],
		"reasoning_content should be empty string when no thinking content")
}

// TestAnthropicConversionStoresXThinkingOnUnionLevel proves that the
// Anthropic→OpenAI conversion path stores x_thinking at the union level,
// matching the DeepSeek transform's read location.
func TestAnthropicConversionStoresXThinkingOnUnionLevel(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// Reproduce the exact pattern from anthropic_v1_to_openai.go:
	//   extraFields := result.ExtraFields()
	//   extraFields["x_thinking"] = thinking
	//   result.SetExtraFields(extraFields)
	extraFields := msg.ExtraFields()
	if extraFields == nil {
		extraFields = map[string]any{}
	}
	extraFields["x_thinking"] = "model thought process"
	msg.SetExtraFields(extraFields)

	// Verify it's stored at union level (not OfAssistant)
	unionExtras := msg.ExtraFields()
	assert.NotNil(t, unionExtras, "union-level ExtraFields should not be nil")
	assert.Equal(t, "model thought process", unionExtras["x_thinking"],
		"x_thinking should be readable from union level")

	// Verify it appears in serialized JSON
	raw := marshalMessage(t, msg)
	assert.Equal(t, "model thought process", raw["x_thinking"],
		"x_thinking stored at union level must appear in JSON")
}

// --- helpers ---

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
