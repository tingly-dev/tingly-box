package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// userAgentRoundTripper forces the outbound User-Agent header to a fixed
// value. It is layered as the INNERMOST wrapper (closer to the network) so it
// takes precedence over any provider-specific UA set by outer round trippers
// (claudeRoundTripper, antigravityRoundTripper, codexRoundTripper, etc.) that
// rewrite headers before delegating downstream.
type userAgentRoundTripper struct {
	http.RoundTripper
	userAgent string
}

func (t *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	return t.RoundTripper.RoundTrip(req)
}

// wrapWithUserAgent wraps a transport with a User-Agent override when the
// provider has a custom UA configured. Returns the original transport unchanged
// when no override is set.
//
// Design note: provider.UserAgent is treated as a deliberate debug knob — when
// non-empty it intentionally overrides even vendor-pinned UAs (claude-cli,
// GeminiCLI, codex, …) because we don't want to hide the configured value
// behind silent precedence rules. Operators who set this should know what
// they're doing; the catch lives in ai/provider.go's UserAgent doc comment.
// Rule-level custom_user_agent layers innermost so it still wins over both.
func wrapWithUserAgent(inner http.RoundTripper, provider *typ.Provider) http.RoundTripper {
	if provider == nil || provider.UserAgent == "" {
		return inner
	}
	return &userAgentRoundTripper{RoundTripper: inner, userAgent: provider.UserAgent}
}

// CreateHTTPClientWithProxy creates an HTTP client with proxy support.
// Supports http(s) and socks5 proxy URLs; falls back to http.DefaultClient
// for any parse/scheme failure (logged).
func CreateHTTPClientWithProxy(proxyURL string) *http.Client {
	if proxyURL == "" {
		return http.DefaultClient
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Errorf("Failed to parse proxy URL %s: %v, using default client", proxyURL, err)
		return http.DefaultClient
	}

	transport := &http.Transport{}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
		if err != nil {
			logrus.Errorf("Failed to create SOCKS5 proxy dialer: %v, using default client", err)
			return http.DefaultClient
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			return http.DefaultClient
		}
	default:
		logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
		return http.DefaultClient
	}

	return &http.Client{
		Transport: transport,
	}
}

// probeHeadersKey is a context key for probe-only HTTP headers.
// Headers stored here are injected by probeHeaderRoundTripper into every
// outgoing request that carries this context. Only set by probe code;
// production traffic never touches this key.
type probeHeadersKey struct{}

// WithProbeHeaders stores headers in ctx so that clients using
// probeHeaderRoundTripper inject them into each SDK HTTP call.
func WithProbeHeaders(ctx context.Context, headers map[string]string) context.Context {
	return context.WithValue(ctx, probeHeadersKey{}, headers)
}

// probeHeaderRoundTripper sits at the outermost layer of a transport chain and
// adds probe-specific headers (stored in the request context by probe code)
// to each outbound request. It is a no-op for non-probe requests.
type probeHeaderRoundTripper struct {
	inner http.RoundTripper
}

