package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertAnthropicV1ToBetaRequest_ToolResultContent guards against the
// tool_result content being silently dropped (hardcoded to "") when the v1
// request is projected onto the Beta shape for smart-routing context
// extraction — the same class of defect as issue #1427, found in a sibling
// converter while fixing that issue.
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

func TestConvertAnthropicV1ToBetaRequest_ToolResultMultipleBlocksJoined(t *testing.T) {
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
	require.Len(t, toolResult.Content, 1)
	assert.Equal(t, "line one\nline two", toolResult.Content[0].OfText.Text)
}
