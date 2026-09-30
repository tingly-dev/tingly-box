package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// DecisionEndpointURL normalizes a decision endpoint base to the native
// decisions URL. "https://host/api/v1", "https://host/api/v1/", and
// "https://host/api/v1/decisions" all map to "https://host/api/v1/decisions";
// query and fragment are dropped.
func DecisionEndpointURL(apiBase string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid decision endpoint base %q", apiBase)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/decisions") {
		u.Path += "/decisions"
	}
	return u.String(), nil
}

// DecisionHTTPResult is the raw upstream answer for a decision request. The
// decision protocol is passed through natively — no SDK type covers it — so
// status, content type, and body travel opaquely (see
// .design/decision-protocol.md §3).
type DecisionHTTPResult struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// DecisionClient is the dedicated client for the structured-decision protocol.
// It is not a chat client with an extra method: the decision surface is an
// endpoint fork that OpenAI/Anthropic upstreams do not natively have, so it
// gets its own client. Any provider that exposes a decision endpoint — a
// Jev-native provider (api_style decision), or a chat provider with a decision
// fork URL (APIBaseDecision) — is served through this one client type, which
// is what lets users configure providers by model fit instead of protocol.
type DecisionClient struct {
	provider   *typ.Provider
	endpoint   string
	HttpClient *http.Client
}

// NewDecisionClient builds the decision client for a provider. The endpoint
// comes from the provider's decision fork URL, falling back to APIBase for
// Jev-native (api_style decision) providers.
func NewDecisionClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*DecisionClient, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is required")
	}
	if !provider.HasDecisionEndpoint() {
		return nil, fmt.Errorf("provider %s does not expose a decision endpoint", provider.Name)
	}
	endpoint, err := DecisionEndpointURL(provider.DecisionBase())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve decision endpoint for provider %s: %w", provider.Name, err)
	}

	issuer := ai.Issuer("")
	if provider.OAuthDetail != nil {
		issuer = provider.OAuthDetail.GetIssuer()
	}
	base := GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, issuer, sessionID)

	return &DecisionClient{
		provider:   provider,
		endpoint:   endpoint,
		HttpClient: &http.Client{Transport: providerTransportChain(base, provider)},
	}, nil
}

// decisionTimeout returns the request timeout for a decision provider,
// mirroring the chat clients' provider-timeout defaulting.
func decisionTimeout(provider *typ.Provider) time.Duration {
	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}
	return timeout
}

// Decisions posts one structured-decision request. body must already carry the
// routed (service) model name. Transport failures surface as the returned
// error; an upstream HTTP error response (4xx/5xx) is NOT an error here — it
// comes back in the result so callers can pass the provider's own error JSON
// through verbatim.
func (c *DecisionClient) Decisions(ctx context.Context, body []byte) (*DecisionHTTPResult, error) {
	ctx, cancel := context.WithTimeout(ctx, decisionTimeout(c.provider))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create decision upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token := c.provider.GetAccessToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read decision upstream response: %w", err)
	}

	return &DecisionHTTPResult{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        respBody,
	}, nil
}

// Close closes any resources held by the client.
func (c *DecisionClient) Close() error {
	if c.HttpClient != nil && c.HttpClient != http.DefaultClient {
		c.HttpClient.CloseIdleConnections()
	}
	return nil
}
