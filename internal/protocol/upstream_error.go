package protocol

import (
	"errors"
	"fmt"
	"net/http"

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

	if _, _, ok := ClassifyTransportError(err); ok {
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
// particular) to the downstream caller for no reason; this reconstructs the
// message from the same fields minus that line. genai.APIError's Error()
// never included the URL, so it passes through unchanged.
func UpstreamMessage(err error) string {
	if err == nil {
		return ""
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		return statusText(oaiErr.StatusCode) + ": " + oaiErr.RawJSON()
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		msg := statusText(anthropicErr.StatusCode)
		if anthropicErr.RequestID != "" {
			msg += fmt.Sprintf(" (Request-ID: %s)", anthropicErr.RequestID)
		}
		return msg + ": " + anthropicErr.RawJSON()
	}

	if reason, msg, ok := ClassifyTransportError(err); ok {
		return string(reason) + ": " + msg
	}

	return err.Error()
}

func statusText(code int) string {
	return fmt.Sprintf("%d %s", code, http.StatusText(code))
}
