package llmclient

import (
	"context"
	"iter"
	"net/http"

	"google.golang.org/genai"

	"tingly-box/internal/llmclient/httpclient"
	"tingly-box/internal/record"
	"tingly-box/internal/typ"
)

// GoogleClient wraps the Google genai SDK client
type GoogleClient struct {
	client     *genai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *record.Sink
}

// NewGoogleClient creates a new Google client wrapper
func NewGoogleClient(provider *typ.Provider) (*GoogleClient, error) {
	// Create base HTTP client
	var httpClient *http.Client
	if provider.ProxyURL != "" {
		httpClient = httpclient.CreateHTTPClientWithProxy(provider.ProxyURL)
	} else {
		httpClient = http.DefaultClient
	}

	// Create Google client config
	config := &genai.ClientConfig{
		APIKey: provider.GetAccessToken(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: provider.APIBase,
		},
		HTTPClient: httpClient,
	}

	// Create Google client (requires context)
	ctx := context.Background()
	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return &GoogleClient{
		client:     client,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *GoogleClient) ProviderType() ProviderType {
	return ProviderTypeGoogle
}

// Close closes any resources held by the client
func (c *GoogleClient) Close() error {
	// genai.Client doesn't need explicit closing
	return nil
}

// Client returns the underlying Google genai SDK client
func (c *GoogleClient) Client() *genai.Client {
	return c.client
}

// GenerateContent generates content using the Google API
func (c *GoogleClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return c.client.Models.GenerateContent(ctx, model, contents, config)
}

// GenerateContentStream generates content using streaming
func (c *GoogleClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	return c.client.Models.GenerateContentStream(ctx, model, contents, config)
}

// SetRecordSink sets the record sink for the client
func (c *GoogleClient) SetRecordSink(sink *record.Sink) {
	c.recordSink = sink
	if sink != nil && sink.IsEnabled() {
		c.applyRecordMode()
	}
}

// applyRecordMode wraps the HTTP client with a record round tripper
func (c *GoogleClient) applyRecordMode() {
	if c.recordSink == nil {
		return
	}
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider.Name, "")
}

// GetProvider returns the provider for this client
func (c *GoogleClient) GetProvider() *typ.Provider {
	return c.provider
}
