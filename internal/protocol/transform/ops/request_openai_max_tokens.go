package ops

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

// ApplyMaxCompletionTokensRewrite moves the value of `max_tokens` into the
// newer `max_completion_tokens` field. OpenAI's o1/o3/gpt-5 families reject
// `max_tokens`; this rewrite lets callers opt in per rule.
//
// The cleared field uses the zero `param.Opt[int64]{}` so the SDK's
// `omitzero` JSON tag drops it from the wire body instead of emitting
// `"max_tokens": 0`.
func ApplyMaxCompletionTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxTokens.Valid() {
		req.MaxCompletionTokens = param.NewOpt(req.MaxTokens.Value)
		req.MaxTokens = param.Opt[int64]{}
	}
}

// ApplyMaxTokensRewrite moves the value of `max_completion_tokens` back into
// the legacy `max_tokens` field. Some providers and older model endpoints
// reject the newer field name; this rewrite lets callers force the legacy
// field per rule.
func ApplyMaxTokensRewrite(req *openai.ChatCompletionNewParams) {
	if req == nil {
		return
	}
	if req.MaxCompletionTokens.Valid() {
		req.MaxTokens = param.NewOpt(req.MaxCompletionTokens.Value)
		req.MaxCompletionTokens = param.Opt[int64]{}
	}
}
