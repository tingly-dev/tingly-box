package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// toolTurnWithoutThinkingBeta is a request continuing a tool-use turn whose
// assistant message has no thinking block — what a provider that did not return
// one leaves in an Anthropic client's own history — with budget thinking on.
func toolTurnWithoutThinkingBeta() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 32000,
		Thinking:  anthropic.BetaThinkingConfigParamUnion{OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{BudgetTokens: 31999}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("run it")),
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolUseBlock("t1", map[string]any{}, "Bash"),
			}},
			anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("t1", "ok", false)),
		},
	}
}

func applyVendor(t *testing.T, source protocol.APIType, extra map[string]interface{}) *anthropic.BetaMessageNewParams {
	t.Helper()
	ctx := &TransformContext{
		SourceAPI: source,
		Provider:  &typ.Provider{APIBase: "https://open.bigmodel.cn/api/anthropic"},
		Request:   toolTurnWithoutThinkingBeta(),
		Extra:     extra,
	}
	require.NoError(t, NewVendorTransform().Apply(ctx))
	return ctx.Request.(*anthropic.BetaMessageNewParams)
}

// An Anthropic client's thinking is forwarded as sent. Flipping it to disabled
// on every tool-loop turn that lacks a thinking block changed the cached
// prefix from one request to the next.
func TestVendorTransform_AnthropicClientThinkingForwardedAsSent(t *testing.T) {
	for _, source := range []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeAnthropicV1} {
		out := applyVendor(t, source, map[string]interface{}{})
		require.NotNil(t, out.Thinking.OfEnabled, "source %s", source)
		assert.Equal(t, int64(31999), out.Thinking.OfEnabled.BudgetTokens)
		assert.Nil(t, out.Thinking.OfDisabled)
	}
}

// A client whose history has no thinking blocks by construction (OpenAI) still
// gets the guard: Anthropic rejects budget thinking on such a tool turn.
func TestVendorTransform_OpenAIClientThinkingStillGuarded(t *testing.T) {
	for _, source := range []protocol.APIType{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses} {
		out := applyVendor(t, source, map[string]interface{}{})
		assert.NotNil(t, out.Thinking.OfDisabled, "source %s", source)
		assert.Nil(t, out.Thinking.OfEnabled)
	}
}

// Thinking a rule's thinking_effort switched on is the gateway's, not the
// client's, even on an Anthropic client, and keeps the guard.
func TestVendorTransform_RuleForcedThinkingStillGuarded(t *testing.T) {
	out := applyVendor(t, protocol.TypeAnthropicBeta, map[string]interface{}{extraRuleForcedThinking: true})
	assert.NotNil(t, out.Thinking.OfDisabled)
	assert.Nil(t, out.Thinking.OfEnabled)
}

// RuleThinkingTransform leaves the mark the vendor step reads.
func TestRuleThinkingTransform_MarksRuleForcedThinking(t *testing.T) {
	ctx := &TransformContext{SourceAPI: protocol.TypeAnthropicBeta, Request: toolTurnWithoutThinkingBeta()}
	require.NoError(t, NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx))
	forced, _ := ctx.Extra[extraRuleForcedThinking].(bool)
	assert.True(t, forced)
}
