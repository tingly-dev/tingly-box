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

// TestAnthropicHistoryThinkingNotCarriedToResponses pins that thinking left in
// the history carries no reasoning.effort to a Responses request: only a
// request that turns thinking on does (a non-reasoning Responses model rejects
// the field).
func TestAnthropicHistoryThinkingNotCarriedToResponses(t *testing.T) {
	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
		{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{
			anthropic.NewBetaThinkingBlock("sig", "earlier thought"),
			anthropic.NewBetaTextBlock("hello"),
		}},
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("again")),
	}
	for name, thinking := range map[string]anthropic.BetaThinkingConfigParamUnion{
		"unset":    {},
		"disabled": {OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}},
	} {
		beta := &anthropic.BetaMessageNewParams{Model: "m", MaxTokens: 4096, Messages: messages, Thinking: thinking}
		assert.Empty(t, ConvertAnthropicBetaToResponsesRequest(beta).Reasoning.Effort, name)
	}
}

// TestOpenAIEffortLeavesAnswerRoom pins that thinking carried from an OpenAI
// client's effort never eats the whole output: with no client limit the
// default answer room grows by the budget, with a client limit the budget
// takes at most half of it, and a limit too small for any budget leaves
// thinking off.
func TestOpenAIEffortLeavesAnswerRoom(t *testing.T) {
	tests := []struct {
		name          string
		limit         int64 // 0 = client set none
		wantMaxTokens int64
		wantBudget    int64 // 0 = no thinking
	}{
		{name: "no limit", limit: 0, wantMaxTokens: 4096 + 20480, wantBudget: 20480},
		{name: "roomy limit", limit: 64000, wantMaxTokens: 64000, wantBudget: 20480},
		{name: "tight limit", limit: 8000, wantMaxTokens: 8000, wantBudget: 4000},
		{name: "small limit floors at 1024", limit: 1500, wantMaxTokens: 1500, wantBudget: 1024},
		{name: "limit leaves no room", limit: 1024, wantMaxTokens: 1024, wantBudget: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatReq := &openai.ChatCompletionNewParams{
				Model:           "m",
				Messages:        []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
				ReasoningEffort: "high",
			}
			respReq := responses.ResponseNewParams{
				Model:     "m",
				Input:     responses.ResponseNewParamsInputUnion{OfString: openai.String("hi")},
				Reasoning: shared.ReasoningParam{Effort: "high"},
			}
			if tt.limit > 0 {
				chatReq.MaxCompletionTokens = openai.Int(tt.limit)
				respReq.MaxOutputTokens = openai.Int(tt.limit)
			}
			for name, req := range map[string]*anthropic.BetaMessageNewParams{
				"chat":      ConvertOpenAIToAnthropicRequest(chatReq, 4096),
				"responses": ConvertOpenAIResponsesToAnthropicBetaRequest(respReq, 4096),
			} {
				assert.Equal(t, tt.wantMaxTokens, req.MaxTokens, name)
				if tt.wantBudget == 0 {
					assert.Nil(t, req.Thinking.OfEnabled, name)
					continue
				}
				if assert.NotNil(t, req.Thinking.OfEnabled, name) {
					assert.Equal(t, tt.wantBudget, req.Thinking.OfEnabled.BudgetTokens, name)
				}
			}
		})
	}
}

// TestChatMaxCompletionTokensReachesAnthropic pins that the modern Chat limit
// field is honored (it was ignored in favor of the 4096 default), and wins
// over the deprecated max_tokens.
func TestChatMaxCompletionTokensReachesAnthropic(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model:               "m",
		Messages:            []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
		MaxCompletionTokens: openai.Int(32000),
		MaxTokens:           openai.Int(1000),
	}
	assert.Equal(t, int64(32000), ConvertOpenAIToAnthropicRequest(req, 4096).MaxTokens)
}
