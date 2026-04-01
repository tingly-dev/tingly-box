package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ThinkingModeTransform controls how extended thinking is enabled based on scenario config.
//
// Supported modes:
//   - Force: always enable thinking with the configured budget.
//   - Adaptive: convert any existing thinking config (OfEnabled or OfAdaptive) to OfEnabled.
//   - Default: only convert OfEnabled to use the configured budget; leave OfAdaptive untouched.
//   - Any other value: disable thinking entirely.
//
// Budget resolution priority: client-provided budget > effort-level mapping > medium fallback.
// Only added to the chain when ThinkingMode is non-empty.
type ThinkingModeTransform struct {
	ScenarioConfig *typ.ScenarioConfig
}

// NewThinkingModeTransform creates a ThinkingModeTransform.
func NewThinkingModeTransform(scenarioConfig *typ.ScenarioConfig) *ThinkingModeTransform {
	return &ThinkingModeTransform{ScenarioConfig: scenarioConfig}
}

func (t *ThinkingModeTransform) Name() string { return "thinking_mode" }

func (t *ThinkingModeTransform) Apply(ctx *protocoltransform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		t.applyAnthropicV1(req)
	case *anthropic.BetaMessageNewParams:
		t.applyAnthropicBeta(req)
	}
	return nil
}

// resolveBudgetTokens determines the thinking budget to use.
// Client-provided budget takes precedence over effort-level mapping.
func (t *ThinkingModeTransform) resolveBudgetTokens(thinkBudget *int64) int64 {
	effort := t.ScenarioConfig.Flags.ThinkingEffort
	if effort == typ.ThinkingEffortDefault {
		effort = typ.ThinkingEffortMedium
	}
	budgetTokens, ok := typ.ThinkingBudgetMapping[effort]
	if !ok {
		budgetTokens = typ.ThinkingBudgetMapping[typ.ThinkingEffortMedium]
	}
	if thinkBudget != nil {
		budgetTokens = *thinkBudget
	}
	return budgetTokens
}

func (t *ThinkingModeTransform) applyAnthropicV1(req *anthropic.MessageNewParams) {
	budgetTokens := t.resolveBudgetTokens(req.Thinking.GetBudgetTokens())

	switch typ.ThinkingMode(t.ScenarioConfig.Flags.ThinkingMode) {
	case typ.ThinkingModeForce:
		req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
	case typ.ThinkingModeAdaptive:
		switch {
		case req.Thinking.OfEnabled != nil:
			req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
		case req.Thinking.OfAdaptive != nil:
			req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
		}
	case typ.ThinkingModeDefault:
		if req.Thinking.OfEnabled != nil {
			req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
		}
	default:
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
	}
}

func (t *ThinkingModeTransform) applyAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	budgetTokens := t.resolveBudgetTokens(req.Thinking.GetBudgetTokens())

	switch typ.ThinkingMode(t.ScenarioConfig.Flags.ThinkingMode) {
	case typ.ThinkingModeForce:
		req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
	case typ.ThinkingModeAdaptive:
		switch {
		case req.Thinking.OfEnabled != nil:
			req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
		case req.Thinking.OfAdaptive != nil:
			req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
		}
	case typ.ThinkingModeDefault:
		if req.Thinking.OfEnabled != nil {
			req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
		}
	default:
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
	}
}
