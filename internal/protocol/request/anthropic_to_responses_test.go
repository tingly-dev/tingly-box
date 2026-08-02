package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertAnthropicBetaToResponsesRequest_ModelConversion tests the bugfix for missing model field conversion
func TestConvertAnthropicBetaToResponsesRequest_ModelConversion(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedModel string
	}{
		{
			name:          "claude-3-5-sonnet-latest",
			model:         "claude-3-5-sonnet-latest",
			expectedModel: "claude-3-5-sonnet-latest",
		},
		{
			name:          "claude-3-5-haiku-latest",
			model:         "claude-3-5-haiku-latest",
			expectedModel: "claude-3-5-haiku-latest",
		},
		{
			name:          "claude-3-opus-latest",
			model:         "claude-3-opus-latest",
			expectedModel: "claude-3-opus-latest",
		},
		{
			name:          "custom model name",
			model:         "custom-model-v1",
			expectedModel: "custom-model-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anthropicReq := &anthropic.BetaMessageNewParams{
				Model: anthropic.Model(tt.model),
				Messages: []anthropic.BetaMessageParam{
					anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Hello")),
				},
			}

			result := ConvertAnthropicBetaToResponsesRequest(anthropicReq)

			// Verify model field is properly converted (bugfix: was missing before)
			assert.Equal(t, tt.expectedModel, string(result.Model))
		})
	}
}

// TestConvertAnthropicV1ToResponsesRequest_ModelConversion tests the bugfix for missing model field conversion in v1
func TestConvertAnthropicV1ToResponsesRequest_ModelConversion(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedModel string
	}{
		{
			name:          "claude-3-5-sonnet-20241022",
			model:         "claude-3-5-sonnet-20241022",
			expectedModel: "claude-3-5-sonnet-20241022",
		},
		{
			name:          "claude-3-5-haiku-20241022",
			model:         "claude-3-5-haiku-20241022",
			expectedModel: "claude-3-5-haiku-20241022",
		},
		{
			name:          "claude-3-opus-20240229",
			model:         "claude-3-opus-20240229",
			expectedModel: "claude-3-opus-20240229",
		},
		{
			name:          "custom model name",
			model:         "custom-model-v1",
			expectedModel: "custom-model-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anthropicReq := &anthropic.MessageNewParams{
				Model: anthropic.Model(tt.model),
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
				},
			}

			result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)

			// Verify model field is properly converted (bugfix: was missing before)
			assert.Equal(t, tt.expectedModel, string(result.Model))
		})
	}
}

// TestConvertAnthropicBetaToResponsesRequest_FullConversion tests the complete conversion including model
func TestConvertAnthropicBetaToResponsesRequest_FullConversion(t *testing.T) {
	anthropicReq := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-latest"),
		MaxTokens: 2048,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("What is the weather?")),
		},
		System: []anthropic.BetaTextBlockParam{
			{Text: "You are a helpful assistant."},
		},
	}

	result := ConvertAnthropicBetaToResponsesRequest(anthropicReq)

	// Verify model is set (the bugfix)
	assert.Equal(t, "claude-3-5-sonnet-latest", string(result.Model))

	// Verify other fields are also converted
	assert.NotNil(t, result.Instructions)
	assert.Equal(t, "You are a helpful assistant.", result.Instructions.Value)

	assert.NotNil(t, result.MaxOutputTokens)
	assert.Equal(t, int64(2048), result.MaxOutputTokens.Value)
}

// TestConvertAnthropicV1ToResponsesRequest_FullConversion tests the complete v1 conversion including model
func TestConvertAnthropicV1ToResponsesRequest_FullConversion(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello, world!")),
		},
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		},
	}

	result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)

	// Verify model is set (the bugfix)
	assert.Equal(t, "claude-3-5-sonnet-20241022", string(result.Model))

	// Verify other fields are also converted
	assert.NotNil(t, result.Instructions)
	assert.Equal(t, "You are a helpful assistant.", result.Instructions.Value)

	assert.NotNil(t, result.MaxOutputTokens)
	assert.Equal(t, int64(4096), result.MaxOutputTokens.Value)
}

