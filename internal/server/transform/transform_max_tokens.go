package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
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
	case *openai.ChatCompletionNewParams:
		t.applyOpenAIChat(req)
	case *responses.ResponseNewParams:
		t.applyOpenAIResponses(req)
	}
	return nil
}

func (t *MaxTokensTransform) applyAnthropicV1(req *anthropic.MessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = int64(t.DefaultMaxTokens)
	}
	if t.MaxAllowed > 0 {
		maxAllowed := int64(t.MaxAllowed)
		if req.MaxTokens > maxAllowed {
			req.MaxTokens = maxAllowed
		}
		if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
			if *thinkBudget > maxAllowed {
				req.Thinking = anthropic.ThinkingConfigParamOfEnabled(max(1024, int64(t.MaxAllowed/10)))
			}
			// Anthropic enforces budget_tokens <= max_tokens. max_tokens is a hard
			// operator limit — cap the budget rather than raising the limit.
			if budget := req.Thinking.GetBudgetTokens(); budget != nil && *budget > req.MaxTokens {
				req.Thinking = anthropic.ThinkingConfigParamOfEnabled(max(1024, req.MaxTokens))
			}
		}
	}
}

func (t *MaxTokensTransform) applyAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = int64(t.DefaultMaxTokens)
	}
	if t.MaxAllowed > 0 {
		maxAllowed := int64(t.MaxAllowed)
		if req.MaxTokens > maxAllowed {
			req.MaxTokens = maxAllowed
		}
		if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
			if *thinkBudget > maxAllowed {
				req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(max(1024, int64(t.MaxAllowed/10)))
			}
			// Anthropic enforces budget_tokens <= max_tokens. max_tokens is a hard
			// operator limit — cap the budget rather than raising the limit.
			if budget := req.Thinking.GetBudgetTokens(); budget != nil && *budget > req.MaxTokens {
				req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(max(1024, req.MaxTokens))
			}
		}
	}
}

// applyOpenAIChat mirrors the Anthropic rules for Chat Completions' two
// competing fields: max_completion_tokens is the modern replacement for the
// deprecated max_tokens, so when the caller already set max_completion_tokens
// we cap it and leave max_tokens alone rather than also auto-filling it.
func (t *MaxTokensTransform) applyOpenAIChat(req *openai.ChatCompletionNewParams) {
	if req.MaxCompletionTokens.Valid() {
		if t.MaxAllowed > 0 && req.MaxCompletionTokens.Value > int64(t.MaxAllowed) {
			req.MaxCompletionTokens = param.NewOpt(int64(t.MaxAllowed))
		}
		return
	}
	if !req.MaxTokens.Valid() {
		if t.DefaultMaxTokens > 0 {
			req.MaxTokens = param.NewOpt(int64(t.DefaultMaxTokens))
		}
		return
	}
	if t.MaxAllowed > 0 && req.MaxTokens.Value > int64(t.MaxAllowed) {
		req.MaxTokens = param.NewOpt(int64(t.MaxAllowed))
	}
}

func (t *MaxTokensTransform) applyOpenAIResponses(req *responses.ResponseNewParams) {
	if !req.MaxOutputTokens.Valid() {
		if t.DefaultMaxTokens > 0 {
			req.MaxOutputTokens = param.NewOpt(int64(t.DefaultMaxTokens))
		}
		return
	}
	if t.MaxAllowed > 0 && req.MaxOutputTokens.Value > int64(t.MaxAllowed) {
		req.MaxOutputTokens = param.NewOpt(int64(t.MaxAllowed))
	}
}
