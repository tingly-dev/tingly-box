package request

import (
	"github.com/openai/openai-go/v3/packages/param"
)

func ParamOpt[T comparable](value T) param.Opt[T] {
	return param.NewOpt(value)
}
