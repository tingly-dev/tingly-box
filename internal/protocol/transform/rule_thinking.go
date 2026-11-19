package transform

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleThinkingTransform applies the unified thinking_effort control at the
// rule level. It runs as a post-base stage so the type-switch sees the
// upstream-bound request shape after protocol conversion.
//
// Effort semantics are documented on ops.ApplyThinkingEffort.
// Only added to the chain when Effort is non-default.
type RuleThinkingTransform struct {
	Effort string
}

// NewRuleThinkingTransform returns a transform that applies the given effort.
func NewRuleThinkingTransform(effort string) *RuleThinkingTransform {
	return &RuleThinkingTransform{Effort: effort}
}

func (t *RuleThinkingTransform) Name() string { return "rule_thinking" }

func (t *RuleThinkingTransform) Apply(ctx *TransformContext) error {
	if t.Effort != typ.ThinkingEffortOff {
		if _, ok := typ.ThinkingBudgetMapping[t.Effort]; !ok {
			return nil
		}
	}

	if err := ops.ApplyThinkingEffort(ctx.Request, t.Effort); err != nil {
		return err
	}
	t.syncConfig(ctx)
	return nil
}

// syncConfig keeps the metadata produced by the base transform aligned with
// the request mutation above. Vendor transforms consult this metadata after
// the rule stage, so leaving it stale can undo an explicit rule override.
func (t *RuleThinkingTransform) syncConfig(ctx *TransformContext) {
	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		if ctx.Config.OpenAIConfig == nil {
			ctx.Config.OpenAIConfig = &protocol.OpenAIConfig{}
		}
		ctx.Config.OpenAIConfig.HasThinking = t.Effort != typ.ThinkingEffortOff
		ctx.Config.OpenAIConfig.ReasoningEffort = req.ReasoningEffort
	case *responses.ResponseNewParams:
		if ctx.Config.ResponsesConfig == nil {
			ctx.Config.ResponsesConfig = &protocol.OpenAIConfig{}
		}
		ctx.Config.ResponsesConfig.HasThinking = t.Effort != typ.ThinkingEffortOff
		ctx.Config.ResponsesConfig.ReasoningEffort = req.Reasoning.Effort
	}
}
