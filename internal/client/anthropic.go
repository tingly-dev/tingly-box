package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeSystemHeader is a special system message for Claude Code OAuth subscriptions
const ClaudeCodeSystemHeader = "You are Claude Code, Anthropic's official CLI for Claude."
const ClaudeCodeSystemBody = "You are a file search specialist for Claude Code, Anthropic's official CLI for Claude. You excel at thoroughly navigating and exploring codebases.\n\n"

// AnthropicClientInterface defines the contract for Anthropic-compatible clients.
// Both AnthropicClient and ClaudeClient (for Claude Code OAuth) implement this interface.
type AnthropicClientInterface interface {
	// Core API methods
	MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error)
	MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion]
	BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error)
	BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error)
	BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error)

	// Utility methods
	ListModels(ctx context.Context) ([]string, error)
	Close() error
	GetProvider() *typ.Provider
	APIStyle() protocol.APIStyle
	SetRecordSink(sink *obs.Sink)
	Client() *anthropic.Client
}

// AnthropicClient wraps the Anthropic SDK client
type AnthropicClient struct {
	client     anthropic.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// NewAnthropicClient creates a new Anthropic client wrapper
func NewAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID, extraOptions ...anthropicOption.RequestOption) (*AnthropicClient, error) {
	// Handle API base URL - Anthropic SDK expects base without /v1
	apiBase := strings.TrimRight(provider.APIBase, "/")
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimSuffix(apiBase, "/v1")
	}

	options := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.GetAccessToken()),
		anthropicOption.WithBaseURL(apiBase),
		anthropicOption.WithMaxRetries(0), // Disable automatic retries for 429 errors in test environments
	}

	// Create HTTP client with session-bound transport
	var transport http.RoundTripper
	if provider.AuthType == typ.AuthTypeOAuth {
		if provider.OAuthDetail != nil && provider.OAuthDetail.Issuer == ai.IssuerClaudeCode {
			// context1m sits inside claudeRoundTripper so the 1M beta flag is
			// appended after the fingerprint-managed header merge (context-1m
			// is on the fingerprint-safe allowlist, so this is equivalent).
			transport = &claudeRoundTripper{
				RoundTripper: &context1mBetaTransport{base: createSessionBoundTransport(provider, sessionID)},
			}
			logrus.Infof("Using session-bound transport for OAuth issuer: %s, session: %s",
				provider.OAuthDetail.GetIssuer(), sessionID.Value)
		} else {
			// OAuth provider with an issuer other than ClaudeCode (or missing OAuthDetail).
			// Use a session-bound transport so proxy_url is respected and env proxy is
			// not inherited — same guarantee as the non-OAuth path below.
			transport = &context1mBetaTransport{base: createSessionBoundTransport(provider, sessionID)}
		}
	} else {
		// Generic non-OAuth Anthropic provider. Apply the same User-Agent
		// layering as the generic OpenAI client (rule > provider): rule-UA
		// wraps innermost so it overwrites the header last, provider-UA sits
		// outside. OAuth issuers above keep their dedicated transport chain
		// unchanged because vendor-specific round-trippers manage UA themselves.
		//
		// Use the transport pool instead of http.DefaultTransport so that env
		// proxy variables (HTTP_PROXY / HTTPS_PROXY) are not inherited when no
		// proxy is explicitly configured for the provider.
		base := GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
		transport = &context1mBetaTransport{base: base}
		transport = &customUserAgentTransport{base: transport}
		transport = wrapWithUserAgent(transport, provider)
		transport = wrapWithLogging(transport, provider)
	}

	httpClient := &http.Client{
		Transport: transport,
	}
	options = append(options, anthropicOption.WithHTTPClient(httpClient))

	// MENTION: extra will be applied at last to confirm override
	options = append(options, extraOptions...)

	// MENTION: must set timeout, otherwise nonstream and stream may work badly
	timeout := time.Duration(provider.Timeout) * time.Second
	if provider.Timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}
	options = append(options, anthropicOption.WithRequestTimeout(timeout))

	anthropicClient := anthropic.NewClient(options...)

	return &AnthropicClient{
		client:     anthropicClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *AnthropicClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleAnthropic
}

// Close closes any resources held by the client
func (c *AnthropicClient) Close() error {
	if c.httpClient != nil && c.httpClient != http.DefaultClient {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// Client returns the underlying Anthropic SDK client
func (c *AnthropicClient) Client() *anthropic.Client {
	return &c.client
}

// HttpClient returns the underlying HTTP client for passthrough/proxy operations
func (c *AnthropicClient) HttpClient() *http.Client {
	return c.httpClient
}

// MessagesNew creates a new message request
func (c *AnthropicClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.client.Messages.New(ctx, *req)
}

// MessagesNewStreaming creates a new streaming message request
func (c *AnthropicClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return c.client.Messages.NewStreaming(ctx, *req)
}

// MessagesCountTokens counts tokens for a message request
func (c *AnthropicClient) MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return c.client.Messages.CountTokens(ctx, *req)
}

func (c *AnthropicClient) BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	return c.client.Beta.Messages.CountTokens(ctx, *req)
}

// BetaMessagesNew creates a new beta message request
func (c *AnthropicClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return c.client.Beta.Messages.New(ctx, *req)
}

// BetaMessagesNewStreaming creates a new beta streaming message request
func (c *AnthropicClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return c.client.Beta.Messages.NewStreaming(ctx, *req)
}

// SetRecordSink sets the record sink for the client
func (c *AnthropicClient) SetRecordSink(sink *obs.Sink) {
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
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider)
}

// GetProvider returns the provider for this client
func (c *AnthropicClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the Anthropic API
func (c *AnthropicClient) ListModels(ctx context.Context) ([]string, error) {
	models, err := c.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, err
	}

	var result []string
	for _, model := range models.Data {
		result = append(result, model.ID)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no models found in provider response")
	}

	return result, nil
}
