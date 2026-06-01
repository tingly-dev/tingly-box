package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// KimiClient wraps OpenAIClient with Kimi-specific behaviors.
// It embeds OpenAIClient to inherit standard OpenAI API functionality,
// while applying Kimi-specific normalization before requests.
//
// Kimi Code OAuth requirements:
// - Requires Kimi-cli impersonation headers (X-Msh-*, User-Agent)
// - Requires model name normalization (strip "kimi-" prefix)
// - Requires tool message format normalization for API compatibility
type KimiClient struct {
	*OpenAIClient
}

// NewKimiClient creates a new Kimi client wrapper.
// The base OpenAIClient is configured with kimiRoundTripper for
// Kimi-cli impersonation headers.
func NewKimiClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*KimiClient, error) {
	if provider.OAuthDetail == nil {
		return nil, fmt.Errorf("kimi client requires OAuth configuration")
	}

	if provider.OAuthDetail.GetIssuer() != ai.IssuerKimiCode {
		return nil, fmt.Errorf("kimi client can only work for Kimi Code providers")
	}

	// Create provider-specific transport with device metadata
	transport := &kimiRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
		deviceID:     getDeviceID(provider),
		deviceName:   oauth.KimiDeviceName(),
		deviceModel:  oauth.KimiDeviceModel(),
		osVersion:    oauth.KimiOsVersion(),
	}

	httpClient := &http.Client{
		Transport: wrapWithLogging(transport, provider),
	}

	options := []option.RequestOption{
		option.WithHTTPClient(httpClient),
	}

	base, err := NewOpenAIClient(provider, model, sessionID, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create base OpenAI client: %w", err)
	}

	return &KimiClient{
		OpenAIClient: base,
	}, nil
}

// getDeviceID extracts the device ID from provider OAuth details.
func getDeviceID(provider *typ.Provider) string {
	if provider != nil && provider.OAuthDetail != nil {
		return provider.OAuthDetail.DeviceID
	}
	return ""
}

// ChatCompletionsNew creates a new chat completion request with Kimi normalization.
func (c *KimiClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	// Apply Kimi-specific normalization
	req = c.normalizeChatRequest(req)

	// Delegate to base client
	return c.OpenAIClient.ChatCompletionsNew(ctx, req)
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request with Kimi normalization.
func (c *KimiClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	// Apply Kimi-specific normalization
	req = c.normalizeChatRequest(req)

	// Delegate to base client
	return c.OpenAIClient.ChatCompletionsNewStreaming(ctx, req)
}

// stripKimiPrefix removes the "kimi-" prefix from model names.
// The prefix check is case-insensitive, but the replacement preserves
// the original case of the remaining part.
// Reference: CLIProxyAPI kimi_executor.go:742-749
//
// Examples:
//
//	"kimi-k2" → "k2"
//	"Kimi-K2" → "Kimi-K2" (case-sensitive, no match)
//	"KiMi-k2" → "k2" (case-insensitive match)
func (c *KimiClient) stripKimiPrefix(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(strings.ToLower(model), "kimi-") {
		return model[5:] // Remove "kimi-" prefix (5 characters)
	}
	return model
}

// normalizeChatRequest applies Kimi-specific normalization to chat requests at SDK level.
// This includes model name normalization and message filtering/transformation.
func (c *KimiClient) normalizeChatRequest(req openai.ChatCompletionNewParams) openai.ChatCompletionNewParams {
	// 1. Strip kimi- prefix from model name
	req.Model = c.stripKimiPrefix(req.Model)

	// 2. Normalize messages if present
	//if len(req.Messages) > 0 {
	//	req.Messages = c.normalizeMessages(req.Messages)
	//}

	return req
}

// normalizeMessages applies Kimi-specific normalization to messages at SDK level.
// This includes filtering empty assistant messages and ensuring proper message structure.
func (c *KimiClient) normalizeMessages(messages []openai.ChatCompletionMessageParamUnion) []openai.ChatCompletionMessageParamUnion {
	normalized := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		// Skip empty assistant messages (no content, no tool calls, no function calls)
		if c.shouldSkipAssistantMessage(msg) {
			continue
		}
		normalized = append(normalized, msg)
	}

	return normalized
}

// shouldSkipAssistantMessage determines if an assistant message should be skipped.
// Messages are skipped if they are assistant messages with no meaningful content.
func (c *KimiClient) shouldSkipAssistantMessage(msg openai.ChatCompletionMessageParamUnion) bool {
	// Check if this is an assistant message
	if msg.OfAssistant == nil {
		return false
	}

	// Keep message if it has tool calls
	if len(msg.OfAssistant.ToolCalls) > 0 {
		return false
	}

	// Keep message if it has function call (deprecated but still needs checking)
	if param.IsOmitted(msg.OfAssistant.FunctionCall) == false {
		return false
	}

	// Keep message if it has meaningful content
	if c.hasMeaningfulContent(msg) {
		return false
	}

	// Skip empty assistant message
	return true
}

