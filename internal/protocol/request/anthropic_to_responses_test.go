package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
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
