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
	// Codex: supported values are: 'png', 'webp', and 'jpeg'
	if req.ResponseFormat != "" {
		tool["output_format"] = string(req.ResponseFormat)
	} else {
		// Default to b64_json for Codex
		tool["output_format"] = "png"
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
//
// The image data comes through two event types:
// 1. response.image_generation_call.partial_image - streaming partial image chunks
// 2. response.output_item.done - final status of the image generation call
func (c *CodexClient) parseImageGenerationStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*openai.ImagesResponse, error) {
	defer stream.Close()

	var b64JSON string
	var imageCallID string

	// Collect image data from stream events
	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.image_generation_call.partial_image":
			// Partial image chunks during generation
			partialEvent := event.AsResponseImageGenerationCallPartialImage()
			if partialEvent.PartialImageB64 != "" {
				b64JSON += partialEvent.PartialImageB64
				imageCallID = partialEvent.ItemID
				logrus.Debugf("[Codex] Received partial image chunk, item_id: %s, total_size: %d",
					partialEvent.ItemID, len(b64JSON))
			}

		case "response.output_item.done":
			// Final status of output items
			doneEvent := event.AsResponseOutputItemDone()
			item := doneEvent.Item

			if item.Type == "image_generation_call" {
				imageCall := item.AsImageGenerationCall()
				// Check for image result in the done event
				// The status can be "generating", "completed", or other values
				// If we haven't received partial images, use the Result field
				if b64JSON == "" && imageCall.Result != "" {
					b64JSON = imageCall.Result
					imageCallID = imageCall.ID
					logrus.Debugf("[Codex] Received image result in done event, id: %s, status: %s",
						imageCall.ID, imageCall.Status)
				}
				// Update imageCallID even if we already have data from partial events
				if imageCallID == "" {
					imageCallID = imageCall.ID
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	if b64JSON == "" {
		return nil, fmt.Errorf("no image data in response (image_call_id: %s)", imageCallID)
	}

	logrus.Infof("[Codex] Successfully extracted image data, id: %s, size: %d bytes", imageCallID, len(b64JSON))

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
