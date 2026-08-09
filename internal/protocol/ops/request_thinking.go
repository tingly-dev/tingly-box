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
//     extension (DeepSeek and friends reject requests that carry both). We
//     clear rather than send OpenAI's "none" because "none" is rejected by
//     models older than gpt-5.1.
//   - "minimal"/"low"/"medium"/"high"/"xhigh"/"max": force thinking enabled at
//     that level. OpenAI targets get reasoning_effort verbatim (all six are
//     SDK-defined). Anthropic targets get output_config.effort (the native
//     effort dialect on Claude 4.5+) plus a budget_tokens fallback; the
//     vendor-stage model transform reconciles per-model support (see
//     request_anthropic_model.go).
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

// enableThinking turns thinking on at the given effort level, with the mapped
// budget as fallback for budget-based targets.
//
// For Anthropic requests: output_config.effort carries the level natively
// (Claude 4.5+). The thinking config keeps the request's existing dialect —
// adaptive stays adaptive (effort is its control knob), everything else is
// forced to enabled(budget). budget_tokens is capped at max_tokens so the
// budget never exceeds the operator's hard limit. When max_tokens itself is
// below 1024 (Anthropic's minimum for extended thinking) we still cap at
// max_tokens and let the API surface the conflict — silently raising the
// budget above max_tokens would violate the operator limit.
// Per-model reconciliation (models without effort / budget / adaptive
// support) happens later in the vendor-stage model transform.
func enableThinking(req interface{}, effort string, budget int64) {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		r.OutputConfig.Effort = anthropicOutputEffort(effort)
		if r.Thinking.OfAdaptive != nil {
			return
		}
		if r.MaxTokens > 0 && budget > r.MaxTokens {
			budget = r.MaxTokens
		}
		r.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
	case *anthropic.BetaMessageNewParams:
		r.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(anthropicOutputEffort(effort))
		if r.Thinking.OfAdaptive != nil {
			return
		}
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
// reasoning_effort. All six ladder levels are OpenAI-defined, so the mapping
// is 1:1 (the old "max collapses to high" rule predates OpenAI's xhigh/max).
// Unknown values fall back to "medium".
func openaiReasoningEffort(effort string) shared.ReasoningEffort {
	switch effort {
	case typ.ThinkingEffortMinimal:
		return shared.ReasoningEffortMinimal
	case typ.ThinkingEffortLow:
		return shared.ReasoningEffortLow
	case typ.ThinkingEffortHigh:
		return shared.ReasoningEffortHigh
	case typ.ThinkingEffortXHigh:
		return shared.ReasoningEffortXhigh
	case typ.ThinkingEffortMax:
		return shared.ReasoningEffortMax
	default:
		return shared.ReasoningEffortMedium
	}
}

// anthropicOutputEffort maps a rule effort level to a valid Anthropic
// output_config.effort. Anthropic's ladder has no "minimal" (collapses to
// "low"); "xhigh" collapses to "high" because no current Claude model
// advertises xhigh support (see internal/data/ref/claude.models.json) even
// though the SDK enum defines it. Unknown values fall back to "medium".
func anthropicOutputEffort(effort string) anthropic.OutputConfigEffort {
	switch effort {
	case typ.ThinkingEffortMinimal, typ.ThinkingEffortLow:
		return anthropic.OutputConfigEffortLow
	case typ.ThinkingEffortHigh, typ.ThinkingEffortXHigh:
		return anthropic.OutputConfigEffortHigh
	case typ.ThinkingEffortMax:
		return anthropic.OutputConfigEffortMax
	default:
		return anthropic.OutputConfigEffortMedium
	}
}
