package server

import (
	"context"
	"iter"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"google.golang.org/genai"
)

// ForwardContext is an alias for forwarding.ForwardContext
type ForwardContext forwarding.ForwardContext

// NewForwardContext wraps forwarding.NewForwardContext for backward compatibility
func NewForwardContext(baseCtx context.Context, provider *typ.Provider) *forwarding.ForwardContext {
	return forwarding.NewForwardContext(baseCtx, provider)
}

// Forward functions - wrappers around forwarding package functions

func ForwardAnthropicV1(fc *forwarding.ForwardContext, wrapper *client.AnthropicClient, req *anthropic.MessageNewParams) (*anthropic.Message, context.CancelFunc, error) {
	return forwarding.ForwardAnthropicV1(fc, wrapper, req)
}

func ForwardAnthropicV1Stream(fc *forwarding.ForwardContext, wrapper *client.AnthropicClient, req *anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], context.CancelFunc, error) {
	return forwarding.ForwardAnthropicV1Stream(fc, wrapper, req)
}

func ForwardAnthropicV1Beta(fc *forwarding.ForwardContext, wrapper *client.AnthropicClient, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, context.CancelFunc, error) {
	return forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
}

func ForwardAnthropicV1BetaStream(fc *forwarding.ForwardContext, wrapper *client.AnthropicClient, req *anthropic.BetaMessageNewParams) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], context.CancelFunc, error) {
	return forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
}

func ForwardOpenAIChat(fc *forwarding.ForwardContext, wrapper *client.OpenAIClient, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, context.CancelFunc, error) {
	return forwarding.ForwardOpenAIChat(fc, wrapper, req)
}

func ForwardOpenAIChatStream(fc *forwarding.ForwardContext, wrapper *client.OpenAIClient, req *openai.ChatCompletionNewParams) (*openaistream.Stream[openai.ChatCompletionChunk], context.CancelFunc, error) {
	return forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
}

func ForwardOpenAIResponses(fc *forwarding.ForwardContext, wrapper *client.OpenAIClient, params responses.ResponseNewParams) (*responses.Response, context.CancelFunc, error) {
	return forwarding.ForwardOpenAIResponses(fc, wrapper, params)
}

func ForwardOpenAIResponsesStream(fc *forwarding.ForwardContext, wrapper *client.OpenAIClient, params responses.ResponseNewParams) (*openaistream.Stream[responses.ResponseStreamEventUnion], context.CancelFunc, error) {
	return forwarding.ForwardOpenAIResponsesStream(fc, wrapper, params)
}

func ForwardGoogle(fc *forwarding.ForwardContext, wrapper *client.GoogleClient, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, context.CancelFunc, error) {
	return forwarding.ForwardGoogle(fc, wrapper, model, contents, config)
}

func ForwardGoogleStream(fc *forwarding.ForwardContext, wrapper *client.GoogleClient, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], context.CancelFunc, error) {
	return forwarding.ForwardGoogleStream(fc, wrapper, model, contents, config)
}
