package forwarding

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
)

// ForwardOpenAIChat sends a non-streaming OpenAI chat completion request.
// IMPORTANT: All transformations (protocol conversion + vendor-specific) should
// be applied by the transform chain BEFORE calling this function.
func ForwardOpenAIChat(fc *ForwardContext, wrapper *client.OpenAIClient, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(req)

	// Clear empty tools array
	if len(req.Tools) == 0 {
		req.Tools = nil
	}

	logrus.Infof("provider: %s, model: %s", fc.Provider.Name, req.Model)

	resp, err := wrapper.ChatCompletionsNew(ctx, *req)
	fc.Complete(ctx, resp, err)

	return resp, cancel, err
}

// ForwardOpenAIChatStream sends a streaming OpenAI chat completion request.
// IMPORTANT: All transformations (protocol conversion + vendor-specific) should
// be applied by the transform chain BEFORE calling this function.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardOpenAIChatStream(fc *ForwardContext, wrapper *client.OpenAIClient, req *openai.ChatCompletionNewParams) (*openaistream.Stream[openai.ChatCompletionChunk], context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}
	logrus.Debugf("provider: %s (streaming)", fc.Provider.Name)

	ctx, cancel := fc.PrepareContext(req)

	stream := wrapper.ChatCompletionsNewStreaming(ctx, *req)
	return stream, cancel, nil
}

// ForwardOpenAIResponses sends a non-streaming OpenAI Responses API request.
func ForwardOpenAIResponses(fc *ForwardContext, wrapper *client.OpenAIClient, params responses.ResponseNewParams) (*responses.Response, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(params)
	resp, err := wrapper.ResponsesNew(ctx, params)
	fc.Complete(ctx, resp, err)
	return resp, cancel, err
}

// ForwardOpenAIResponsesStream sends a streaming OpenAI Responses API request.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardOpenAIResponsesStream(fc *ForwardContext, wrapper *client.OpenAIClient, params responses.ResponseNewParams) (*openaistream.Stream[responses.ResponseStreamEventUnion], context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(params)
	stream := wrapper.ResponsesNewStreaming(ctx, params)
	return stream, cancel, nil
}
