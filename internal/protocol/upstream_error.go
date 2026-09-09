package protocol

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// UpstreamFailure is the client-facing shape of a failed upstream call: the
// HTTP status to answer the caller with, and a client-safe description of
// what happened. It exists because every call site that reports an upstream
// failure needs both — Status decides the JSON status code, Message fills
// the body — and building them separately via UpstreamStatus + UpstreamMessage
// means classifying the same err twice. ClassifyUpstreamFailure does it once.
type UpstreamFailure struct {
	Status  int
	Message string
}

// ClassifyUpstreamFailure classifies err once into the status
// (UpstreamStatus's rules) and message (UpstreamMessage's rules) a call site
// needs to report it to a client. Prefer this over calling UpstreamStatus and
// UpstreamMessage separately when a call site needs both — which is every
// current call site that has an HTTP status to report at all (the
// exceptions — respondMCPError, FailAttemptSetup, and the mid-stream SSE
// error-event sites — only ever need the message: they either hardcode their
// status or, mid-stream, have none to send).
func ClassifyUpstreamFailure(err error, fallbackStatus int) UpstreamFailure {
	if err == nil {
		return UpstreamFailure{Status: fallbackStatus}
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		redacted := *oaiErr
		redacted.Request = redactedRequest(oaiErr.Request)
		f := UpstreamFailure{Status: fallbackStatus, Message: redacted.Error()}
		if oaiErr.StatusCode >= 400 {
			f.Status = oaiErr.StatusCode
		}
		return f
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		redacted := *anthropicErr
		redacted.Request = redactedRequest(anthropicErr.Request)
		f := UpstreamFailure{Status: fallbackStatus, Message: redacted.Error()}
		if anthropicErr.StatusCode >= 400 {
			f.Status = anthropicErr.StatusCode
		}
		return f
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) {
		f := UpstreamFailure{Status: fallbackStatus, Message: genaiErr.Error()}
		if genaiErr.Code >= 400 {
			f.Status = genaiErr.Code
		}
		return f
	}

	if reason, ok := ClassifyTransportError(err); ok {
		return UpstreamFailure{
			Status:  http.StatusBadGateway,
			Message: string(reason) + ": " + transportFailureMessages[reason],
		}
	}

	return UpstreamFailure{Status: fallbackStatus, Message: err.Error()}
}

// UpstreamStatus extracts the HTTP status code that an upstream provider
// returned, so the gateway can propagate it to the client instead of
// flattening every forwarding failure into a 500. A thin wrapper over
// ClassifyUpstreamFailure for call sites that only need the status; prefer
// ClassifyUpstreamFailure directly when a message is needed too, so err is
// only classified once.
func UpstreamStatus(err error, fallback int) int {
	return ClassifyUpstreamFailure(err, fallback).Status
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
// would otherwise have missed), ClassifyUpstreamFailure calls the SDK's own
// Error() on a copy with the URL blanked out, so it stays byte-for-byte in
// sync with upstream except for that one line. genai.APIError's Error()
// never included the URL, so it passes through unchanged.
//
// A thin wrapper over ClassifyUpstreamFailure for call sites that only need
// the message (no HTTP status to report — a mid-stream SSE error event, or a
// handler that hardcodes its status); prefer ClassifyUpstreamFailure directly
// when the status is needed too, so err is only classified once.
func UpstreamMessage(err error) string {
	return ClassifyUpstreamFailure(err, 0).Message
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
