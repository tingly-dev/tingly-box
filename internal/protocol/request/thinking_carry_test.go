package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
)

// TestAnthropicThinkingToResponsesEffort pins that an Anthropic request's
// thinking reaches a Responses request as reasoning.effort, through the same
// producer the Chat conversion uses (anthropicViewReasoningEffort).
func TestAnthropicThinkingToResponsesEffort(t *testing.T) {
	user := []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))}
	tests := []struct {
		name     string
		thinking anthropic.BetaThinkingConfigParamUnion
		effort   anthropic.BetaOutputConfigEffort
		want     shared.ReasoningEffort
	}{
		{name: "no thinking", want: ""},
		{name: "disabled", thinking: anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}, want: ""},
		{name: "budget tiered", thinking: anthropic.BetaThinkingConfigParamOfEnabled(10240), want: "medium"},
		{name: "max budget tiered", thinking: anthropic.BetaThinkingConfigParamOfEnabled(31999), want: "max"},
		{name: "adaptive defaults to medium", thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}, want: "medium"},
		{name: "output_config.effort wins", thinking: anthropic.BetaThinkingConfigParamOfEnabled(1024), effort: "high", want: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beta := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 40000, Messages: user, Thinking: tt.thinking}
			beta.OutputConfig.Effort = tt.effort
			assert.Equal(t, tt.want, ConvertAnthropicBetaToResponsesRequest(beta).Reasoning.Effort, "beta")

			v1, err := ConvertAnthropicBetaToV1Request(beta)
			if assert.NoError(t, err) {
				assert.Equal(t, tt.want, ConvertAnthropicV1ToResponsesRequest(v1).Reasoning.Effort, "v1")
			}
		})
	}
}

// TestOpenAIEffortToAnthropicThinking pins that an OpenAI client's reasoning
// effort reaches an Anthropic request as thinking at the ladder's budget plus
// output_config.effort.
func TestOpenAIEffortToAnthropicThinking(t *testing.T) {
	tests := []struct {
		effort     shared.ReasoningEffort
		wantBudget int64 // 0 = no thinking
		wantEffort anthropic.BetaOutputConfigEffort
	}{
		{effort: ""},
		{effort: "none"},
		{effort: "minimal", wantBudget: 1024, wantEffort: "low"},
		{effort: "medium", wantBudget: 10240, wantEffort: "medium"},
		{effort: "high", wantBudget: 20480, wantEffort: "high"},
		{effort: "xhigh", wantBudget: 24576, wantEffort: "xhigh"},
	}
	for _, tt := range tests {
		t.Run(string(tt.effort), func(t *testing.T) {
			chat := ConvertOpenAIToAnthropicRequest(&openai.ChatCompletionNewParams{
				Model:           "m",
				Messages:        []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
				ReasoningEffort: tt.effort,
			}, 4096)
			resp := ConvertOpenAIResponsesToAnthropicBetaRequest(responses.ResponseNewParams{
				Model:     "m",
				Input:     responses.ResponseNewParamsInputUnion{OfString: openai.String("hi")},
				Reasoning: shared.ReasoningParam{Effort: tt.effort},
			}, 4096)
			for name, req := range map[string]*anthropic.BetaMessageNewParams{"chat": chat, "responses": resp} {
				if tt.wantBudget == 0 {
					assert.Nil(t, req.Thinking.OfEnabled, name)
					assert.Empty(t, req.OutputConfig.Effort, name)
					continue
				}
				if assert.NotNil(t, req.Thinking.OfEnabled, name) {
					assert.Equal(t, tt.wantBudget, req.Thinking.OfEnabled.BudgetTokens, name)
				}
				assert.Equal(t, tt.wantEffort, req.OutputConfig.Effort, name)
			}
		})
	}
}
