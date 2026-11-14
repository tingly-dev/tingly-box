package transform

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

func TestLongAnthropicToolIDSurvivesChatAlignment(t *testing.T) {
	id := "toolu_" + strings.Repeat("x", 44)
	in := &anthropic.BetaMessageNewParams{
		Model: "m", MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolUseBlock(id, map[string]any{}, "test")}},
			anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, "ok", false)),
		},
	}
	out, _ := request.ConvertAnthropicBetaToOpenAIRequest(in, false, false, false)
	AlignToolMessagesForOpenAI(out)
	if len(out.Messages) != 2 || out.Messages[0].OfAssistant == nil || out.Messages[1].OfTool == nil {
		t.Fatalf("tool result was not preserved: %+v", out.Messages)
	}
	call := out.Messages[0].OfAssistant.ToolCalls[0].OfFunction.ID
	if len(call) > 40 || out.Messages[1].OfTool.ToolCallID != call {
		t.Fatalf("tool call %q does not match result %q", call, out.Messages[1].OfTool.ToolCallID)
	}
}
