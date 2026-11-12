package transform

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
)

func TestRemoveAnthropicThinkingBudget(t *testing.T) {
	for name, req := range map[string]interface{}{
		"v1":   &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamOfEnabled(4096)},
		"beta": &anthropic.BetaMessageNewParams{Thinking: anthropic.BetaThinkingConfigParamOfEnabled(4096)},
	} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(req)
			require.NoError(t, err)
			require.Contains(t, string(before), `"budget_tokens":4096`)
			require.NoError(t, NewRemoveThinkingBudgetTransform().Apply(&TransformContext{Request: req}))
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.NotContains(t, string(after), `"budget_tokens"`)
			require.Contains(t, string(after), `"type":"enabled"`)
		})
	}
}

func TestRemoveThinkingBudgetLeavesOpenAIRequest(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	req.SetExtraFields(map[string]any{"thinking_budget": 4096})
	before, err := json.Marshal(req)
	require.NoError(t, err)
	require.NoError(t, NewRemoveThinkingBudgetTransform().Apply(&TransformContext{Request: req}))
	after, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestRemoveAnthropicThinkingBudgetLeavesOtherModes(t *testing.T) {
	for name, req := range map[string]interface{}{
		"v1 disabled":   &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}},
		"beta disabled": &anthropic.BetaMessageNewParams{Thinking: anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}},
		"v1 adaptive":   &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}},
		"beta adaptive": &anthropic.BetaMessageNewParams{Thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}},
	} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(req)
			require.NoError(t, err)
			require.NoError(t, NewRemoveThinkingBudgetTransform().Apply(&TransformContext{Request: req}))
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}
