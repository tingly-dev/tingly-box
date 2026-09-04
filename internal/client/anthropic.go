package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"

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
	ListModels(ctx context.Context) (*ModelListResult, error)
	Close() error
	GetProvider() *typ.Provider
	APIStyle() protocol.APIStyle
	Client() *anthropic.Client
}

// AnthropicClient wraps the Anthropic SDK client
type AnthropicClient struct {
	client     anthropic.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic client wrapper
func NewAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID, extraOptions ...anthropicOption.RequestOption) (*AnthropicClient, error) {
	// Handle API base URL - Anthropic SDK expects base without /v1
	apiBase := provider.APIBase
	if virtualserver.IsAPIBase(apiBase) {
		apiBase = virtualserver.HTTPBase(apiBase, provider.APIStyle)
	}
	apiBase = strings.TrimRight(apiBase, "/")
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimSuffix(apiBase, "/v1")
	}

	options := []anthropicOption.RequestOption{
		anthropicOption.WithBaseURL(apiBase),
		anthropicOption.WithMaxRetries(0), // Disable automatic retries for 429 errors in test environments
	}
	// Multi-field credential providers (Bedrock / Vertex) carry no bearer token;
	// their cloud adapter option (appended last) installs its own auth. Setting
	// an empty WithAPIKey would only plant a stray x-api-key header.
	if !provider.IsMultiFieldCredential() {
		options = append(options, anthropicOption.WithAPIKey(provider.GetAccessToken()))
	}

	// Create HTTP client with session-bound transport.
	//
	// context-1m is NOT injected here: it's a request-body/header concern
	// applied per-call in the Beta/Messages methods (withContext1MBeta /
	// context1MHeaderOpts) from the resolved rule flags (typ.GetRuleFlags), so
	// it reaches both this generic client and ClaudeClient without a dedicated
	// transport.
	//
	// Note: Claude Code OAuth providers never reach this constructor —
	// ClientPool.GetAnthropicClient routes them to NewClaudeClient. So the
	// transport here only ever serves generic Anthropic providers.
	var transport http.RoundTripper
	if provider.AuthType == typ.AuthTypeOAuth {
		// OAuth provider that is not Claude Code (the Claude Code issuer is
		// dispatched to NewClaudeClient upstream). Use a session-bound
		// transport so proxy_url is respected and env proxy is not inherited —
		// same guarantee as the non-OAuth path below.
		transport = createSessionBoundTransport(provider, sessionID)
	} else {
		// Generic non-OAuth Anthropic provider. The single ruleFlagTransport
		// resolves the same fixed UA precedence as the generic OpenAI client
		// (rule/scenario custom_user_agent > inbound client UA > SDK default)
		// and applies extra_headers on api_key providers. There is deliberately
		// no provider-level UA layer (see .design/user-agent.md). OAuth issuers
		// above keep their dedicated transport chain unchanged because
		// vendor-specific round-trippers pin the handshake UA themselves — that
		// pin is decisive and must not be overwritten by the rule or client UA.
		//
		// Use the transport pool instead of http.DefaultTransport so that env
		// proxy variables (HTTP_PROXY / HTTPS_PROXY) are not inherited when no
		// proxy is explicitly configured for the provider.
		transport = anthropicTransport(provider, model, sessionID)
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

// anthropicTransport builds the transport chain generic Anthropic providers
// use: pooled session-bound base (provider proxy_url honored, env proxy not
// inherited), rule-flag layer, advisor loopback stamp, logging. Shared with the
// Vertex path, which must rebuild this chain under its OAuth transport (see
// vertexAnthropicOptions).
func anthropicTransport(provider *typ.Provider, model string, sessionID typ.SessionID) http.RoundTripper {
	var base http.RoundTripper = GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
	if virtualserver.IsAPIBase(provider.APIBase) {
		// vmodel provider: same chain, but dial the in-process virtualserver.
		base = virtualserver.Transport()
	}
	return wrapWithLogging(wrapWithAdvisorLoopback(wrapWithRuleFlags(base, provider, true)), provider)
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

// withContext1MBeta appends Anthropic's context-1m beta to a beta request's
// Betas when the request's resolved rule flags carry Context1M (set by the
// gateway from the context_1m rule / [1m] alias). Deduped, so a client that
// already sent it is left untouched. The SDK serializes Betas into the
// anthropic-beta header via WithHeaderAdd *after* the client's base options, so
// this also reaches ClaudeClient — its static anthropic-beta header gets the
// value appended. Consumed at the SDK layer, not in a transport — see the
// ruleFlagTransport doc for why.
func withContext1MBeta(ctx context.Context, betas []anthropic.AnthropicBeta) []anthropic.AnthropicBeta {
	if !typ.GetRuleFlags(ctx).Context1M {
		return betas
	}
	if slices.Contains(betas, anthropic.AnthropicBetaContext1m2025_08_07) {
		return betas
	}
	return append(betas, anthropic.AnthropicBetaContext1m2025_08_07)
}

// context1MHeaderOpts carries the context-1m beta via the anthropic-beta header
// for the non-beta Messages API, whose params have no Betas field. No-op
// without the 1M flag.
func context1MHeaderOpts(ctx context.Context) []anthropicOption.RequestOption {
	if !typ.GetRuleFlags(ctx).Context1M {
		return nil
	}
	return []anthropicOption.RequestOption{anthropicOption.WithHeaderAdd("anthropic-beta", AnthropicContext1m)}
}

// MessagesNew creates a new message request
func (c *AnthropicClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.client.Messages.New(ctx, *req, context1MHeaderOpts(ctx)...)
}

// MessagesNewStreaming creates a new streaming message request
func (c *AnthropicClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return c.client.Messages.NewStreaming(ctx, *req, context1MHeaderOpts(ctx)...)
}

// MessagesCountTokens counts tokens for a message request
func (c *AnthropicClient) MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return c.client.Messages.CountTokens(ctx, *req, context1MHeaderOpts(ctx)...)
}

func (c *AnthropicClient) BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	return c.client.Beta.Messages.CountTokens(ctx, *req)
}

// BetaMessagesNew creates a new beta message request
func (c *AnthropicClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	return c.client.Beta.Messages.New(ctx, *req)
}

// BetaMessagesNewStreaming creates a new beta streaming message request
func (c *AnthropicClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	return c.client.Beta.Messages.NewStreaming(ctx, *req)
}

// GetProvider returns the provider for this client
func (c *AnthropicClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the Anthropic API
func (c *AnthropicClient) ListModels(ctx context.Context) (*ModelListResult, error) {
	// Bedrock / Vertex expose only the messages endpoint, not the Anthropic
	// /v1/models list; use the template model list instead.
	if c.provider.IsMultiFieldCredential() {
		return nil, &ErrModelsEndpointNotSupported{
			Provider: c.provider.Name,
			Reason:   "cloud-credential providers use template model lists",
		}
	}
	res, err := c.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range res.Data {
		models = append(models, model.ID)
	}

	if len(models) == 0 {
		return &ModelListResult{Raw: res}, fmt.Errorf("no models found in provider response")
	}

	return &ModelListResult{Models: models, Raw: res}, nil
}
