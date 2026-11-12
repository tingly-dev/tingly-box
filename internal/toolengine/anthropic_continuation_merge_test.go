package toolengine

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
)

func TestMergeAnthropicBetaContinuation_MergesIncomingToolResultIntoSameUserMessage(t *testing.T) {
	segment := []anthropic.BetaMessageParam{
		msgparam.NewBeta("assistant", []anthropic.BetaContentBlockParamUnion{
			anthropic.NewBetaToolUseBlock("toolu_virtual", map[string]any{"q": "a"}, "tingly_box_mcp__builtin__advisor"),
			anthropic.NewBetaToolUseBlock("toolu_external", map[string]any{"command": "echo hi"}, "Bash"),
		}),
		anthropic.NewBetaUserMessage(
			anthropic.NewBetaToolResultBlock("toolu_virtual", "advisor-result", false),
		),
	}
	incoming := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(
				anthropic.NewBetaToolResultBlock("toolu_external", "bash-result", false),
			),
		},
	}

	updated := &anthropic.BetaMessageNewParams{Messages: mergeAnthropicBetaContinuation(segment, incoming.Messages)}
	require.Len(t, updated.Messages, 2)
	require.Equal(t, "assistant", msgparam.BetaRole(updated.Messages[0]))
	require.Equal(t, "user", msgparam.BetaRole(updated.Messages[1]))
	require.Len(t, msgparam.BetaContent(updated.Messages[1]), 2)
	require.NotNil(t, msgparam.BetaContent(updated.Messages[1])[0].OfToolResult)
	require.NotNil(t, msgparam.BetaContent(updated.Messages[1])[1].OfToolResult)
	require.Equal(t, "toolu_virtual", msgparam.BetaContent(updated.Messages[1])[0].OfToolResult.ToolUseID)
	require.Equal(t, "toolu_external", msgparam.BetaContent(updated.Messages[1])[1].OfToolResult.ToolUseID)
}
