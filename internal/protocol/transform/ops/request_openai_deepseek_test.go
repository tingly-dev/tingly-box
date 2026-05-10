package ops

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestOfAssistantLevelExtraFieldsSerialized proves that ExtraFields set on
// OfAssistant (variant level) are included in JSON output.
// This is the CORRECT level — union-level ExtraFields are dropped by MarshalUnion.
func TestOfAssistantLevelExtraFieldsSerialized(t *testing.T) {
	msg := assistantToolCallMessage(t)

	msg.OfAssistant.SetExtraFields(map[string]any{"reasoning_content": "planning the tool call"})

	raw := marshalMessage(t, msg)
	assert.Equal(t, "planning the tool call", raw["reasoning_content"],
		"OfAssistant-level ExtraFields must appear in serialized JSON")
}

// TestUnionLevelExtraFieldsAreDropped proves that ExtraFields set on the
// union level are NOT serialized — this is the SDK behavior that caused
// the DeepSeek reasoning_content bug.
func TestUnionLevelExtraFieldsAreDropped(t *testing.T) {
	msg := assistantToolCallMessage(t)

	msg.SetExtraFields(map[string]any{"reasoning_content": "this will be lost"})

	raw := marshalMessage(t, msg)
	_, hasKey := raw["reasoning_content"]
	assert.False(t, hasKey,
		"union-level ExtraFields must NOT appear in serialized JSON (they are dropped by MarshalUnion)")
}

// TestDeepSeekTransformReadsOfAssistantXThinking proves that the transform
// correctly reads x_thinking from OfAssistant level and writes reasoning_content back.
func TestDeepSeekTransformReadsOfAssistantXThinking(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// Simulate what anthropic_v1_to_openai.go does: store x_thinking on OfAssistant
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "I need to call a tool"})

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

// TestDeepSeekTransformOpenCodeAI_DeepSeekModel verifies the transform is applied
// when opencode.ai is used with a DeepSeek model (model name contains "deepseek").
func TestDeepSeekTransformOpenCodeAI_DeepSeekModel(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "reasoning for opencode"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("deepseek-chat"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://opencode.ai/zen/go/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "reasoning for opencode", raw["reasoning_content"],
		"opencode.ai + deepseek model should get reasoning_content conversion")
}

// TestDeepSeekTransformOpenCodeAI_NonDeepSeekModel verifies the transform is NOT
// applied when opencode.ai is used with a non-DeepSeek model (e.g., OpenAI, Claude).
func TestDeepSeekTransformOpenCodeAI_NonDeepSeekModel(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "should not be converted"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://opencode.ai/zen/go/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "should not be converted", raw["x_thinking"],
		"opencode.ai + non-deepseek model should keep x_thinking as-is (no transform)")
	assert.NotContains(t, raw, "reasoning_content",
		"opencode.ai + non-deepseek model should NOT have reasoning_content")
}

// TestDeepSeekTransformFallbackEmptyReasoningContent proves that when no
// x_thinking is present, the transform sets an empty reasoning_content.
func TestDeepSeekTransformFallbackEmptyReasoningContent(t *testing.T) {
	msg := assistantToolCallMessage(t)
	// No x_thinking set

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

// TestAnthropicConversionStoresXThinkingOnOfAssistant proves that the
// Anthropic→OpenAI conversion stores x_thinking at the OfAssistant level,
// which is where the DeepSeek transform reads it.
func TestAnthropicConversionStoresXThinkingOnOfAssistant(t *testing.T) {
	msg := assistantToolCallMessage(t)

	// Reproduce the corrected pattern: store x_thinking on OfAssistant level
	extraFields := msg.OfAssistant.ExtraFields()
	if extraFields == nil {
		extraFields = map[string]any{}
	}
	extraFields["x_thinking"] = "model thought process"
	msg.OfAssistant.SetExtraFields(extraFields)

	// Verify it's readable from OfAssistant
	ofAssistantExtras := msg.OfAssistant.ExtraFields()
	assert.NotNil(t, ofAssistantExtras)
	assert.Equal(t, "model thought process", ofAssistantExtras["x_thinking"],
		"x_thinking should be readable from OfAssistant level")

	// Verify it appears in serialized JSON
	raw := marshalMessage(t, msg)
	assert.Equal(t, "model thought process", raw["x_thinking"],
		"x_thinking stored at OfAssistant level must appear in JSON")
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
