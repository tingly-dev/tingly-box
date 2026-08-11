package transform

import (
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// OpenAIChatProviderCleanupTransform removes gateway-only fields after vendor
// transforms and before the request reaches provider-bound observers.
type OpenAIChatProviderCleanupTransform struct{}

func NewOpenAIChatProviderCleanupTransform() *OpenAIChatProviderCleanupTransform {
	return &OpenAIChatProviderCleanupTransform{}
}

func (*OpenAIChatProviderCleanupTransform) Name() string { return "openai_chat_provider_cleanup" }

func (*OpenAIChatProviderCleanupTransform) Apply(ctx *TransformContext) error {
	requestValue, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok || requestValue == nil {
		return fmt.Errorf("OpenAI Chat provider cleanup received %T", ctx.Request)
	}
	request.CleanupOpenaiFields(requestValue)
	ctx.Request = requestValue
	return nil
}
