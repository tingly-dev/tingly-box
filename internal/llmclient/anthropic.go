package llmclient

import (
	"context"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"

	"tingly-box/internal/llmclient/httpclient"
	"tingly-box/internal/record"
	"tingly-box/internal/typ"
	"tingly-box/pkg/oauth"
)

// AnthropicClient wraps the Anthropic SDK client
type AnthropicClient struct {
	client     anthropic.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *record.Sink
}

// defaultNewAnthropicClient creates a new Anthropic client wrapper
func defaultNewAnthropicClient(provider *typ.Provider) (*AnthropicClient, error) {
	// Handle API base URL - Anthropic SDK expects base without /v1
	apiBase := strings.TrimRight(provider.APIBase, "/")
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimSuffix(apiBase, "/v1")
	}

	options := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.GetAccessToken()),
		anthropicOption.WithBaseURL(apiBase),
	}

	// Create base HTTP client
	var httpClient *http.Client
	// Add proxy and/or custom headers if configured
	if provider.ProxyURL != "" || provider.AuthType == typ.AuthTypeOAuth {
		var providerType oauth.ProviderType
		if provider.OAuthDetail != nil {
			providerType = oauth.ProviderType(provider.OAuthDetail.ProviderType)
		}
		httpClient = httpclient.CreateHTTPClientForProvider(providerType, provider.ProxyURL, provider.AuthType == typ.AuthTypeOAuth)

		if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
			logrus.Infof("Using custom headers/params for OAuth provider type: %s", provider.OAuthDetail.ProviderType)
		}
		if provider.ProxyURL != "" {
			logrus.Infof("Using proxy for Anthropic client: %s", provider.ProxyURL)
		}
	} else {
		httpClient = http.DefaultClient
	}

	if provider.ProxyURL != "" || provider.AuthType == typ.AuthTypeOAuth {
		options = append(options, anthropicOption.WithHTTPClient(httpClient))
	}

	anthropicClient := anthropic.NewClient(options...)

	return &AnthropicClient{
		client:     anthropicClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *AnthropicClient) ProviderType() ProviderType {
	return ProviderTypeAnthropic
}

// Close closes any resources held by the client
func (c *AnthropicClient) Close() error {
	// Anthropic client doesn't need explicit closing
	return nil
}

// Client returns the underlying Anthropic SDK client
func (c *AnthropicClient) Client() *anthropic.Client {
	return &c.client
}

// MessagesNew creates a new message request
func (c *AnthropicClient) MessagesNew(ctx context.Context, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.client.Messages.New(ctx, req)
}

// MessagesNewStreaming creates a new streaming message request
func (c *AnthropicClient) MessagesNewStreaming(ctx context.Context, req anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return c.client.Messages.NewStreaming(ctx, req)
}

// MessagesCountTokens counts tokens for a message request
func (c *AnthropicClient) MessagesCountTokens(ctx context.Context, req anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return c.client.Messages.CountTokens(ctx, req)
}

func (c *AnthropicClient) BetaMessagesCountTokens(ctx context.Context, req anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	return c.client.Beta.Messages.CountTokens(ctx, req)
}

// BetaMessagesNew creates a new beta message request
func (c *AnthropicClient) BetaMessagesNew(ctx context.Context, req anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return c.client.Beta.Messages.New(ctx, req)
}

// BetaMessagesNewStreaming creates a new beta streaming message request
func (c *AnthropicClient) BetaMessagesNewStreaming(ctx context.Context, req anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return c.client.Beta.Messages.NewStreaming(ctx, req)
}

// SetRecordSink sets the record sink for the client
func (c *AnthropicClient) SetRecordSink(sink *record.Sink) {
	c.recordSink = sink
	if sink != nil && sink.IsEnabled() {
		c.applyRecordMode()
	}
}

// applyRecordMode wraps the HTTP client with a record round tripper
func (c *AnthropicClient) applyRecordMode() {
	if c.recordSink == nil {
		return
	}
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider.Name, "")
}

// GetProvider returns the provider for this client
func (c *AnthropicClient) GetProvider() *typ.Provider {
	return c.provider
}
