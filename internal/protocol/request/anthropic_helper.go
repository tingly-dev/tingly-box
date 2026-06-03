package request

import (
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// AnthropicParamOpt wraps param.NewOpt for the Anthropic SDK.
// Kept as a named alias to avoid import collisions when both Anthropic and OpenAI
// SDK param packages are used in the same file (both export NewOpt/Opt).
func AnthropicParamOpt[T comparable](value T) param.Opt[T] {
	return param.NewOpt(value)
}
