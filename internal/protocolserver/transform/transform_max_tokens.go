package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// MaxTokensDefaultTransform fills max_tokens on an Anthropic request that
// arrived without one. max_tokens is required by the Anthropic protocol, so
// this normalizes the client's own request: it runs in the source half of the
// chain and leaves every other shape alone (see
// .design/protocol-stage-pipeline.md).
type MaxTokensDefaultTransform struct {
	DefaultMaxTokens int
}

// NewMaxTokensDefaultTransform creates a MaxTokensDefaultTransform.
func NewMaxTokensDefaultTransform(defaultMaxTokens int) *MaxTokensDefaultTransform {
	return &MaxTokensDefaultTransform{DefaultMaxTokens: defaultMaxTokens}
}

func (t *MaxTokensDefaultTransform) Name() string { return "max_tokens_default" }

func (t *MaxTokensDefaultTransform) Apply(ctx *protocoltransform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		if req.MaxTokens == 0 {
			req.MaxTokens = int64(t.DefaultMaxTokens)
		}
	case *anthropic.BetaMessageNewParams:
		if req.MaxTokens == 0 {
			req.MaxTokens = int64(t.DefaultMaxTokens)
		}
	}
	return nil
}

// OutputLimitTransform bounds the upstream-bound request by the model's
// output-token limit on the provider. It runs in the target half of the
// chain, so it sees the provider's own shape and each rule applies only where
// that shape has the field:
//
//   - Every shape: cap the output-token field(s) at MaxAllowed.
//   - Anthropic: a thinking budget over MaxAllowed shrinks to
//     max(MaxAllowed/10, 1024), and a budget that reaches max_tokens is capped
//     to max_tokens-1 (Anthropic requires 1024 <= budget_tokens < max_tokens;
//     max_tokens is the hard operator limit, so the budget yields, and
//     thinking is turned off when max_tokens leaves no room for it).
//
// OpenAI Chat / Responses have no budget: an Anthropic client's budget was
// already tiered onto reasoning_effort by the conversion, so it is never
// shrunk to fit a limit it does not travel under (#1897).
//
// MaxAllowed <= 0 disables the transform.
type OutputLimitTransform struct {
	MaxAllowed int
}

// NewOutputLimitTransform creates an OutputLimitTransform.
func NewOutputLimitTransform(maxAllowed int) *OutputLimitTransform {
	return &OutputLimitTransform{MaxAllowed: maxAllowed}
}

func (t *OutputLimitTransform) Name() string { return "output_limit" }

func (t *OutputLimitTransform) Apply(ctx *protocoltransform.TransformContext) error {
	if t.MaxAllowed <= 0 {
		return nil
	}
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		t.applyAnthropicV1(req)
	case *anthropic.BetaMessageNewParams:
		t.applyAnthropicBeta(req)
	case *openai.ChatCompletionNewParams:
		t.applyOpenAIChat(req)
	case *responses.ResponseNewParams:
		t.applyOpenAIResponses(req)
	case *protocol.GoogleRequest:
		t.applyGoogle(req)
	}
	return nil
}

func (t *OutputLimitTransform) applyAnthropicV1(req *anthropic.MessageNewParams) {
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxTokens > maxAllowed {
		req.MaxTokens = maxAllowed
	}
	if req.Thinking.OfEnabled == nil {
		return
	}
	if budget, ok := fitThinkingBudget(req.Thinking.OfEnabled.BudgetTokens, req.MaxTokens, maxAllowed); ok {
		req.Thinking.OfEnabled.BudgetTokens = budget
	} else {
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
	}
}

func (t *OutputLimitTransform) applyAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxTokens > maxAllowed {
		req.MaxTokens = maxAllowed
	}
	if req.Thinking.OfEnabled == nil {
		return
	}
	if budget, ok := fitThinkingBudget(req.Thinking.OfEnabled.BudgetTokens, req.MaxTokens, maxAllowed); ok {
		req.Thinking.OfEnabled.BudgetTokens = budget
	} else {
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
	}
}

// minThinkingBudget is Anthropic's smallest accepted budget_tokens.
const minThinkingBudget int64 = 1024

// fitThinkingBudget keeps an Anthropic thinking budget within the model limit
// and within Anthropic's wire rule 1024 <= budget_tokens < max_tokens, without
// raising max_tokens (the hard operator limit). A budget over the model limit
// shrinks to max(1024, limit/10); one that still reaches max_tokens is capped
// to max_tokens-1. ok is false when max_tokens leaves no room for a valid
// budget (<= 1024): thinking has to be turned off.
func fitThinkingBudget(budget, maxTokens, maxAllowed int64) (fitted int64, ok bool) {
	if budget > maxAllowed {
		budget = max(minThinkingBudget, maxAllowed/10)
	}
	if maxTokens <= 0 || budget < maxTokens {
		return budget, true
	}
	if maxTokens <= minThinkingBudget {
		return 0, false
	}
	return maxTokens - 1, true
}

// applyOpenAIChat caps both of Chat Completions' competing limit fields:
// max_completion_tokens (the modern one) and the deprecated max_tokens. An
// absent field stays absent — Chat has no required limit to fill.
func (t *OutputLimitTransform) applyOpenAIChat(req *openai.ChatCompletionNewParams) {
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxCompletionTokens.Valid() && req.MaxCompletionTokens.Value > maxAllowed {
		req.MaxCompletionTokens = param.NewOpt(maxAllowed)
	}
	if req.MaxTokens.Valid() && req.MaxTokens.Value > maxAllowed {
		req.MaxTokens = param.NewOpt(maxAllowed)
	}
}

func (t *OutputLimitTransform) applyOpenAIResponses(req *responses.ResponseNewParams) {
	maxAllowed := int64(t.MaxAllowed)
	if req.MaxOutputTokens.Valid() && req.MaxOutputTokens.Value > maxAllowed {
		req.MaxOutputTokens = param.NewOpt(maxAllowed)
	}
}

func (t *OutputLimitTransform) applyGoogle(req *protocol.GoogleRequest) {
	if req.Config != nil && int64(req.Config.MaxOutputTokens) > int64(t.MaxAllowed) {
		req.Config.MaxOutputTokens = int32(t.MaxAllowed)
	}
}
