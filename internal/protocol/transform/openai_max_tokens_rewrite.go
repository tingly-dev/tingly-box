package transform

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// OpenAIMaxTokensRewriteTransform rewrites the OpenAI Chat `max_tokens` /
// `max_completion_tokens` field pair based on per-rule flags. It runs as a
// post-base stage so it sees the request in its target shape — protocol
// conversion (Anthropic ↔ OpenAI) has already happened by then.
//
// The transform is a thin shell: ops.ApplyMaxCompletionTokensRewrite and
// ops.ApplyMaxTokensRewrite are the operation primitives; this stage only
// decides when to invoke them based on the rule and the post-base request
// shape.
//
// Only added to the chain when at least one of the two flags is set.
type OpenAIMaxTokensRewriteTransform struct {
	UseMaxCompletionTokens bool
	UseMaxTokens           bool
}

// NewOpenAIMaxTokensRewriteTransform creates a new transform configured with
// the rule flags.
func NewOpenAIMaxTokensRewriteTransform(useMaxCompletionTokens, useMaxTokens bool) *OpenAIMaxTokensRewriteTransform {
	return &OpenAIMaxTokensRewriteTransform{
		UseMaxCompletionTokens: useMaxCompletionTokens,
		UseMaxTokens:           useMaxTokens,
	}
}

func (t *OpenAIMaxTokensRewriteTransform) Name() string {
	return "openai_max_tokens_rewrite"
}

// Apply rewrites the token field on OpenAI Chat requests. For any other
// post-base shape (Anthropic, Responses, Google) it is a no-op.
func (t *OpenAIMaxTokensRewriteTransform) Apply(ctx *TransformContext) error {
	req, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil
	}
	if t.UseMaxCompletionTokens {
		ops.ApplyMaxCompletionTokensRewrite(req)
	}
	if t.UseMaxTokens {
		ops.ApplyMaxTokensRewrite(req)
	}
	return nil
}
