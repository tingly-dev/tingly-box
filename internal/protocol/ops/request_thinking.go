package ops

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ApplyThinkingEffort enforces a thinking_effort level on any supported request
// shape. Shared by both the rule-level and scenario-level thinking transforms.
//
// Effort semantics:
//   - "" (default): pass through, no change.
//   - "off": force thinking disabled. For Anthropic this sets OfDisabled;
//     for OpenAI this clears reasoning_effort and removes any stray `thinking`
//     extension (DeepSeek and friends reject requests that carry both).
//   - "low"/"medium"/"high"/"max": force thinking enabled with the matching
//     budget. "max" collapses to "high" for OpenAI which has no "max".
func ApplyThinkingEffort(req interface{}, effort string) {
	switch effort {
	case typ.ThinkingEffortDefault:
		return
	case typ.ThinkingEffortOff:
		disableThinking(req)
		return
	}
	budget, ok := typ.ThinkingBudgetMapping[effort]
	if !ok {
		return
	}
	enableThinking(req, effort, budget)
}

// disableThinking turns thinking off on the target request and scrubs any
// conflicting extension fields.
func disableThinking(req interface{}) {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		r.Thinking = anthropic.ThinkingConfigParamUnion{
			OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
		}
		r.OutputConfig.Effort = ""
	case *anthropic.BetaMessageNewParams:
		r.Thinking = anthropic.BetaThinkingConfigParamUnion{
			OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
		}
		r.OutputConfig.Effort = ""
	case *openai.ChatCompletionNewParams:
		r.ReasoningEffort = ""
		stripOpenAIThinkingExtra(r)
	case *responses.ResponseNewParams:
		r.Reasoning.Effort = ""
	}
}

// enableThinking turns thinking on with the matching budget / effort.
//
// For Anthropic requests: budget_tokens is capped at max_tokens so the budget
// never exceeds the operator's hard limit. When max_tokens itself is below 1024
// (Anthropic's minimum for extended thinking) we still cap at max_tokens and
// let the API surface the conflict — silently raising the budget above
// max_tokens would violate the operator limit.
func enableThinking(req interface{}, effort string, budget int64) {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r.MaxTokens > 0 && budget > r.MaxTokens {
			budget = r.MaxTokens
		}
		r.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
	case *anthropic.BetaMessageNewParams:
		if r.MaxTokens > 0 && budget > r.MaxTokens {
			budget = r.MaxTokens
		}
		r.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
	case *openai.ChatCompletionNewParams:
		r.ReasoningEffort = openaiReasoningEffort(effort)
		stripOpenAIThinkingExtra(r)
	case *responses.ResponseNewParams:
		r.Reasoning.Effort = openaiReasoningEffort(effort)
	}
}

// stripOpenAIThinkingExtra removes any non-standard `thinking` blob from an
// OpenAI Chat request's ExtraFields. Several upstreams (DeepSeek, Moonshot)
// reject requests that carry both the typed `reasoning_effort` and a
// `thinking.type` extension.
func stripOpenAIThinkingExtra(req *openai.ChatCompletionNewParams) {
	extra := req.ExtraFields()
	if extra == nil {
		return
	}
	if _, ok := extra["thinking"]; !ok {
		return
	}
	delete(extra, "thinking")
	req.SetExtraFields(extra)
}

// openaiReasoningEffort maps a rule effort level to a valid OpenAI
// reasoning_effort. "max" collapses to "high" because OpenAI has no "max".
func openaiReasoningEffort(effort string) shared.ReasoningEffort {
	switch effort {
	case typ.ThinkingEffortLow:
		return shared.ReasoningEffortLow
	case typ.ThinkingEffortHigh, typ.ThinkingEffortMax:
		return shared.ReasoningEffortHigh
	default:
		return shared.ReasoningEffortMedium
	}
}
