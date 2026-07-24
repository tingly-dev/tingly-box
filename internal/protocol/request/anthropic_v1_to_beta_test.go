package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertAnthropicV1ToBetaRequest_ToolResultContent guards against
// tool_result content being silently dropped when the v1 request is
// projected onto the Beta shape for smart-routing context extraction — the
// same class of defect as issue #1427, found in this sibling converter while
// fixing that issue (a prior hand-written field-by-field copy hardcoded the
// content to ""). The current implementation is a JSON round-trip, which is
// lossless by construction.
func TestConvertAnthropicV1ToBetaRequest_ToolResultContent(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewToolResultBlock("call_1", "The secret word is ZANZIBAR", false)),
		},
	}

	result := ConvertAnthropicV1ToBetaRequest(req)

	require.Len(t, result.Messages, 1)
	require.Len(t, result.Messages[0].Content, 1)
	toolResult := result.Messages[0].Content[0].OfToolResult
	require.NotNil(t, toolResult)
	require.Len(t, toolResult.Content, 1)
	assert.Equal(t, "The secret word is ZANZIBAR", toolResult.Content[0].OfText.Text)
}

// TestConvertAnthropicV1ToBetaRequest_ToolResultMultipleBlocksPreserved checks
// that a tool_result with several content blocks survives the round-trip as
// distinct blocks (the JSON round-trip preserves structure exactly, unlike a
// hand-written converter that might flatten/join them).
func TestConvertAnthropicV1ToBetaRequest_ToolResultMultipleBlocksPreserved(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfToolResult: &anthropic.ToolResultBlockParam{
							ToolUseID: "call_1",
							Content: []anthropic.ToolResultBlockParamContentUnion{
								{OfText: &anthropic.TextBlockParam{Text: "line one"}},
								{OfText: &anthropic.TextBlockParam{Text: "line two"}},
							},
						},
					},
				},
			},
		},
	}

	result := ConvertAnthropicV1ToBetaRequest(req)

	toolResult := result.Messages[0].Content[0].OfToolResult
	require.NotNil(t, toolResult)
	require.Len(t, toolResult.Content, 2)
	assert.Equal(t, "line one", toolResult.Content[0].OfText.Text)
	assert.Equal(t, "line two", toolResult.Content[1].OfText.Text)
}

// TestConvertAnthropicV1ToBetaRequest_ImagePreserved guards against image
// content being dropped — the prior field-by-field converter created an
// empty BetaImageBlockParam with no Source at all, discarding the image
// entirely. The round-trip carries the real source through.
func TestConvertAnthropicV1ToBetaRequest_ImagePreserved(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfBase64: &anthropic.Base64ImageSourceParam{
									Data:      "AAAA",
									MediaType: "image/png",
								},
							},
						},
					},
				},
			},
		},
	}

	result := ConvertAnthropicV1ToBetaRequest(req)

	image := result.Messages[0].Content[0].OfImage
	require.NotNil(t, image)
	require.NotNil(t, image.Source.OfBase64)
	assert.Equal(t, "AAAA", image.Source.OfBase64.Data)
	assert.Equal(t, anthropic.BetaBase64ImageSourceMediaType("image/png"), image.Source.OfBase64.MediaType)
}

// TestConvertAnthropicV1ToBetaRequest_ModelAndThinking checks the scalar
// fields smart-routing context extraction reads survive the round-trip.
func TestConvertAnthropicV1ToBetaRequest_ModelAndThinking(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: 2048},
		},
		System: []anthropic.TextBlockParam{{Text: "You are a helpful assistant."}},
	}

	result := ConvertAnthropicV1ToBetaRequest(req)

	assert.Equal(t, anthropic.Model("claude-3-5-sonnet-20241022"), result.Model)
	assert.Equal(t, int64(1024), result.MaxTokens)
	require.NotNil(t, result.Thinking.OfEnabled)
	assert.Equal(t, int64(2048), result.Thinking.OfEnabled.BudgetTokens)
	require.Len(t, result.System, 1)
	assert.Equal(t, "You are a helpful assistant.", result.System[0].Text)
}

func TestConvertAnthropicV1ToBetaRequest_Nil(t *testing.T) {
	assert.Nil(t, ConvertAnthropicV1ToBetaRequest(nil))
}
