package request

import (
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

func AnthropicParamOpt[T comparable](value T) param.Opt[T] {
	return param.NewOpt(value)
}
