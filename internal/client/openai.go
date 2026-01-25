package client

import (
	"context"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// chatGPTBackendAPIRewriteTransport is an HTTP transport that rewrites URL paths
// for ChatGPT backend API to use the correct endpoint structure.
// The OpenAI SDK appends /v1/... paths, but ChatGPT backend API uses /codex/... paths.
type chatGPTBackendAPIRewriteTransport struct {
	base http.RoundTripper
}

// RoundTrip rewrites the URL path for ChatGPT backend API requests
func (t *chatGPTBackendAPIRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only process requests to chatgpt.com
	if req.URL.Host == "chatgpt.com" {
		originalPath := req.URL.Path
		newPath := req.URL.Path

		// Pattern 1: Rewrite /backend-api/v1/... to /backend-api/codex/...
		// Example: /backend-api/v1/chat/completions → /backend-api/codex/chat/completions
		if strings.HasPrefix(newPath, "/backend-api/v1/") {
			newPath = strings.Replace(newPath, "/backend-api/v1/", "/backend-api/codex/", 1)
		}

		// Pattern 2: Rewrite /backend-api/responses to /backend-api/codex/responses
		// The Responses API may use a different URL structure than chat completions
		if newPath == "/backend-api/responses" {
			newPath = "/backend-api/codex/responses"
		}

		// Pattern 3: Rewrite /v1/... to /codex/... (if base URL doesn't include /backend-api)
		// Example: /v1/chat/completions → /codex/chat/completions
		if strings.HasPrefix(newPath, "/v1/") && !strings.Contains(newPath, "/codex/") {
			newPath = strings.Replace(newPath, "/v1/", "/codex/", 1)
		}

		// Apply the rewrite if the path was changed
		if newPath != originalPath {
			logrus.Debugf("[ChatGPT] Rewriting URL path: %s -> %s", originalPath, newPath)
			req.URL.Path = newPath
		}
	}

	// Call the base transport
	return t.base.RoundTrip(req)
}

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client     openai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// defaultNewOpenAIClient creates a new OpenAI client wrapper
func defaultNewOpenAIClient(provider *typ.Provider) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
	}

	// Add ChatGPT-specific headers for Codex OAuth (ChatGPT backend API)
	// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
	if provider.APIBase == "https://chatgpt.com/backend-api" {
		// Required headers for ChatGPT backend API
		options = append(options, option.WithHeader("OpenAI-Beta", "responses=experimental"))
		options = append(options, option.WithHeader("originator", "tingly-box"))

		// Add ChatGPT-Account-ID header if available from OAuth metadata
		if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
			if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
				options = append(options, option.WithHeader("ChatGPT-Account-ID", accountID))
			}
		}
	}

	// Create base HTTP client
	var httpClient *http.Client
	// Add proxy if configured
	if provider.ProxyURL != "" {
		httpClient = CreateHTTPClientWithProxy(provider.ProxyURL)
		logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
	} else {
		httpClient = http.DefaultClient
	}

	// Wrap the HTTP client's transport with ChatGPT backend API rewrite transport
	// This rewrites /v1/... paths to /codex/... paths for ChatGPT backend API
	if provider.APIBase == "https://chatgpt.com/backend-api" {
		baseTransport := httpClient.Transport
		if baseTransport == nil {
			baseTransport = http.DefaultTransport
		}
		httpClient.Transport = &chatGPTBackendAPIRewriteTransport{base: baseTransport}
		logrus.Infof("[ChatGPT] Using custom transport for ChatGPT backend API path rewriting")
	}

	options = append(options, option.WithHTTPClient(httpClient))

	openaiClient := openai.NewClient(options...)

	return &OpenAIClient{
		client:     openaiClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *OpenAIClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleOpenAI
}

// Close closes any resources held by the client
func (c *OpenAIClient) Close() error {
	// OpenAI client doesn't need explicit closing
	return nil
}

// Client returns the underlying OpenAI SDK client
func (c *OpenAIClient) Client() *openai.Client {
	return &c.client
}

// HttpClient returns the underlying HTTP client for passthrough/proxy operations
func (c *OpenAIClient) HttpClient() *http.Client {
	return c.httpClient
}

// ChatCompletionsNew creates a new chat completion request
func (c *OpenAIClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, req)
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request
func (c *OpenAIClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, req)
}

// ResponsesNew creates a new Responses API request
func (c *OpenAIClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return c.client.Responses.New(ctx, req)
}

// ResponsesNewStreaming creates a new streaming Responses API request
func (c *OpenAIClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return c.client.Responses.NewStreaming(ctx, req)
}

// SetRecordSink sets the record sink for the client
func (c *OpenAIClient) SetRecordSink(sink *obs.Sink) {
	c.recordSink = sink
	if sink != nil && sink.IsEnabled() {
		c.applyRecordMode()
	}
}

// applyRecordMode wraps the HTTP client with a record round tripper
func (c *OpenAIClient) applyRecordMode() {
	if c.recordSink == nil {
		return
	}
	// Create a record round tripper with a default model (will be updated per request)
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider.Name, "")
}

// GetProvider returns the provider for this client
func (c *OpenAIClient) GetProvider() *typ.Provider {
	return c.provider
}
