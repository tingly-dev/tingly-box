package server

import (
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// ThinkingEffortTransform is the scenario-level pre-base transform that
// applies the unified thinking_effort control on the inbound request. It is
// a thin wrapper around ops.ApplyThinkingEffort so the scenario-level and
// rule-level chains share a single implementation.
//
// Only added to the chain when Effort is non-default ("" / "by client").
type ThinkingEffortTransform struct {
	Effort string
}

// NewThinkingEffortTransform creates a ThinkingEffortTransform.
func NewThinkingEffortTransform(effort string) *ThinkingEffortTransform {
	return &ThinkingEffortTransform{Effort: effort}
}

func (t *ThinkingEffortTransform) Name() string { return "thinking_effort" }

func (t *ThinkingEffortTransform) Apply(ctx *protocoltransform.TransformContext) error {
	ops.ApplyThinkingEffort(ctx.Request, t.Effort)
	return nil
}
