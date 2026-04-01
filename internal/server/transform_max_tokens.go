package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// MaxTokensTransform ensures max_tokens is set and properly bounded for Anthropic requests.
//
// It applies three rules:
//  1. Fill defaultMaxTokens when the client sends 0.
//  2. Cap max_tokens at maxAllowed.
//  3. If the thinking budget exceeds maxAllowed, shrink it to max(maxAllowed/10, 1024).
type MaxTokensTransform struct {
	DefaultMaxTokens int
	MaxAllowed       int
}

// NewMaxTokensTransform creates a MaxTokensTransform.
func NewMaxTokensTransform(defaultMaxTokens, maxAllowed int) *MaxTokensTransform {
	return &MaxTokensTransform{
		DefaultMaxTokens: defaultMaxTokens,
		MaxAllowed:       maxAllowed,
	}
}

func (t *MaxTokensTransform) Name() string { return "max_tokens" }

func (t *MaxTokensTransform) Apply(ctx *protocoltransform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		t.applyAnthropicV1(req)
	case *anthropic.BetaMessageNewParams:
		t.applyAnthropicBeta(req)
	}
	return nil
}

func (t *MaxTokensTransform) applyAnthropicV1(req *anthropic.MessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = int64(t.DefaultMaxTokens)
	}
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxTokens > maxAllowed {
		req.MaxTokens = maxAllowed
	}
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil && *thinkBudget > maxAllowed {
		req.Thinking = anthropic.ThinkingConfigParamOfEnabled(max(1024, int64(t.MaxAllowed/10)))
	}
}

func (t *MaxTokensTransform) applyAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = int64(t.DefaultMaxTokens)
	}
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxTokens > maxAllowed {
		req.MaxTokens = maxAllowed
	}
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil && *thinkBudget > maxAllowed {
		req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(max(1024, int64(t.MaxAllowed/10)))
	}
}