// TestConvertAnthropicBetaToResponsesRequest_WithTemperature tests conversion with temperature
func TestConvertAnthropicBetaToResponsesRequest_WithTemperature(t *testing.T) {
	anthropicReq := &anthropic.BetaMessageNewParams{
		Model:       anthropic.Model("claude-3-5-sonnet-latest"),
		Messages:    []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Test"))},
		Temperature: anthropic.Opt(0.7),
	}

	result := ConvertAnthropicBetaToResponsesRequest(anthropicReq)

	// Verify model is set
	assert.Equal(t, "claude-3-5-sonnet-latest", string(result.Model))

	// Verify temperature is converted
	assert.NotNil(t, result.Temperature)
	assert.Equal(t, 0.7, result.Temperature.Value)
}

// TestConvertAnthropicV1ToResponsesRequest_WithTemperature tests v1 conversion with temperature
func TestConvertAnthropicV1ToResponsesRequest_WithTemperature(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:       anthropic.Model("claude-3-5-sonnet-20241022"),
		Messages:    []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("Test"))},
		Temperature: anthropic.Opt(0.8),
	}

	result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)

	// Verify model is set
	assert.Equal(t, "claude-3-5-sonnet-20241022", string(result.Model))

	// Verify temperature is converted
	assert.NotNil(t, result.Temperature)
	assert.Equal(t, 0.8, result.Temperature.Value)
}

// TestConvertAnthropicBetaToolChoiceToResponses tests tool choice conversion
func TestConvertAnthropicBetaToolChoiceToResponses(t *testing.T) {
	tests := []struct {
		name     string
		tc       anthropic.BetaToolChoiceUnionParam
		expected responses.ToolChoiceOptions
	}{
		{
			name: "auto mode",
			tc: anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			},
			expected: responses.ToolChoiceOptionsAuto,
		},
		{
			name: "any mode (required)",
			tc: anthropic.BetaToolChoiceUnionParam{
				OfAny: &anthropic.BetaToolChoiceAnyParam{},
			},
			expected: responses.ToolChoiceOptionsRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertAnthropicBetaToolChoiceToResponses(&tt.tc)

			assert.NotNil(t, result.OfToolChoiceMode)
			assert.Equal(t, tt.expected, result.OfToolChoiceMode.Value)
		})
	}
}

// TestConvertAnthropicV1ToolChoiceToResponses tests v1 tool choice conversion
func TestConvertAnthropicV1ToolChoiceToResponses(t *testing.T) {
	tests := []struct {
		name     string
		tc       anthropic.ToolChoiceUnionParam
		expected responses.ToolChoiceOptions
	}{
		{
			name: "auto mode",
			tc: anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			},
			expected: responses.ToolChoiceOptionsAuto,
		},
		{
			name: "any mode (required)",
			tc: anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{},
			},
			expected: responses.ToolChoiceOptionsRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertAnthropicV1ToolChoiceToResponses(&tt.tc)

			assert.NotNil(t, result.OfToolChoiceMode)
			assert.Equal(t, tt.expected, result.OfToolChoiceMode.Value)
		})
	}
}

// TestAnthropicV1ThinkingToReasoning_EffortTiers pins the budget_tokens -> effort
// mapping used by ConvertAnthropicV1ToResponsesRequest.
func TestAnthropicV1ThinkingToReasoning_EffortTiers(t *testing.T) {
	tests := []struct {
		name   string
		budget int64
		want   shared.ReasoningEffort
	}{
		{"below low ceiling", 2048, shared.ReasoningEffortLow},
		{"at low ceiling boundary", 4096, shared.ReasoningEffortMedium},
		{"below medium ceiling", 8192, shared.ReasoningEffortMedium},
		{"at medium ceiling boundary", 16384, shared.ReasoningEffortHigh},
		{"well above medium ceiling", 32000, shared.ReasoningEffortHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking := anthropic.ThinkingConfigParamOfEnabled(tt.budget)
			reasoning, ok := anthropicV1ThinkingToReasoning(thinking)
			assert.True(t, ok)
			assert.Equal(t, tt.want, reasoning.Effort)
		})
	}

	t.Run("disabled thinking produces no reasoning param", func(t *testing.T) {
		var disabled anthropic.ThinkingConfigParamUnion
		disabled.OfDisabled = &anthropic.ThinkingConfigDisabledParam{}
		_, ok := anthropicV1ThinkingToReasoning(disabled)
		assert.False(t, ok)
	})

	t.Run("omitted thinking produces no reasoning param", func(t *testing.T) {
		_, ok := anthropicV1ThinkingToReasoning(anthropic.ThinkingConfigParamUnion{})
		assert.False(t, ok)
	})
}

