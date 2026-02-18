package client

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"time"

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
	httpOptions := genai.HTTPOptions{
		BaseURL: provider.APIBase,
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

// ProbeChatEndpoint tests the chat endpoint with a minimal request
func (c *GoogleClient) ProbeChatEndpoint(ctx context.Context, model string) ProbeResult {
	startTime := time.Now()

	// Create minimal content for probe
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: "hi"},
			},
		},
	}

	// Configure generation with minimal tokens
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: 1000,
	}

	// Make request
	resp, err := c.client.Models.GenerateContent(ctx, model, contents, config)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
		}
	}

	// Extract response data
	responseContent := ""
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0

	if resp != nil && len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					responseContent += part.Text
				}
			}
		}
		if resp.UsageMetadata != nil {
			promptTokens = int(resp.UsageMetadata.PromptTokenCount)
			completionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			totalTokens = int(resp.UsageMetadata.TotalTokenCount)
		}
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return ProbeResult{
		Success:          true,
		Message:          "Chat endpoint is accessible",
		Content:          responseContent,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

// ProbeModelsEndpoint tests the models list endpoint
func (c *GoogleClient) ProbeModelsEndpoint(ctx context.Context) ProbeResult {
	// Google genai SDK doesn't provide a models list endpoint
	// Return an error result indicating this is not supported
	return ProbeResult{
		Success:      false,
		ErrorMessage: "Google genai SDK does not support listing models via API",
		LatencyMs:    0,
	}
}

// ProbeOptionsEndpoint tests basic connectivity with an OPTIONS request
func (c *GoogleClient) ProbeOptionsEndpoint(ctx context.Context) ProbeResult {
	startTime := time.Now()

	// Use the API base URL for OPTIONS request
	optionsURL := c.provider.APIBase
	if !strings.HasSuffix(optionsURL, "/") {
		optionsURL += "/"
	}

	req, err := http.NewRequestWithContext(ctx, "OPTIONS", optionsURL, nil)
	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create OPTIONS request: %v", err),
		}
	}

	// Set authentication header
	req.Header.Set("x-goog-api-key", c.provider.GetAccessToken())

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("OPTIONS request failed: %v", err),
			LatencyMs:    latencyMs,
		}
	}
	defer resp.Body.Close()

	// Consider any 2xx status as success for OPTIONS
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ProbeResult{
			Success:   true,
			Message:   "OPTIONS request successful",
			LatencyMs: latencyMs,
		}
	}

	return ProbeResult{
		Success:      false,
		ErrorMessage: fmt.Sprintf("OPTIONS request failed with status: %d", resp.StatusCode),
		LatencyMs:    latencyMs,
	}
}
