package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// A third-party Anthropic-compatible provider, not a Claude Code backend.
const thirdPartyAnthropicURL = "https://api.z.ai/api/anthropic"

// toolTurnWithoutThinkingV1 is what an Anthropic client sends back after the
// provider's model answered a tool call without a leading thinking block.
func toolTurnWithoutThinkingV1() *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:       anthropic.Model("glm-5"),
		MaxTokens:   32000,
		Temperature: param.NewOpt(0.6),
		Thinking:    anthropic.ThinkingConfigParamOfEnabled(16000),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("list files")),
			anthropic.NewAssistantMessage(
				anthropic.NewTextBlock("ok"),
				anthropic.NewToolUseBlock("t1", map[string]any{"cmd": "ls"}, "Bash"),
			),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock("t1", "a.txt", false)),
		},
	}
}

func toolTurnWithoutThinkingBeta() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:       anthropic.Model("glm-5"),
		MaxTokens:   32000,
		Temperature: param.NewOpt(0.6),
		Thinking:    anthropic.BetaThinkingConfigParamOfEnabled(16000),
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("list files")}},
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("ok"),
				anthropic.NewBetaToolUseBlock("t1", map[string]any{"cmd": "ls"}, "Bash"),
			}},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("t1", "a.txt", false)}},
		},
	}
}

// An Anthropic client's own thinking is sent as it came on a third-party
// provider: reconciling it flipped thinking between requests of one
// conversation and invalidated the provider's prompt cache (see
// reconcileThinking).
func TestVendorTransform_AnthropicPassthroughKeepsClientThinking(t *testing.T) {
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		t.Run(string(source)+"/v1", func(t *testing.T) {
			req := toolTurnWithoutThinkingV1()
			ctx := &TransformContext{SourceAPI: source, TargetAPI: protocol.TypeAnthropicV1,
				Provider: &typ.Provider{APIBase: thirdPartyAnthropicURL}, Request: req}
			require.NoError(t, NewVendorTransform().Apply(ctx))
			out := ctx.Request.(*anthropic.MessageNewParams)
			require.NotNil(t, out.Thinking.OfEnabled, "client thinking must stay enabled")
			assert.EqualValues(t, 16000, out.Thinking.OfEnabled.BudgetTokens)
			assert.Equal(t, 0.6, out.Temperature.Value, "client sampling must stay as sent")
		})
		t.Run(string(source)+"/beta", func(t *testing.T) {
			req := toolTurnWithoutThinkingBeta()
			ctx := &TransformContext{SourceAPI: source, TargetAPI: protocol.TypeAnthropicBeta,
				Provider: &typ.Provider{APIBase: thirdPartyAnthropicURL}, Request: req}
			require.NoError(t, NewVendorTransform().Apply(ctx))
			out := ctx.Request.(*anthropic.BetaMessageNewParams)
			require.NotNil(t, out.Thinking.OfEnabled, "client thinking must stay enabled")
			assert.EqualValues(t, 16000, out.Thinking.OfEnabled.BudgetTokens)
			assert.Equal(t, 0.6, out.Temperature.Value, "client sampling must stay as sent")
		})
	}
}

// Gateway-produced thinking is still reconciled: a Bridge from an OpenAI
// client (no thinking blocks in its history), and a Claude Code backend.
func TestVendorTransform_GatewayThinkingStillReconciled(t *testing.T) {
	cases := []struct {
		name string
		ctx  func(req any) *TransformContext
	}{
		{"openai_chat_client", func(req any) *TransformContext {
			return &TransformContext{SourceAPI: protocol.TypeOpenAIChat, Provider: &typ.Provider{APIBase: thirdPartyAnthropicURL}, Request: req}
		}},
		{"openai_responses_client", func(req any) *TransformContext {
			return &TransformContext{SourceAPI: protocol.TypeOpenAIResponses, Provider: &typ.Provider{APIBase: thirdPartyAnthropicURL}, Request: req}
		}},
		{"claude_code_backend", func(req any) *TransformContext {
			return &TransformContext{SourceAPI: protocol.TypeAnthropicBeta, Provider: &typ.Provider{APIBase: "https://api.anthropic.com"}, Request: req,
				Extra: map[string]interface{}{"device": "d", "user_id": "u"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/v1", func(t *testing.T) {
			ctx := tc.ctx(toolTurnWithoutThinkingV1())
			require.NoError(t, NewVendorTransform().Apply(ctx))
			out := ctx.Request.(*anthropic.MessageNewParams)
			assert.Nil(t, out.Thinking.OfEnabled, "tool turn without a thinking block turns budget thinking off")
		})
		t.Run(tc.name+"/beta", func(t *testing.T) {
			ctx := tc.ctx(toolTurnWithoutThinkingBeta())
			require.NoError(t, NewVendorTransform().Apply(ctx))
			out := ctx.Request.(*anthropic.BetaMessageNewParams)
			assert.Nil(t, out.Thinking.OfEnabled, "tool turn without a thinking block turns budget thinking off")
		})
	}
}

// A rule's thinking_effort rewrites the budget but not who owns the history:
// an Anthropic client's request stays unreconciled on a third-party provider.
func TestVendorTransform_RuleThinkingKeepsAnthropicPassthrough(t *testing.T) {
	req := toolTurnWithoutThinkingBeta()
	ctx := &TransformContext{SourceAPI: protocol.TypeAnthropicBeta, Provider: &typ.Provider{APIBase: thirdPartyAnthropicURL}, Request: req}
	require.NoError(t, NewRuleThinkingTransform("high").Apply(ctx))
	require.NoError(t, NewVendorTransform().Apply(ctx))
	out := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.NotNil(t, out.Thinking.OfEnabled, "rule-set thinking on a client-owned history must not flip off")
}
