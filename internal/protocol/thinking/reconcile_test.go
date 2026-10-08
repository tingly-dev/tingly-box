package thinking

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
)

// TestToolTurnWithoutThinkingTurnsBudgetThinkingOff pins the wire guard: with
// budget thinking enabled, Anthropic requires the final assistant message to
// start with a thinking block, so a tool-use turn without one (an OpenAI
// client's history) keeps thinking off instead of failing upstream.
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
			DisableBetaForUnsignedToolTurn(req)
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
			DisableV1ForUnsignedToolTurn(v1)
			if got := v1.Thinking.OfEnabled != nil; got != tt.wantEnabled {
				t.Errorf("v1 thinking enabled = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

// TestReconcileThinkingWithRequest pins the wire rules thinking imposes on the
// rest of an Anthropic request: forced tool use turns thinking off, and the
// sampling parameters thinking forbids are dropped or clamped.
func TestReconcileThinkingWithRequest(t *testing.T) {
	user := []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))}
	enabled := anthropic.BetaThinkingConfigParamOfEnabled(2048)
	adaptive := anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}

	t.Run("forced tool turns thinking off", func(t *testing.T) {
		for name, choice := range map[string]anthropic.BetaToolChoiceUnionParam{
			"tool": anthropic.BetaToolChoiceParamOfTool("x"),
			"any":  {OfAny: &anthropic.BetaToolChoiceAnyParam{}},
		} {
			for kind, th := range map[string]anthropic.BetaThinkingConfigParamUnion{"enabled": enabled, "adaptive": adaptive} {
				req := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 4096, Messages: user, Thinking: th, ToolChoice: choice}
				ReconcileBetaWithRequest(req)
				assert.NotNil(t, req.Thinking.OfDisabled, "%s/%s", name, kind)
			}
		}
	})

	t.Run("auto tool choice keeps thinking", func(t *testing.T) {
		req := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 4096, Messages: user, Thinking: enabled,
			ToolChoice: anthropic.BetaToolChoiceUnionParam{OfAuto: &anthropic.BetaToolChoiceAutoParam{}}}
		ReconcileBetaWithRequest(req)
		assert.NotNil(t, req.Thinking.OfEnabled)
	})

	t.Run("sampling made legal", func(t *testing.T) {
		req := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 4096, Messages: user, Thinking: enabled,
			Temperature: anthropic.Float(0.2), TopK: anthropic.Int(40), TopP: anthropic.Float(0.5)}
		ReconcileBetaWithRequest(req)
		assert.False(t, req.Temperature.Valid())
		assert.False(t, req.TopK.Valid())
		assert.Equal(t, 0.95, req.TopP.Value)

		v1 := &anthropic.MessageNewParams{Model: "m", MaxTokens: 4096,
			Messages:    []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
			Thinking:    anthropic.ThinkingConfigParamOfEnabled(2048),
			Temperature: anthropic.Float(1), TopP: anthropic.Float(0.98)}
		ReconcileV1WithRequest(v1)
		assert.Equal(t, 1.0, v1.Temperature.Value, "temperature 1 is allowed")
		assert.Equal(t, 0.98, v1.TopP.Value, "top_p in range is kept")
	})

	t.Run("thinking off leaves the request alone", func(t *testing.T) {
		req := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 4096, Messages: user,
			Temperature: anthropic.Float(0.2), ToolChoice: anthropic.BetaToolChoiceParamOfTool("x")}
		ReconcileBetaWithRequest(req)
		assert.Equal(t, 0.2, req.Temperature.Value)
		assert.Nil(t, req.Thinking.OfDisabled)
	})
}
