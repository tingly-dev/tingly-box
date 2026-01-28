package client

import (
	"context"
	"net/http"

	"iter"

	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// GoogleClient wraps the Google genai SDK client
type GoogleClient struct {
	client     *genai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// NewGoogleClient creates a new Google client wrapper
func NewGoogleClient(provider *typ.Provider) (*GoogleClient, error) {
	// Create HTTP client with proper OAuth/proxy configuration
	var httpClient *http.Client
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		// Use CreateHTTPClientForProvider which applies OAuth hooks and uses shared transport
		httpClient = CreateHTTPClientForProvider(provider)
		providerType := oauth.ProviderType(provider.OAuthDetail.ProviderType)
		logrus.Infof("Using shared transport for OAuth provider type: %s", providerType)
	} else {
		// For non-OAuth providers, use simple proxy client or default
		if provider.ProxyURL != "" {
			httpClient = CreateHTTPClientWithProxy(provider.ProxyURL)
			logrus.Infof("Using proxy for Google client: %s", provider.ProxyURL)
		} else {
			httpClient = http.DefaultClient
		}
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
func (c *GoogleClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleGoogle
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
func (c *GoogleClient) SetRecordSink(sink *obs.Sink) {
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
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider)
}

// GetProvider returns the provider for this client
func (c *GoogleClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the Google Gemini API
// Note: Google genai SDK doesn't have a direct ListModels method, so we return
// ErrModelsEndpointNotSupported to signal the caller to use template fallback.
func (c *GoogleClient) ListModels(ctx context.Context) ([]string, error) {
	// Google genai SDK doesn't provide a models list endpoint
	// The caller should use template fallback instead
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "Google genai SDK does not support listing models via API",
	}
}
