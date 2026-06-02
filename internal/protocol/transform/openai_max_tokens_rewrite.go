package transform

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

// OpenAIMaxTokensRewriteTransform rewrites the OpenAI Chat `max_tokens` /
// `max_completion_tokens` field pair based on per-rule flags. It runs as a
// post-base stage so it sees the request in its target shape — protocol
// conversion (Anthropic ↔ OpenAI) has already happened by then.
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
		applyMaxCompletionTokensRewrite(req)
	}
	if t.UseMaxTokens {
		applyMaxTokensRewrite(req)
	}
	return nil
}

// applyMaxCompletionTokensRewrite moves the value of `max_tokens` into the
// newer `max_completion_tokens` field. OpenAI's o1/o3/gpt-5 families reject
// `max_tokens`; this rewrite lets callers opt in per rule.
//
// The cleared field uses the zero `param.Opt[int64]{}` so the SDK's
// `omitzero` JSON tag drops it from the wire body instead of emitting
// `"max_tokens": 0`.
func applyMaxCompletionTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxTokens.Valid() {
		req.MaxCompletionTokens = param.NewOpt(req.MaxTokens.Value)
		req.MaxTokens = param.Opt[int64]{}
	}
}

// applyMaxTokensRewrite moves the value of `max_completion_tokens` back into
// the legacy `max_tokens` field. Some providers and older model endpoints
// reject the newer field name; this rewrite lets callers force the legacy
// field per rule.
func applyMaxTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxCompletionTokens.Valid() {
		req.MaxTokens = param.NewOpt(req.MaxCompletionTokens.Value)
		req.MaxCompletionTokens = param.Opt[int64]{}
	}
}
