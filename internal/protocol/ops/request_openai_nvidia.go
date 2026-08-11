package ops

import (
	"github.com/openai/openai-go/v3"
)

// stripPromptCacheForNVIDIA removes prompt-cache fields that the NVIDIA NIM
// OpenAI-compatible endpoint rejects with:
//
//	400 Bad Request: {"message":"Validation: Unsupported parameter(s): `prompt_cache_options`"}
//
// Claude Code sends `prompt_cache_options` / `prompt_cache_retention` at the
// request top level on every multi-turn request. Both fields are tagged
// `omitzero` in the SDK, so zeroing them omits them from the marshaled request
// without a JSON round-trip — which would drop per-message extra fields such
// as `x_thinking` / `reasoning_content`.
func stripPromptCacheForNVIDIA(req *openai.ChatCompletionNewParams) *openai.ChatCompletionNewParams {
	req.PromptCacheOptions = openai.ChatCompletionNewParamsPromptCacheOptions{}
	req.PromptCacheRetention = ""

	return req
}
