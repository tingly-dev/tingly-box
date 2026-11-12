package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/v2/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/v2/packages/ssestream"
	"github.com/tingly-dev/tingly-box/internal/constant"

	"github.com/tingly-dev/tingly-box/ai"
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
	// wire selects the API wire version (see AnthropicWire): WireV2, the
	// keeps the SDK's default version header, authenticates with
	// Authorization: Bearer, skips the classic ?beta=true routing query, and
	// strips temperature/top_p/top_k sampling extras the v2 wire rejects.
	// Classic (2023-06-01) providers keep every old behavior unchanged.
	wire AnthropicWire
}

// NewAnthropicClient creates a new Anthropic client wrapper over the standard
// transport chain for its auth type.
func NewAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID, extraOptions ...anthropicOption.RequestOption) (*AnthropicClient, error) {
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
	return newAnthropicClientWithTransport(provider, provider.APIBase, transport, WireV1, extraOptions...)
}

// NewAnthropicClientV2 creates the new-API-version (2026-10-21) variant of the
// Anthropic client wrapper. It shares the transport chain and lifecycle with
// the classic client but differs on the wire:
//   - no pinned anthropic-version header (SDK default 2026-10-21);
//   - Authorization: Bearer via the SDK's WithAPIKey instead of the classic
//     x-api-key header;
//   - no per-call ?beta=true query — the v2 API has no such routing.
//
// Intended for the official Anthropic API; third-party providers keep the
// classic client.
func NewAnthropicClientV2(provider *typ.Provider, model string, sessionID typ.SessionID, extraOptions ...anthropicOption.RequestOption) (*AnthropicClient, error) {
	// Same transport assembly as the classic client: the only differences
	// between the two variants are header/query/auth wire details, not
	// proxying, UA resolution or session binding.
	var transport http.RoundTripper
	if provider.AuthType == typ.AuthTypeOAuth {
		transport = createSessionBoundTransport(provider, sessionID)
	} else {
		transport = anthropicTransport(provider, model, sessionID)
	}
	return newAnthropicClientWithTransport(provider, provider.APIBase, transport, WireV2, extraOptions...)
}

// anthropicBaseURL normalizes a provider APIBase for the Anthropic SDK, which
// appends /v1 itself.
func anthropicBaseURL(apiBase string) string {
	return strings.TrimSuffix(strings.TrimRight(apiBase, "/"), "/v1")
}

// newAnthropicClientWithTransport builds the SDK client for baseURL on an
// already assembled transport. Constructors that need a non-pooled base or a
// rewritten base URL (e.g. vmodel) call this directly. wire selects the API
// wire variant (see AnthropicWire; WireV2 is the new-API-version wire).
func newAnthropicClientWithTransport(provider *typ.Provider, baseURL string, transport http.RoundTripper, wire AnthropicWire, extraOptions ...anthropicOption.RequestOption) (*AnthropicClient, error) {
	options := []anthropicOption.RequestOption{
		// Provider credentials come only from the provider record; the gateway's
		// own ANTHROPIC_* environment must never be sent upstream.
		anthropicOption.WithoutEnvironmentDefaults(),
		anthropicOption.WithBaseURL(anthropicBaseURL(baseURL)),
		anthropicOption.WithMaxRetries(0), // Disable automatic retries for 429 errors in test environments
	}
	// The v2 SDK defaults to the 2026-10-21 API version, which drops
	// temperature/top_p/top_k; the classic ecosystem this gateway serves is on
	// the old version, so the classic client pins the header. The V2 wire
	// variant keeps the SDK's 2026-10-21 default instead.
	if wire == WireV1 {
		options = append(options,
			anthropicOption.WithHeader("anthropic-version", anthropicVersion))
	}
	// Multi-field credential providers (Bedrock / Vertex) carry no bearer token;
	// their cloud adapter option (appended last) installs its own auth. Setting
	// an empty key header would only plant a stray x-api-key header.
	if !provider.IsMultiFieldCredential() {
		if wire == WireV2 {
			// v2's WithAPIKey sends Authorization: Bearer — the new-API-version
			// auth shape. The classic branch below keeps x-api-key for the
			// 2023-06-01 ecosystem.
			options = append(options, anthropicOption.WithAPIKey(provider.GetAccessToken()))
		} else {
			// v2's WithAPIKey sends Authorization: Bearer; the classic ecosystem
			// this client serves (2023-06-01) expects x-api-key.
			options = append(options, anthropicOption.WithHeader("x-api-key", provider.GetAccessToken()))
		}
	}

	// context-1m is NOT injected here: it's a request-body/header concern
	// applied per-call in the Beta/Messages methods (withContext1MBeta /
	// context1MHeaderOpts) from the resolved rule flags (typ.GetRuleFlags), so
	// it reaches both this generic client and ClaudeClient without a dedicated
	// transport.
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
		wire:       wire,
	}, nil
}

