package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guard
var _ OpenAIClientInterface = (*XAIClient)(nil)

// XAIClient wraps OpenAIClient with xAI (Grok) OAuth specific behavior.
// The upstream is an OpenAI-compatible chat completions API, so the only
// thing this wrapper adds over the base client is the Grok CLI
// impersonation headers required by cli-chat-proxy.grok.com — see
// xaiRoundTripper.
type XAIClient struct {
	*OpenAIClient
}

// NewXAIClient creates a new xAI (Grok) client wrapper.
func NewXAIClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*XAIClient, error) {
	if provider.OAuthDetail == nil {
		return nil, fmt.Errorf("xai client requires OAuth configuration")
	}
	if provider.OAuthDetail.GetIssuer() != ai.IssuerXAI {
		return nil, fmt.Errorf("xai client can only work for xAI (Grok) providers")
	}

	transport := newXAIRoundTripper(createSessionBoundTransport(provider, sessionID))
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

	return &XAIClient{OpenAIClient: base}, nil
}

// Client returns the underlying OpenAI SDK client.
func (c *XAIClient) Client() *openai.Client {
	return c.OpenAIClient.Client()
}

// GetProvider returns the provider configuration.
func (c *XAIClient) GetProvider() *typ.Provider {
	return c.OpenAIClient.GetProvider()
}

// Close closes the client and releases resources.
func (c *XAIClient) Close() error {
	return c.OpenAIClient.Close()
}

// APIStyle returns the API style (OpenAI) for this client.
func (c *XAIClient) APIStyle() protocol.APIStyle {
	return c.OpenAIClient.APIStyle()
}

// ListModels returns the list of available models.
func (c *XAIClient) ListModels(ctx context.Context) (*ModelListResult, error) {
	return c.OpenAIClient.ListModels(ctx)
}

// ResponsesNew is not supported by xAI's Grok CLI proxy.
func (c *XAIClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return nil, fmt.Errorf("xai (grok): /responses endpoint is not supported")
}

// ResponsesNewStreaming is not supported by xAI's Grok CLI proxy.
func (c *XAIClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return nil
}
