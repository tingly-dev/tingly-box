package client

import (
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
