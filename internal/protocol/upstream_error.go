package protocol

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// UpstreamStatus extracts the HTTP status code that an upstream provider
// returned, so the gateway can propagate it to the client instead of flattening
// every forwarding failure into a 500. It understands the error types returned
// by each vendor SDK (OpenAI / Anthropic share apierror.Error; google-genai
// uses genai.APIError). When the error does not carry a usable upstream status,
// a transport-level failure (DNS, TCP connect, TLS, timeout — no HTTP response
// was ever received) maps to 502 Bad Gateway, since that describes it more
// accurately than a generic internal error; anything else falls back to
// fallback.
func UpstreamStatus(err error, fallback int) int {
	if err == nil {
		return fallback
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) && oaiErr.StatusCode >= 400 {
		return oaiErr.StatusCode
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) && anthropicErr.StatusCode >= 400 {
		return anthropicErr.StatusCode
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) && genaiErr.Code >= 400 {
		return genaiErr.Code
	}

	if _, ok := ClassifyTransportError(err); ok {
		return http.StatusBadGateway
	}

	return fallback
}

// UpstreamMessage returns a client-safe description of err.
//
// A transport-level failure is described by category instead of the raw Go
// dial/DNS/TLS error text — long, jargon-heavy, and a server-log detail, not
// something an API caller needs verbatim.
//
// An SDK-typed provider error already carries the provider's own error body,
// which is genuinely useful to the caller (e.g. "rate_limit_error"), so that
// part passes through. But openai.Error and anthropic.Error's own Error()
// also embeds the full outbound request line — method + the exact URL we
// called upstream (openai-go/internal/apierror/apierror.go, anthropic-sdk-go
// same) — which leaks our upstream endpoint (a custom/internal API base in
// particular) to the downstream caller for no reason. Rather than hand-picking
// which fields to re-print (which silently drops whatever the SDK's Error()
// format adds later — anthropic.Error already prints a WorkspaceID this PR
// would otherwise have missed), this calls the SDK's own Error() on a copy
// with the URL blanked out, so it stays byte-for-byte in sync with upstream
// except for that one line. genai.APIError's Error() never included the URL,
// so it passes through unchanged.
func UpstreamMessage(err error) string {
	if err == nil {
		return ""
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		redacted := *oaiErr
		redacted.Request = redactedRequest(oaiErr.Request)
		return redacted.Error()
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		redacted := *anthropicErr
		redacted.Request = redactedRequest(anthropicErr.Request)
		return redacted.Error()
	}

	if reason, ok := ClassifyTransportError(err); ok {
		return string(reason) + ": " + transportFailureMessages[reason]
	}

	return err.Error()
}

// redactedRequest returns a copy of req (nil-safe) with the URL replaced by a
// fixed placeholder, keeping the method intact. Used to launder an SDK-typed
// error's own Error() through unmodified except for the leaked endpoint.
func redactedRequest(req *http.Request) *http.Request {
	redacted := &http.Request{URL: &url.URL{Path: "REDACTED"}}
	if req != nil {
		redacted.Method = req.Method
	}
	return redacted
}
