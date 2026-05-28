package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleThinkingTransform applies the unified thinking_effort control at the
// rule level. It runs as a post-base stage so the type-switch sees the
// upstream-bound request shape after protocol conversion.
//
// Effort semantics:
//   - "" (default): pass through, no change.
//   - "off": force thinking disabled. For Anthropic this sets OfDisabled;
//     for OpenAI this clears reasoning_effort and removes any stray `thinking`
//     extension some clients/vendors leave in ExtraFields (DeepSeek and friends
//     reject requests that carry both reasoning_effort and thinking.type).
//   - "low"/"medium"/"high"/"max": force thinking enabled with the matching
//     budget. "max" collapses to "high" for OpenAI which has no "max".
//
// Only added to the chain when Effort is non-default.
type RuleThinkingTransform struct {
	Effort string
}

// NewRuleThinkingTransform returns a transform that applies the given effort.
func NewRuleThinkingTransform(effort string) *RuleThinkingTransform {
	return &RuleThinkingTransform{Effort: effort}
}

func (t *RuleThinkingTransform) Name() string { return "rule_thinking" }

// Apply enforces the configured thinking_effort on the target request. For
// shapes it does not recognize (e.g. Google) it is a no-op.
func (t *RuleThinkingTransform) Apply(ctx *TransformContext) error {
	ApplyThinkingEffort(ctx.Request, t.Effort)
	return nil
}

// ApplyThinkingEffort is the shared implementation behind both the rule-level
// and scenario-level thinking_effort handling. It is exported so the
// server-side prechain (scenario flags) can reuse the same semantics without
// duplicating the type-switch.
//
// Behavior is documented on RuleThinkingTransform.
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
// conflicting extension fields. Conservative: only touches the union variant
// matching the request type.
func disableThinking(req interface{}) {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		r.Thinking = anthropic.ThinkingConfigParamUnion{
			OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
		}
	case *anthropic.BetaMessageNewParams:
		r.Thinking = anthropic.BetaThinkingConfigParamUnion{
			OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
		}
	case *openai.ChatCompletionNewParams:
		r.ReasoningEffort = ""
		stripOpenAIThinkingExtra(r)
	case *responses.ResponseNewParams:
		r.Reasoning.Effort = ""
	}
}

// enableThinking turns thinking on with the matching budget / effort. As with
// disable, it also clears any stray `thinking` extension on OpenAI requests
// so the typed field is the single source of truth.
func enableThinking(req interface{}, effort string, budget int64) {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		r.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
	case *anthropic.BetaMessageNewParams:
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
// `thinking.type` extension — they want one or the other. Once the transform
// has expressed intent through the typed field, the extras blob is redundant
// at best and a 400 trigger at worst.
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