// anthropicTransport builds the transport chain generic Anthropic providers
// use: pooled session-bound base (provider proxy_url honored, env proxy not
// inherited), rule-flag layer, advisor loopback stamp, logging. Shared with the
// Vertex path, which must rebuild this chain under its OAuth transport (see
// vertexAnthropicOptions).
func anthropicTransport(provider *typ.Provider, model string, sessionID typ.SessionID) http.RoundTripper {
	base := GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
	return providerTransportChain(base, provider)
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
	c.stripSamplingExtrasV2(req)
	return c.client.Messages.New(ctx, *req, context1MHeaderOpts(ctx)...)
}

// MessagesNewStreaming creates a new streaming message request
func (c *AnthropicClient) MessagesNewStreaming(ctx context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	c.stripSamplingExtrasV2(req)
	return c.client.Messages.NewStreaming(ctx, *req, context1MHeaderOpts(ctx)...)
}

// MessagesCountTokens counts tokens for a message request
func (c *AnthropicClient) MessagesCountTokens(ctx context.Context, req *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	StripCountTokensExtras(req)
	return c.client.Messages.CountTokens(ctx, *req, context1MHeaderOpts(ctx)...)
}

// betaQuery keeps the classic beta egress wire shape. The v1 SDK carried
// ?beta=true on beta endpoints (the v2 SDK does not); Anthropic's official
// API ignores it, but some compatible providers route on it.
func betaQuery() anthropicOption.RequestOption {
	return anthropicOption.WithQuery("beta", "true")
}

func (c *AnthropicClient) BetaMessagesCountTokens(ctx context.Context, req *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	StripCountTokensExtras(req)
	betaOpts := []anthropicOption.RequestOption{betaQuery()}
	if c.wire == WireV2 {
		// V2 API has no ?beta=true routing; drop the classic query.
		betaOpts = nil
	}
	return c.client.Beta.Messages.CountTokens(ctx, *req, betaOpts...)
}

// BetaMessagesNew creates a new beta message request
func (c *AnthropicClient) BetaMessagesNew(ctx context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	betaOpts := []anthropicOption.RequestOption{betaQuery()}
	if c.wire == WireV2 {
		// V2 API has no ?beta=true routing; drop the classic query.
		betaOpts = nil
	}
	c.stripSamplingExtrasV2(req)
	return c.client.Beta.Messages.New(ctx, *req, betaOpts...)
}

// BetaMessagesNewStreaming creates a new beta streaming message request
func (c *AnthropicClient) BetaMessagesNewStreaming(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	req.Betas = withContext1MBeta(ctx, req.Betas)
	betaOpts := []anthropicOption.RequestOption{betaQuery()}
	if c.wire == WireV2 {
		// V2 API has no ?beta=true routing; drop the classic query.
		betaOpts = nil
	}
	c.stripSamplingExtrasV2(req)
	return c.client.Beta.Messages.NewStreaming(ctx, *req, betaOpts...)
}

// stripSamplingExtrasV2 applies the WireV2 message egress filter: the
// 2026-10-21 wire rejects temperature / top_p / top_k, which the decode
// preservation patch routes through extras (see
// internal/protocol/request/sampling_extra.go). The classic 2023-06-01 wire
// still carries them — and every other extra — untouched: ingress decoding is
// lossless, so what each wire accepts is decided here, per wire, explicitly.
// No-op on the classic client.
func (c *AnthropicClient) stripSamplingExtrasV2(req interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}) {
	if c.wire == WireV1 {
		return
	}
	stripSamplingExtras(req)
}

// countTokensExtras are the extras the count_tokens endpoints do not accept.
// Ingress keeps them (decoding is lossless); they are stripped here so the
// count_tokens wire stays minimal regardless of the messages body it came
// from.
var countTokensExtras = map[string]bool{
	"stream":      true,
	"max_tokens":  true,
	"temperature": true,
	"top_p":       true,
	"top_k":       true,
	"user":        true,
}

// StripCountTokensExtras removes messages-only fields from a count_tokens
// request's extras. Runs on both wires: these fields never had a
// count_tokens form.
func StripCountTokensExtras(req interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}) {
	extras := req.ExtraFields()
	if len(extras) == 0 {
		return
	}
	stripped := make(map[string]any, len(extras))
	for k, v := range extras {
		if countTokensExtras[k] {
			continue
		}
		stripped[k] = v
	}
	req.SetExtraFields(stripped)
}

// stripSamplingExtras is the unconditional form of stripSamplingExtrasV2, for
// callers that already know they are on the V2 wire (e.g. ClaudeClientV2's
// Guard entry points). See stripSamplingExtrasV2 for why this exists.
func stripSamplingExtras(req interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}) {
	extras := req.ExtraFields()
	if len(extras) == 0 {
		return
	}
	stripped := make(map[string]any, len(extras))
	for k, v := range extras {
		switch k {
		case "temperature", "top_p", "top_k":
			continue
		}
		stripped[k] = v
	}
	req.SetExtraFields(stripped)
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
