package client

import (
	"context"
	"iter"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GoogleClient wraps the Google genai SDK client
type GoogleClient struct {
	client     *genai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// NewGoogleClient creates a new Google client wrapper.
// sessionID is used for session-scoped transport creation for OAuth providers.
//
// For provider-specific wire formats (Gemini CLI / Antigravity Code Assist
// envelope), prefer the dedicated *Client constructors (NewGeminiClient,
// NewAntigravityClient) — they layer the right round tripper on top of the
// session-bound transport. NewGoogleClient itself stays generic.
func NewGoogleClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*GoogleClient, error) {
	var transport http.RoundTripper
	if provider.AuthType == typ.AuthTypeOAuth || provider.ProxyURL != "" {
		transport = createSessionBoundTransport(provider, sessionID)

		issuer := ""
		if provider.OAuthDetail != nil {
			issuer = string(provider.OAuthDetail.Issuer)
		}
		if issuer != "" {
			logrus.Infof("Using session-bound transport for OAuth provider type: %s, session: %s", issuer, sessionID.Value)
		}
		if provider.ProxyURL != "" {
			logrus.Infof("Using proxy for Google client: %s", provider.ProxyURL)
		}
	} else {
		// Use the transport pool instead of http.DefaultTransport so that env
		// proxy variables (HTTP_PROXY / HTTPS_PROXY) are not inherited when no
		// proxy is explicitly configured for the provider.
		transport = GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
	}

	// MENTION: must set timeout, otherwise operations may fail unexpectedly
	timeout := time.Duration(provider.Timeout) * time.Second
	if provider.Timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}

	httpClient := &http.Client{
		Transport: wrapWithLogging(transport, provider),
		Timeout:   timeout,
	}
	return newGoogleClientFromHTTPClient(provider, httpClient)
}

// newGoogleClientFromHTTPClient builds a GoogleClient around an already-configured
// http.Client. Provider-specific clients (Gemini, Antigravity) use this to
// inject their round tripper without going through the generic transport path.
func newGoogleClientFromHTTPClient(provider *typ.Provider, httpClient *http.Client) (*GoogleClient, error) {
	httpOptions := genai.HTTPOptions{
		BaseURL: provider.APIBase,
	}

	config := &genai.ClientConfig{
		APIKey:      provider.GetAccessToken(),
		HTTPOptions: httpOptions,
		HTTPClient:  httpClient,
	}

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
	if c.httpClient != nil && c.httpClient != http.DefaultClient {
		c.httpClient.CloseIdleConnections()
	}
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