func (t *probeHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if headers, ok := req.Context().Value(probeHeadersKey{}).(map[string]string); ok && len(headers) > 0 {
		req = req.Clone(req.Context())
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	return t.inner.RoundTrip(req)
}

// wrapWithProbeHeaders wraps a transport so probe headers stored in request
// contexts are injected into outgoing requests. No-op on non-probe requests.
func wrapWithProbeHeaders(inner http.RoundTripper) http.RoundTripper {
	return &probeHeaderRoundTripper{inner: inner}
}

// GetProbeHeaders returns probe headers stored in ctx by WithProbeHeaders.
// Returns nil, false when no headers are present.
func GetProbeHeaders(ctx context.Context) (map[string]string, bool) {
	h, ok := ctx.Value(probeHeadersKey{}).(map[string]string)
	return h, ok && len(h) > 0
}

// applyTransportWrap layers wrap() onto the HTTP transport of a probe client.
// Accepts *OpenAIClient or *AnthropicClient; no-op for any other type.
func applyTransportWrap(c interface{}, wrap func(http.RoundTripper) http.RoundTripper) {
	switch tc := c.(type) {
	case *OpenAIClient:
		tc.HttpClient.Transport = wrap(tc.HttpClient.Transport)
	case *AnthropicClient:
		tc.httpClient.Transport = wrap(tc.httpClient.Transport)
	}
}

// ApplyProbeHeadersToClient wraps the HTTP transport of a client so that
// probe headers from the context are forwarded on every outgoing request.
// Call this only on probe clients — not on production client instances.
func ApplyProbeHeadersToClient(c interface{}) {
	applyTransportWrap(c, wrapWithProbeHeaders)
}

// RoutingCapture holds routing-decision headers captured from a TB-loopback
// probe response. Fields mirror the X-Tingly-Selected-* / X-Tingly-Routing-Source
// headers set by SimpleSelector when X-Tingly-Debug-Routing: 1 is present.
type RoutingCapture struct {
	Mu                   sync.Mutex
	SelectedProvider     string
	SelectedProviderUUID string
	SelectedModel        string
	RoutingSource        string
	MatchedSmartRule     string // raw header value; "-1" or index string

	// Execution-level facts (set at dispatch, after endpoint resolution).
	UpstreamAPI     string // e.g. "openai_chat", "openai_responses", "anthropic_v1"
	UpstreamURL     string // real upstream endpoint TB forwarded to
	MatchedRule     string // matched rule UUID (empty for synthetic provider probes)
	MatchedRuleDesc string // percent-encoded; decoded by the probe layer
	AppliedFlags    string // compact "endpoint=responses, thinking=high"
}

// captureRoutingRoundTripper reads X-Tingly-* routing headers from every
// response and records them in the shared RoutingCapture. It is layered
// outermost on probe clients so it sees the loopback response before the SDK.
type captureRoutingRoundTripper struct {
	inner   http.RoundTripper
	capture *RoutingCapture
}

func (t *captureRoutingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err == nil && resp != nil {
		t.capture.Mu.Lock()
		t.capture.SelectedProvider = resp.Header.Get("X-Tingly-Selected-Provider")
		t.capture.SelectedProviderUUID = resp.Header.Get("X-Tingly-Selected-Provider-UUID")
		t.capture.SelectedModel = resp.Header.Get("X-Tingly-Selected-Model")
		t.capture.RoutingSource = resp.Header.Get("X-Tingly-Routing-Source")
		t.capture.MatchedSmartRule = resp.Header.Get("X-Tingly-Matched-Smart-Rule")
		t.capture.UpstreamAPI = resp.Header.Get("X-Tingly-Upstream-API")
		t.capture.UpstreamURL = resp.Header.Get("X-Tingly-Upstream-URL")
		t.capture.MatchedRule = resp.Header.Get("X-Tingly-Matched-Rule")
		t.capture.MatchedRuleDesc = resp.Header.Get("X-Tingly-Matched-Rule-Desc")
		t.capture.AppliedFlags = resp.Header.Get("X-Tingly-Applied-Flags")
		t.capture.Mu.Unlock()
	}
	return resp, err
}

// ApplyRoutingCaptureToClient layers a captureRoutingRoundTripper on a probe
// client's transport chain. The capture pointer is populated after the SDK
// call completes. Returns the capture so the caller can read the result.
func ApplyRoutingCaptureToClient(c interface{}) *RoutingCapture {
	cap := &RoutingCapture{}
	applyTransportWrap(c, func(inner http.RoundTripper) http.RoundTripper {
		return &captureRoutingRoundTripper{inner: inner, capture: cap}
	})
	return cap
}

// TransportPoolInterface defines the interface for transport pools.
// This allows both the real TransportPool and test doubles to be used.
type TransportPoolInterface interface {
	GetTransport(providerUUID, model, proxyURL string, issuer ai.Issuer, sessionID typ.SessionID) *http.Transport
	AcquireTransport(providerUUID, model, proxyURL string, issuer ai.Issuer, sessionID typ.SessionID) (*http.Transport, func())
}

// refCountedBody wraps an io.ReadCloser and calls onclose exactly once when closed.
// The sync.Once prevents double-decrement if the body is closed multiple times.
type refCountedBody struct {
	io.ReadCloser
	once    sync.Once
	onclose func()
}

func (b *refCountedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.onclose)
	return err
}

// SessionBoundTransport is a RoundTripper that binds to a specific session.
// It stores the sessionID and routes all requests through that session's transport.
// This enables session-scoped transport isolation while keeping the http.Client
// as a lightweight, stateless shell.
type SessionBoundTransport struct {
	transportPool TransportPoolInterface
	providerUUID  string
	proxyURL      string
	issuer        ai.Issuer
	sessionID     typ.SessionID // Bound at creation time

	// Optional: provider-specific response wrapper (e.g., for tool prefix stripping)
	responseWrapper func(*http.Response) *http.Response
}

// RoundTrip implements http.RoundTripper for SessionBoundTransport.
// It acquires the transport for the stored session (incrementing refCount),
// executes the request, and wraps the response body to auto-release on close.
func (t *SessionBoundTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport, release := t.transportPool.AcquireTransport(
		t.providerUUID,
		"", // model - not used for transport keying
		t.proxyURL,
		t.issuer,
		t.sessionID,
	)

	resp, err := transport.RoundTrip(req)
	if err != nil {
		release()
		return nil, err
	}

	if resp.Body != nil {
		resp.Body = &refCountedBody{ReadCloser: resp.Body, onclose: release}
	} else {
		release()
	}

	if t.responseWrapper != nil {
		resp = t.responseWrapper(resp)
	}

	return resp, nil
}

// createSessionBoundTransport builds the base session-bound transport for a
// provider. It is intentionally generic — provider-specific wire-format
// adapters (Code Assist envelope, ChatGPT backend translation, etc.) live in
// their dedicated xxx_client.go files and layer their RoundTripper on top of
// what this returns.
//
// A custom User-Agent override (provider.UserAgent) is layered here at the
// innermost position so it wins against any outer adapter that rewrites
// headers before delegating downstream.
func createSessionBoundTransport(provider *typ.Provider, sessionID typ.SessionID) http.RoundTripper {
	var issuer ai.Issuer
	if provider.OAuthDetail != nil {
		issuer = ai.Issuer(provider.OAuthDetail.GetIssuer())
	}

	if provider.ProxyURL != "" {
		logrus.Debugf("Using proxy for provider %s: %s", provider.UUID, provider.ProxyURL)
	}

	base := &SessionBoundTransport{
		transportPool: GetGlobalTransportPool(),
		providerUUID:  provider.UUID,
		proxyURL:      provider.ProxyURL,
		issuer:        issuer,
		sessionID:     sessionID,
	}

	return wrapWithUserAgent(base, provider)
}
