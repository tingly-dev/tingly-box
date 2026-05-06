package mcp

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

func TestAnthropicBetaApplyContinuation_MergesIntoFirstUserMessageEvenWithLeadingAssistantOrText(t *testing.T) {
	adapter := NewAnthropicBetaAdapter()
	segment := []anthropic.BetaMessageParam{
		anthropic.BetaMessageParam{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolUseBlock("toolu_virtual", map[string]any{"q": "a"}, "tingly_box_mcp__builtin__advisor"),
				anthropic.NewBetaToolUseBlock("toolu_external", map[string]any{"command": "echo hi"}, "Bash"),
			},
		},
		anthropic.NewBetaUserMessage(
			anthropic.NewBetaToolResultBlock("toolu_virtual", "advisor-result", false),
		),
	}
	incoming := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("transient-assistant-wrapper")}},
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("system-reminder or wrapper text")),
			anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("toolu_external", "bash-result", false)),
		},
	}

	updatedAny, err := adapter.ApplyContinuation(incoming, segment)
	require.NoError(t, err)
	updated := updatedAny.(*anthropic.BetaMessageNewParams)
	require.Len(t, updated.Messages, 4)

	require.Equal(t, anthropic.BetaMessageParamRoleAssistant, updated.Messages[0].Role)
	require.Len(t, updated.Messages[0].Content, 2)
	require.Equal(t, anthropic.BetaMessageParamRoleUser, updated.Messages[1].Role)
	require.Len(t, updated.Messages[1].Content, 2)
	require.NotNil(t, updated.Messages[1].Content[0].OfToolResult)
	require.NotNil(t, updated.Messages[1].Content[1].OfToolResult)
	require.Equal(t, "toolu_virtual", updated.Messages[1].Content[0].OfToolResult.ToolUseID)
	require.Equal(t, "toolu_external", updated.Messages[1].Content[1].OfToolResult.ToolUseID)

	require.Equal(t, anthropic.BetaMessageParamRoleAssistant, updated.Messages[2].Role)
	require.Equal(t, anthropic.BetaMessageParamRoleUser, updated.Messages[3].Role)
	require.Len(t, updated.Messages[3].Content, 1)
	require.NotNil(t, updated.Messages[3].Content[0].OfText)
}

func TestAnthropicV1ApplyContinuation_MergesIntoFirstUserMessageEvenWithLeadingAssistantOrText(t *testing.T) {
	adapter := NewAnthropicV1Adapter()
	segment := []anthropic.MessageParam{
		anthropic.MessageParam{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("toolu_virtual", map[string]any{"q": "a"}, "tingly_box_mcp__builtin__advisor"),
				anthropic.NewToolUseBlock("toolu_external", map[string]any{"command": "echo hi"}, "Bash"),
			},
		},
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("toolu_virtual", "advisor-result", false),
		),
	}
	incoming := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("transient-assistant-wrapper")}},
			anthropic.NewUserMessage(anthropic.NewTextBlock("system-reminder or wrapper text")),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_external", "bash-result", false)),
		},
	}

	updatedAny, err := adapter.ApplyContinuation(incoming, segment)
	require.NoError(t, err)
	updated := updatedAny.(*anthropic.MessageNewParams)
	require.Len(t, updated.Messages, 4)

	require.Equal(t, anthropic.MessageParamRoleAssistant, updated.Messages[0].Role)
	require.Len(t, updated.Messages[0].Content, 2)
	require.Equal(t, anthropic.MessageParamRoleUser, updated.Messages[1].Role)
	require.Len(t, updated.Messages[1].Content, 2)
	require.NotNil(t, updated.Messages[1].Content[0].OfToolResult)
	require.NotNil(t, updated.Messages[1].Content[1].OfToolResult)
	require.Equal(t, "toolu_virtual", updated.Messages[1].Content[0].OfToolResult.ToolUseID)
	require.Equal(t, "toolu_external", updated.Messages[1].Content[1].OfToolResult.ToolUseID)

	require.Equal(t, anthropic.MessageParamRoleAssistant, updated.Messages[2].Role)
	require.Equal(t, anthropic.MessageParamRoleUser, updated.Messages[3].Role)
	require.Len(t, updated.Messages[3].Content, 1)
	require.NotNil(t, updated.Messages[3].Content[0].OfText)
}
