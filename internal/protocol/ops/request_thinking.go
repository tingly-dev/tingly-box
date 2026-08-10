package ops

import (
	"fmt"

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
func ApplyThinkingEffort(req interface{}, effort string) error {
	switch effort {
	case typ.ThinkingEffortDefault:
		return nil
	case typ.ThinkingEffortOff:
		disableThinking(req)
		return nil
	}
	budget, ok := typ.ThinkingBudgetMapping[effort]
	if !ok {
		return nil
	}
	return enableThinking(req, effort, budget)
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
// forced to enabled(budget). budget_tokens is fitted without raising the
// operator limit, so it remains at least 1024 and strictly below max_tokens,
// as required by Anthropic. An impossible limit is returned as a local error
// rather than emitting a request that the upstream will reject.
// Per-model reconciliation (models without effort / budget / adaptive
// support) happens later in the vendor-stage model transform.
func enableThinking(req interface{}, effort string, budget int64) error {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r.Thinking.OfAdaptive != nil {
			r.OutputConfig.Effort = anthropicOutputEffort(effort)
			return nil
		}
		fitted, err := fitAnthropicThinkingBudget(budget, r.MaxTokens)
		if err != nil {
			return err
		}
		r.OutputConfig.Effort = anthropicOutputEffort(effort)
		r.Thinking = anthropic.ThinkingConfigParamOfEnabled(fitted)
	case *anthropic.BetaMessageNewParams:
		if r.Thinking.OfAdaptive != nil {
			r.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(anthropicOutputEffort(effort))
			return nil
		}
		fitted, err := fitAnthropicThinkingBudget(budget, r.MaxTokens)
		if err != nil {
			return err
		}
		r.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(anthropicOutputEffort(effort))
		r.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(fitted)
	case *openai.ChatCompletionNewParams:
		r.ReasoningEffort = openaiReasoningEffort(effort)
		stripOpenAIThinkingExtra(r)
	case *responses.ResponseNewParams:
		r.Reasoning.Effort = openaiReasoningEffort(effort)
	}
	return nil
}

const minAnthropicThinkingBudget int64 = 1024

// fitAnthropicThinkingBudget enforces Anthropic's wire constraints without
// raising max_tokens: budget_tokens >= 1024 and budget_tokens < max_tokens.
func fitAnthropicThinkingBudget(budget, maxTokens int64) (int64, error) {
	if budget < minAnthropicThinkingBudget {
		budget = minAnthropicThinkingBudget
	}
	if maxTokens <= 0 {
		return budget, nil
	}
	if maxTokens <= minAnthropicThinkingBudget {
		return 0, fmt.Errorf("anthropic thinking requires max_tokens greater than %d (got %d)", minAnthropicThinkingBudget, maxTokens)
	}
	if budget >= maxTokens {
		budget = maxTokens - 1
	}
	return budget, nil
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
// "low"); all other native values remain distinct so the later model-aware
// transform can clamp them against cataloged support. Unknown values fall
// back to "medium".
func anthropicOutputEffort(effort string) anthropic.OutputConfigEffort {
	switch effort {
	case typ.ThinkingEffortMinimal, typ.ThinkingEffortLow:
		return anthropic.OutputConfigEffortLow
	case typ.ThinkingEffortHigh:
		return anthropic.OutputConfigEffortHigh
	case typ.ThinkingEffortXHigh:
		return anthropic.OutputConfigEffortXhigh
	case typ.ThinkingEffortMax:
		return anthropic.OutputConfigEffortMax
	default:
		return anthropic.OutputConfigEffortMedium
	}
}
