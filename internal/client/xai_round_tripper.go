package client

import (
	"net/http"
)

// Grok CLI impersonation headers sent with every inference request against
// xAI's OAuth-only CLI proxy (cli-chat-proxy.grok.com). Without them the
// proxy rejects the token even though it is otherwise valid.
// Reference: reverse-engineered from third-party Grok CLI OAuth clients
// (e.g. dongguatanglinux/grok-build-auth) — xAI does not publish this API.
const (
	xaiTokenAuthHeader       = "xai-grok-cli"
	xaiClientVersionHeader   = "0.2.93"
	xaiClientIdentifierValue = "grok-shell"
)

// xaiRoundTripper layers Grok CLI impersonation headers on an inner
// transport. The Authorization Bearer is set by the OpenAI SDK.
type xaiRoundTripper struct {
	http.RoundTripper
}

func newXAIRoundTripper(inner http.RoundTripper) *xaiRoundTripper {
	return &xaiRoundTripper{RoundTripper: inner}
}

func (t *xaiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-XAI-Token-Auth", xaiTokenAuthHeader)
	req.Header.Set("x-grok-client-version", xaiClientVersionHeader)
	req.Header.Set("x-grok-client-identifier", xaiClientIdentifierValue)

	return t.RoundTripper.RoundTrip(req)
}