// TestConvertAnthropicV1ToResponsesRequest_ThinkingSetsReasoningEffort verifies
// the top-level request field lands on params.Reasoning.
func TestConvertAnthropicV1ToResponsesRequest_ThinkingSetsReasoningEffort(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:     "claude-3-5-sonnet-latest",
		MaxTokens: 1024,
		Thinking:  anthropic.ThinkingConfigParamOfEnabled(20000),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)

	assert.Equal(t, shared.ReasoningEffortHigh, result.Reasoning.Effort)
}

// TestConvertAnthropicV1ToResponsesRequest_ThinkingBlockBecomesReasoningItem
// verifies a history thinking block survives the round trip as a `reasoning`
// input item, ordered before the message/tool_use items from the same turn —
// DeepSeek's own Responses API emits reasoning items before message items,
// and being stateless, requires the client to replay history in that order.
func TestConvertAnthropicV1ToResponsesRequest_ThinkingBlockBecomesReasoningItem(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:     "claude-3-5-sonnet-latest",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2?")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig-abc", "Let me compute: 2+2=4."),
				anthropic.NewTextBlock("The answer is 4."),
			),
		},
	}

	result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)

	items := result.Input.OfInputItemList
	require.Len(t, items, 3, "user message + reasoning item + assistant message")
	require.NotNil(t, items[1].OfReasoning, "second item (after the user message) should be the reasoning item")
	require.Len(t, items[1].OfReasoning.Summary, 1)
	assert.Equal(t, "Let me compute: 2+2=4.", items[1].OfReasoning.Summary[0].Text)
	assert.NotEmpty(t, items[1].OfReasoning.ID)
	require.NotNil(t, items[2].OfMessage, "assistant text should follow the reasoning item")
}

// TestConvertAnthropicV1ToResponsesRequest_EmptyThinkingBlockSkipped verifies
// a thinking block with no text produces no reasoning item (nothing to carry).
func TestConvertAnthropicV1ToResponsesRequest_EmptyThinkingBlockSkipped(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:     "claude-3-5-sonnet-latest",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hi")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig", ""),
				anthropic.NewTextBlock("Hello!"),
			),
		},
	}

	result := ConvertAnthropicV1ToResponsesRequest(anthropicReq)
	items := result.Input.OfInputItemList
	require.Len(t, items, 2, "user message + assistant message, no empty reasoning item")
	for _, item := range items {
		assert.Nil(t, item.OfReasoning)
	}
}

// TestConvertAnthropicBetaToResponsesRequest_ThinkingBlockBecomesReasoningItem
// mirrors the v1 test for the beta request/response shapes.
func TestConvertAnthropicBetaToResponsesRequest_ThinkingBlockBecomesReasoningItem(t *testing.T) {
	anthropicReq := &anthropic.BetaMessageNewParams{
		Model:     "claude-3-5-sonnet-latest",
		MaxTokens: 1024,
		Thinking:  anthropic.BetaThinkingConfigParamOfEnabled(2000),
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("What is 2+2?")),
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaThinkingBlock("sig-abc", "2+2=4."),
					anthropic.NewBetaTextBlock("The answer is 4."),
				},
			},
		},
	}

	result := ConvertAnthropicBetaToResponsesRequest(anthropicReq)

	assert.Equal(t, shared.ReasoningEffortLow, result.Reasoning.Effort)

	items := result.Input.OfInputItemList
	require.Len(t, items, 3)
	require.NotNil(t, items[1].OfReasoning)
	require.Len(t, items[1].OfReasoning.Summary, 1)
	assert.Equal(t, "2+2=4.", items[1].OfReasoning.Summary[0].Text)
}