// hasMeaningfulContent checks if the message has meaningful content.
func (c *KimiClient) hasMeaningfulContent(msg openai.ChatCompletionMessageParamUnion) bool {
	if msg.OfAssistant == nil {
		return false
	}

	content := msg.OfAssistant.Content
	return c.isContentMeaningful(content)
}

// isContentMeaningful checks if content has meaningful text or data.
func (c *KimiClient) isContentMeaningful(content openai.ChatCompletionAssistantMessageParamContentUnion) bool {
	// Handle string content
	if !param.IsOmitted(content.OfString) {
		text := content.OfString.Value
		return strings.TrimSpace(text) != ""
	}

	// Handle array content (for multi-modal messages)
	if len(content.OfArrayOfContentParts) > 0 {
		for _, part := range content.OfArrayOfContentParts {
			if c.isContentPartMeaningful(part) {
				return true
			}
		}
		return false
	}

	return false
}

// isContentPartMeaningful checks if a content part has meaningful data.
func (c *KimiClient) isContentPartMeaningful(part openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion) bool {
	// Check text content
	if part.OfText != nil {
		text := part.OfText.Text
		return strings.TrimSpace(text) != ""
	}

	// Check refusal content
	if part.OfRefusal != nil {
		return true // Refusal is meaningful content
	}

	return false
}

// SetRecordSink sets the record sink for the client.
// For KimiClient, we delegate to the embedded OpenAIClient.
func (c *KimiClient) SetRecordSink(sink *obs.Sink) {
	c.OpenAIClient.SetRecordSink(sink)
}

// Client returns the underlying OpenAI SDK client.
func (c *KimiClient) Client() *openai.Client {
	return c.OpenAIClient.Client()
}

// GetProvider returns the provider configuration.
func (c *KimiClient) GetProvider() *typ.Provider {
	return c.OpenAIClient.GetProvider()
}

// Close closes the client and releases resources.
func (c *KimiClient) Close() error {
	return c.OpenAIClient.Close()
}

// APIStyle returns the API style (OpenAI) for this client.
func (c *KimiClient) APIStyle() protocol.APIStyle {
	return c.OpenAIClient.APIStyle()
}

// EmbeddingsNew creates embeddings with Kimi model normalization.
func (c *KimiClient) EmbeddingsNew(ctx context.Context, req openai.EmbeddingNewParams) (*openai.CreateEmbeddingResponse, error) {
	return nil, &ErrKimiNotSupported{
		Operation: "Responses API",
		Reason:    "Kimi Code API does not support /embeddings endpoint",
	}
}

// ImagesGenerate is not supported by Kimi Code OAuth providers.
func (c *KimiClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	return nil, &ErrKimiNotSupported{
		Operation: "Image Generation",
		Reason:    "Kimi Code API does not support /images/generations endpoint",
	}
}

// ResponsesNew is not supported by Kimi Code OAuth providers.
func (c *KimiClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return nil, &ErrKimiNotSupported{
		Operation: "Responses API",
		Reason:    "Kimi Code API does not support /responses endpoint",
	}
}

// ResponsesNewStreaming is not supported by Kimi Code OAuth providers.
func (c *KimiClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	logrus.WithContext(ctx).Errorf("[Kimi] Responses API not supported")
	return nil
}

// ListModels returns the list of available models.
func (c *KimiClient) ListModels(ctx context.Context) ([]string, error) {
	return c.OpenAIClient.ListModels(ctx)
}

// Probe tests the chat endpoint for Kimi provider.
func (c *KimiClient) Probe(ctx context.Context, model string) ProbeResult {
	return c.OpenAIClient.Probe(ctx, model)
}

// ProbeStream performs a streaming probe with Kimi model normalization.
func (c *KimiClient) ProbeStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error) {
	// Apply Kimi-specific model name normalization
	normalizedModel := c.stripKimiPrefix(model)
	return c.OpenAIClient.ProbeStream(ctx, normalizedModel, message, testMode)
}

// ProbeResponsesStream performs a streaming Responses API probe with Kimi model normalization.
func (c *KimiClient) ProbeResponsesStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error) {
	return nil, &ErrKimiNotSupported{
		Operation: "Responses API",
		Reason:    "Kimi Code API does not support /responses endpoint",
	}
}

// ProbeChatEndpoint tests the chat endpoint with Kimi model normalization.
func (c *KimiClient) ProbeChatEndpoint(ctx context.Context, model string, opts ProbeEndpointOptions) (*ProbeResult, error) {
	// Apply Kimi-specific model name normalization
	normalizedModel := c.stripKimiPrefix(model)
	return c.OpenAIClient.ProbeChatEndpoint(ctx, normalizedModel, opts)
}

// ProbeResponsesEndpoint tests the Responses endpoint with Kimi model normalization.
func (c *KimiClient) ProbeResponsesEndpoint(ctx context.Context, model string, opts ProbeEndpointOptions) (*ProbeResult, error) {
	return nil, &ErrKimiNotSupported{
		Operation: "Responses API",
		Reason:    "Kimi Code API does not support /responses endpoint",
	}
}
