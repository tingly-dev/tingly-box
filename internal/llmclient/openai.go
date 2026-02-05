package llmclient

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"tingly-box/internal/llmclient/httpclient"
	"tingly-box/internal/typ"
)

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client     openai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
}

// defaultNewOpenAIClient creates a new OpenAI client wrapper
func defaultNewOpenAIClient(provider *typ.Provider) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
	}

	// Create base HTTP client
	var httpClient *http.Client
	// Add proxy if configured
	if provider.ProxyURL != "" {
		httpClient = httpclient.CreateHTTPClientWithProxy(provider.ProxyURL)
		options = append(options, option.WithHTTPClient(httpClient))
		logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
	} else {
		// Create a dedicated HTTP client for this provider to avoid
		// connection pool interference with other providers
		httpClient = &http.Client{}
		options = append(options, option.WithHTTPClient(httpClient))
	}

	openaiClient := openai.NewClient(options...)

	return &OpenAIClient{
		client:     openaiClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *OpenAIClient) ProviderType() ProviderType {
	return ProviderTypeOpenAI
}

// Close closes any resources held by the client
func (c *OpenAIClient) Close() error {
	// OpenAI client doesn't need explicit closing
	return nil
}

// SetMode sets the client mode. When debug is true, all API headers are logged as indented JSON.
func (c *OpenAIClient) SetMode(debug bool) {
	c.debugMode = debug
	if debug {
		c.applyDebugMode()
	}
}

// applyDebugMode wraps the HTTP client with a debug round tripper
func (c *OpenAIClient) applyDebugMode() {
	c.httpClient.Transport = NewDebugRoundTripper(c.httpClient.Transport)
}

// Client returns the underlying OpenAI SDK client
func (c *OpenAIClient) Client() *openai.Client {
	return &c.client
}

// ChatCompletionsNew creates a new chat completion request
func (c *OpenAIClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, req)
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request
func (c *OpenAIClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, req)
}
