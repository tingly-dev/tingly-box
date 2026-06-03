package request

import (
	"github.com/openai/openai-go/v3/packages/param"
)

// ParamOpt wraps param.NewOpt for the OpenAI SDK.
// Kept as a named alias to avoid import collisions when both Anthropic and OpenAI
// SDK param packages are used in the same file (both export NewOpt/Opt).
func ParamOpt[T comparable](value T) param.Opt[T] {
	return param.NewOpt(value)
}
