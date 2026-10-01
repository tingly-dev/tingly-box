package ops

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestToolTurnWithoutThinkingTurnsBudgetThinkingOff pins the vendor-stage
// guard (run for every Anthropic-shaped target, before model reconciliation): with budget thinking enabled, Anthropic requires the final assistant
// message to start with a thinking block, so a tool-use turn without one (an
// OpenAI client's history, or a rule forcing thinking_effort onto it) keeps
// thinking off instead of failing upstream.
func TestToolTurnWithoutThinkingTurnsBudgetThinkingOff(t *testing.T) {
	toolUse := anthropic.BetaContentBlockParamUnion{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: "toolu_1", Name: "shell", Input: map[string]any{}}}
	signed := anthropic.BetaContentBlockParamUnion{OfThinking: &anthropic.BetaThinkingBlockParam{Thinking: "plan", Signature: "sig"}}
	toolResult := anthropic.NewBetaToolResultBlock("toolu_1", "ok", false)
	history := func(assistant ...anthropic.BetaContentBlockParamUnion) []anthropic.BetaMessageParam {
		return []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("run it")),
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: assistant},
			anthropic.NewBetaUserMessage(toolResult),
		}
	}
	tests := []struct {
		name        string
		messages    []anthropic.BetaMessageParam
		thinking    anthropic.BetaThinkingConfigParamUnion
		wantEnabled bool
	}{
		{name: "tool turn without thinking: off", messages: history(toolUse), thinking: anthropic.BetaThinkingConfigParamOfEnabled(4096), wantEnabled: false},
		{name: "tool turn with signed thinking: kept", messages: history(signed, toolUse), thinking: anthropic.BetaThinkingConfigParamOfEnabled(4096), wantEnabled: true},
		{name: "no tool turn: kept", messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))}, thinking: anthropic.BetaThinkingConfigParamOfEnabled(4096), wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &anthropic.BetaMessageNewParams{Model: "third-party-model", MaxTokens: 8192, Messages: tt.messages, Thinking: tt.thinking}
			DisableBetaThinkingForUnsignedToolTurn(req)
			if got := req.Thinking.OfEnabled != nil; got != tt.wantEnabled {
				t.Errorf("thinking enabled = %v, want %v (thinking=%+v)", got, tt.wantEnabled, req.Thinking)
			}
			if !tt.wantEnabled && req.Thinking.OfDisabled == nil {
				t.Errorf("thinking not explicitly disabled: %+v", req.Thinking)
			}

			v1 := &anthropic.MessageNewParams{Model: "third-party-model", MaxTokens: 8192, Thinking: anthropic.ThinkingConfigParamOfEnabled(4096)}
			for _, m := range tt.messages {
				var blocks []anthropic.ContentBlockParamUnion
				for _, b := range m.Content {
					switch {
					case b.OfToolUse != nil:
						blocks = append(blocks, anthropic.ContentBlockParamUnion{OfToolUse: &anthropic.ToolUseBlockParam{ID: b.OfToolUse.ID, Name: b.OfToolUse.Name, Input: b.OfToolUse.Input}})
					case b.OfThinking != nil:
						blocks = append(blocks, anthropic.NewThinkingBlock(b.OfThinking.Signature, b.OfThinking.Thinking))
					case b.OfToolResult != nil:
						blocks = append(blocks, anthropic.NewToolResultBlock(b.OfToolResult.ToolUseID, "ok", false))
					default:
						blocks = append(blocks, anthropic.NewTextBlock("hi"))
					}
				}
				v1.Messages = append(v1.Messages, anthropic.MessageParam{Role: anthropic.MessageParamRole(m.Role), Content: blocks})
			}
			DisableV1ThinkingForUnsignedToolTurn(v1)
			if got := v1.Thinking.OfEnabled != nil; got != tt.wantEnabled {
				t.Errorf("v1 thinking enabled = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}
