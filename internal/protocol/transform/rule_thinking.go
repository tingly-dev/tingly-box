package transform

import "github.com/tingly-dev/tingly-box/internal/protocol/ops"

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
	ops.ApplyThinkingEffort(ctx.Request, t.Effort)
	return nil
}
