package client

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CodexClient wraps OpenAIClient with Codex-specific behaviors.
// It embeds OpenAIClient to inherit standard OpenAI API functionality,
// while overriding methods that require special handling for ChatGPT backend API.
//
// Codex (ChatGPT OAuth) limitations:
// - Does NOT support standard Chat Completions API
// - Does NOT support /models endpoint
// - Does NOT support /images/generations endpoint
// - ONLY supports Responses API with special parameters
type CodexClient struct {
	*OpenAIClient
}

// NewCodexClient creates a new Codex client wrapper.
// The base OpenAIClient is configured with codexRoundTripper for path/header transformation.
func NewCodexClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*CodexClient, error) {
	base, err := NewOpenAIClient(provider, model, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create base OpenAI client: %w", err)
	}

	// Set the override function for chat completions (reject with error)
	base.chatCompletionsHandler = func(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return nil, &ErrCodexNotSupported{
			Operation: "Chat Completions",
			Reason:    "ChatGPT backend API does not support standard /v1/chat/completions endpoint. Use Responses API instead.",
		}
	}

	// Set the override function for streaming chat completions (reject with error)
	base.chatCompletionsStreamingHandler = func(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
		logrus.Errorf("[Codex] Chat Completions Streaming not supported, use Responses API instead")
		return nil
	}

	// Set the override function for image generation (use Responses API)
	base.imagesGenerateHandler = func(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
		return (&CodexClient{OpenAIClient: base}).ImagesGenerate(ctx, req)
	}

	return &CodexClient{
		OpenAIClient: base,
	}, nil
}

// ChatCompletionsNew creates a new chat completion request.
// For Codex, this returns an error as ChatGPT backend API does not support standard Chat Completions.
// Use Responses API instead.
func (c *CodexClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return nil, &ErrCodexNotSupported{
		Operation: "Chat Completions",
		Reason:    "ChatGPT backend API does not support standard /v1/chat/completions endpoint. Use Responses API instead.",
	}
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request.
// For Codex, this returns nil as ChatGPT backend API does not support standard Chat Completions.
// Use Responses API instead.
func (c *CodexClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	logrus.Errorf("[Codex] Chat Completions Streaming not supported, use Responses API instead")
	return nil
}

// ImagesGenerate creates a new image generation request.
// For Codex, this transforms the request to use the Responses API with the image_generation tool,
// as ChatGPT backend API does not support the standard /images/generations endpoint.
func (c *CodexClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	logrus.Debugf("[Codex] Using Responses API for image generation, model: %s", req.Model)

	// Build Responses API request
	responsesReq := c.buildImageGenerationResponsesRequest(req)

	// Call streaming Responses API
	stream := c.ResponsesNewStreaming(ctx, responsesReq)

	// Parse streaming response
	return c.parseImageGenerationStream(ctx, stream)
}

// ListModels returns the list of available models.
// For Codex, this returns an error as ChatGPT OAuth tokens cannot access /models endpoint.
func (c *CodexClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "ChatGPT OAuth token cannot access /models endpoint",
	}
}

// buildImageGenerationResponsesRequest transforms ImageGenerateParams into
// a Responses API request with the image_generation tool.
func (c *CodexClient) buildImageGenerationResponsesRequest(req openai.ImageGenerateParams) responses.ResponseNewParams {
	// Build input item from prompt
	inputItem := map[string]interface{}{
		"type": "message",
		"role": "user",
		"content": []map[string]string{
			{"type": "input_text", "text": string(req.Prompt)},
		},
	}

	// Build image_generation tool with base parameters
	tool := map[string]interface{}{
		"type": "image_generation",
		"size": string(req.Size),
	}

	// Map quality parameter (if provided)
	// OpenAI: "standard", "hd" -> Codex: "medium", "high"
	if req.Quality != "" {
		quality := string(req.Quality)
		if quality == "standard" {
			tool["quality"] = "medium"
		} else if quality == "hd" {
			tool["quality"] = "high"
		} else {
			tool["quality"] = quality
		}
	}

	// Map response_format to output_format
	// OpenAI: "url", "b64_json" -> Codex: "url", "b64_json"
	if req.ResponseFormat != "" {
		tool["output_format"] = string(req.ResponseFormat)
	} else {
		// Default to b64_json for Codex
		tool["output_format"] = "b64_json"
	}

	// Log warning for unsupported N parameter
	if req.N.Valid() {
		n := req.N.Value
		if n > 1 {
			logrus.Warnf("[Codex] Multiple images (N=%d) not supported, using N=1", n)
		}
	}

	// Log warning for unsupported style parameter
	if req.Style != "" {
		logrus.Warnf("[Codex] Style parameter not supported for image generation")
	}

	// Build the Responses API request
	params := responses.ResponseNewParams{
		Model: req.Model,
	}

	// Use extra fields to set the custom format
	params.SetExtraFields(map[string]interface{}{
		"input":   []interface{}{inputItem},
		"tools":   []interface{}{tool},
		"stream":  true,
		"store":   false,
		"include": []string{"reasoning.encrypted_content"},
	})

	return params
}

// parseImageGenerationStream parses the streaming Responses API response
// and extracts the generated image data from the output array.
func (c *CodexClient) parseImageGenerationStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*openai.ImagesResponse, error) {
	defer stream.Close()

	var b64JSON string

	// Collect output items from response.output_item.done events
	for stream.Next() {
		event := stream.Current()

		// Look for response.output_item.done events
		if event.Type == "response.output_item.done" {
			doneEvent := event.AsResponseOutputItemDone()

			// Extract image data from output item
			item := doneEvent.Item
			if item.Type == "image_generation_call" {
				imageCall := item.AsImageGenerationCall()
				if imageCall.Status == "completed" && imageCall.Result != "" {
					b64JSON = imageCall.Result
					logrus.Debugf("[Codex] Received completed image, id: %s", imageCall.ID)
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	if b64JSON == "" {
		return nil, fmt.Errorf("no image data in response")
	}

	// Build standard ImagesResponse from extracted data
	return &openai.ImagesResponse{
		Data: []openai.Image{
			{
				B64JSON: b64JSON,
			},
		},
	}, nil
}

// IsCodexProvider returns true for CodexClient instances.
func (c *CodexClient) IsCodexProvider() bool {
	return true
}

// GetProvider returns the provider for this client.
func (c *CodexClient) GetProvider() *typ.Provider {
	return c.provider
}
