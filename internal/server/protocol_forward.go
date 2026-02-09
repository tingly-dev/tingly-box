package server

import (
	"context"
	"fmt"
	"iter"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/protocol/request/transformer"
)

// ===================================================================
// Anthropic Forward Functions
// ===================================================================

// ForwardAnthropicV1 sends a non-streaming Anthropic v1 message request.
func ForwardAnthropicV1(fc *ForwardContext, req anthropic.MessageNewParams) (*anthropic.Message, context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetAnthropicClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	message, err := wrapper.MessagesNew(ctx, req)
	fc.Complete(ctx, message, err)

	if err != nil {
		cancel()
		return nil, nil, err
	}

	return message, cancel, nil
}

// ForwardAnthropicV1Stream sends a streaming Anthropic v1 message request.
// Note: Set BaseCtx via WithBaseCtx() to support client cancellation.
func ForwardAnthropicV1Stream(fc *ForwardContext, req anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetAnthropicClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	logrus.Debugln("Creating Anthropic v1 streaming request")
	stream := wrapper.MessagesNewStreaming(ctx, req)
	return stream, cancel, nil
}

// ForwardAnthropicV1Beta sends a non-streaming Anthropic v1 beta message request.
func ForwardAnthropicV1Beta(fc *ForwardContext, req anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetAnthropicClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	message, err := wrapper.BetaMessagesNew(ctx, req)
	fc.Complete(ctx, message, err)

	if err != nil {
		cancel()
		return nil, nil, err
	}

	return message, cancel, nil
}

// ForwardAnthropicV1BetaStream sends a streaming Anthropic v1 beta message request.
// Note: Set BaseCtx via WithBaseCtx() to support client cancellation.
func ForwardAnthropicV1BetaStream(fc *ForwardContext, req anthropic.BetaMessageNewParams) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetAnthropicClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get Anthropic client for provider: %s", fc.Provider.Name)
	}

	logrus.Debugln("Creating Anthropic v1 beta streaming request")
	stream := wrapper.BetaMessagesNewStreaming(ctx, req)
	return stream, cancel, nil
}

// ===================================================================
// OpenAI Forward Functions
// ===================================================================

// ForwardOpenAIChat sends a non-streaming OpenAI chat completion request.
func ForwardOpenAIChat(fc *ForwardContext, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, err
	}
	defer cancel()

	wrapper := fc.ClientPool.GetOpenAIClient(fc.Provider, fc.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	// Apply provider-specific transformations
	config := buildOpenAIConfig(req)
	transformedReq := transformer.ApplyProviderTransforms(req, fc.Provider, fc.Model, config)
	*req = *transformedReq

	// Clear empty tools array
	if len(req.Tools) == 0 {
		req.Tools = nil
	}

	logrus.Infof("provider: %s, model: %s", fc.Provider.Name, req.Model)

	resp, err := wrapper.ChatCompletionsNew(ctx, *req)
	fc.Complete(ctx, resp, err)

	return resp, err
}

// ForwardOpenAIChatStream sends a streaming OpenAI chat completion request.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardOpenAIChatStream(fc *ForwardContext, req *openai.ChatCompletionNewParams) (*openaistream.Stream[openai.ChatCompletionChunk], context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(req)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetOpenAIClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	logrus.Debugf("provider: %s (streaming)", fc.Provider.Name)

	// Apply provider-specific transformations
	config := buildOpenAIConfig(req)
	transformedReq := transformer.ApplyProviderTransforms(req, fc.Provider, fc.Model, config)
	*req = *transformedReq

	// Clear empty tools array
	if len(req.Tools) == 0 {
		req.Tools = nil
	}

	stream := wrapper.ChatCompletionsNewStreaming(ctx, *req)
	return stream, cancel, nil
}

// ForwardOpenAIResponses sends a non-streaming OpenAI Responses API request.
func ForwardOpenAIResponses(fc *ForwardContext, params responses.ResponseNewParams) (*responses.Response, error) {
	ctx, cancel, err := fc.PrepareContext(params)
	if err != nil {
		return nil, err
	}
	defer cancel()

	wrapper := fc.ClientPool.GetOpenAIClient(fc.Provider, fc.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	resp, err := wrapper.ResponsesNew(ctx, params)
	fc.Complete(ctx, resp, err)

	return resp, err
}

// ForwardOpenAIResponsesStream sends a streaming OpenAI Responses API request.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardOpenAIResponsesStream(fc *ForwardContext, params responses.ResponseNewParams) (*openaistream.Stream[responses.ResponseStreamEventUnion], context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(params)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetOpenAIClient(fc.Provider, fc.Model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get OpenAI client for provider: %s", fc.Provider.Name)
	}

	stream := wrapper.ResponsesNewStreaming(ctx, params)
	return stream, cancel, nil
}

// ===================================================================
// Google Forward Functions
// ===================================================================

// ForwardGoogle sends a non-streaming Google Generative AI request.
func ForwardGoogle(fc *ForwardContext, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	ctx, cancel, err := fc.PrepareContext(nil)
	if err != nil {
		return nil, err
	}
	defer cancel()

	wrapper := fc.ClientPool.GetGoogleClient(fc.Provider, model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get Google client for provider: %s", fc.Provider.Name)
	}

	resp, err := wrapper.GenerateContent(ctx, model, contents, config)
	fc.Complete(ctx, resp, err)

	return resp, err
}

// ForwardGoogleStream sends a streaming Google Generative AI request.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardGoogleStream(fc *ForwardContext, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], context.CancelFunc, error) {
	ctx, cancel, err := fc.PrepareContext(nil)
	if err != nil {
		return nil, nil, err
	}

	wrapper := fc.ClientPool.GetGoogleClient(fc.Provider, model)
	if wrapper == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to get Google client for provider: %s", fc.Provider.Name)
	}

	logrus.Debugln("Creating Google streaming request")
	stream := wrapper.GenerateContentStream(ctx, model, contents, config)
	return stream, cancel, nil
}

// ===================================================================
// Helper Functions
// ===================================================================

// buildOpenAIConfig builds the OpenAIConfig for provider transformations.
func buildOpenAIConfig(req *openai.ChatCompletionNewParams) *transformer.OpenAIConfig {
	config := &transformer.OpenAIConfig{
		HasThinking:     false,
		ReasoningEffort: "",
	}

	// Check if request has thinking configuration in extra_fields
	extraFields := req.ExtraFields()
	if thinking, ok := extraFields["thinking"]; ok {
		if _, ok := thinking.(map[string]interface{}); ok {
			config.HasThinking = true
			// Set default reasoning effort to "low" for OpenAI-compatible APIs
			config.ReasoningEffort = "low"
		}
	}

	return config
}
