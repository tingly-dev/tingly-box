package client

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// isAntigravityProvider checks if the provider is using Antigravity's API
func isAntigravityProvider(provider *typ.Provider) bool {
	return provider.OAuthDetail != nil &&
		provider.OAuthDetail.ProviderType == string(oauth.ProviderAntigravity)
}

// getAntigravityExtraFields retrieves Antigravity-specific fields from OAuth extra fields
func getAntigravityExtraFields(provider *typ.Provider) (project, model string) {
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if v, ok := provider.OAuthDetail.ExtraFields["project"].(string); ok {
			project = v
		}
		if v, ok := provider.OAuthDetail.ExtraFields["model"].(string); ok {
			model = v
		}
	}
	return
}

// newAntigravityRequestWrapper creates a request wrapper for Antigravity's custom API format
//
// Antigravity's API requires wrapping the standard genai request in an outer structure:
//
//	{
//	  "project": "project-id",
//	  "requestId": "uuid",
//	  "request": { /* standard genai request */ },
//	  "model": "gemini-2.5-flash",
//	  "userAgent": "antigravity",
//	  "requestType": "agent"
//	}
func newAntigravityRequestWrapper(project, model string) genai.ExtrasRequestProvider {
	return func(originalBody map[string]any) map[string]any {
		// Remove model from original body as it will be at the top level
		// The genai SDK puts "model" in the body, but Antigravity expects it at the wrapper level
		cleanBody := make(map[string]any)
		for k, v := range originalBody {
			if k != "model" {
				cleanBody[k] = v
			}
		}

		// Wrap the original genai request
		wrapped := map[string]any{
			"project":     project,
			"requestId":   fmt.Sprintf("agent-%s", uuid.New().String()),
			"request":     cleanBody,
			"model":       model,
			"userAgent":   "antigravity",
			"requestType": "agent",
		}

		return wrapped
	}
}

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
	httpOptions := genai.HTTPOptions{
		BaseURL: provider.APIBase,
	}

	// Apply Antigravity-specific configuration
	if isAntigravityProvider(provider) {
		project, model := getAntigravityExtraFields(provider)

		// Apply request wrapper to transform request body
		if project != "" && model != "" {
			httpOptions.ExtrasRequestProvider = newAntigravityRequestWrapper(project, model)
			logrus.Infof("Applied Antigravity request wrapper for project=%s, model=%s", project, model)
		} else {
			logrus.Warnf("Antigravity provider missing project or model in ExtraFields")
		}
	}

	config := &genai.ClientConfig{
		APIKey:      provider.GetAccessToken(),
		HTTPOptions: httpOptions,
		HTTPClient:  httpClient,
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
