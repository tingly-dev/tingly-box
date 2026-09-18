package protocol

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// UpstreamFailure is the client-facing status + message for a failed
// upstream call. See ClassifyUpstreamFailure.
type UpstreamFailure struct {
	Status  int
	Message string
}

// ClassifyUpstreamFailure classifies err once into both the HTTP status to
// answer the client with and a client-safe message — the status and message
// rules are documented on UpstreamStatus and UpstreamMessage respectively.
// Prefer this over calling those two separately when a call site needs both,
// so err is only classified once (see .design/logging.md §3 for why this
// exists and which call sites need only one of the two).
func ClassifyUpstreamFailure(err error, fallbackStatus int) UpstreamFailure {
	if err == nil {
		return UpstreamFailure{Status: fallbackStatus}
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		redacted := *oaiErr
		redacted.Request = redactedRequest(oaiErr.Request)
		return withSDKStatus(fallbackStatus, oaiErr.StatusCode, redacted.Error())
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		redacted := *anthropicErr
		redacted.Request = redactedRequest(anthropicErr.Request)
		return withSDKStatus(fallbackStatus, anthropicErr.StatusCode, redacted.Error())
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) {
		return withSDKStatus(fallbackStatus, genaiErr.Code, genaiErr.Error())
	}

	if reason, ok := ClassifyTransportError(err); ok {
		return UpstreamFailure{
			Status:  http.StatusBadGateway,
			Message: string(reason) + ": " + transportFailureMessages[reason],
		}
	}

	return UpstreamFailure{Status: fallbackStatus, Message: err.Error()}
}

// UpstreamStatus extracts the HTTP status an upstream provider returned
// (openai.Error / anthropic.Error / genai.APIError's own status), so a
// 401/429/4xx isn't flattened into a generic 500; a transport-level failure
// (no HTTP response at all) maps to 502; anything else falls back to
// fallback. A thin wrapper over ClassifyUpstreamFailure.
func UpstreamStatus(err error, fallback int) int {
	return ClassifyUpstreamFailure(err, fallback).Status
}

// UpstreamMessage returns a client-safe description of err: a transport
// failure is described by category, not its raw Go dial/DNS/TLS text; an
// SDK-typed error is the vendor's own Error() with the outbound request URL
// redacted (delegating to Error() itself rather than reprinting selected
// fields keeps every other field — e.g. anthropic.Error's WorkspaceID — in
// sync with the SDK automatically). A thin wrapper over
// ClassifyUpstreamFailure.
func UpstreamMessage(err error) string {
	return ClassifyUpstreamFailure(err, 0).Message
}

// withSDKStatus applies the one rule shared by every SDK-typed error branch:
// use the SDK's own status if it's a real error status, else fall back.
func withSDKStatus(fallbackStatus, sdkStatus int, message string) UpstreamFailure {
	f := UpstreamFailure{Status: fallbackStatus, Message: message}
	if sdkStatus >= 400 {
		f.Status = sdkStatus
	}
	return f
}

// redactedRequest returns a copy of req (nil-safe) with the URL replaced by
// a fixed placeholder, keeping the method intact.
func redactedRequest(req *http.Request) *http.Request {
	redacted := &http.Request{URL: &url.URL{Path: "REDACTED"}}
	if req != nil {
		redacted.Method = req.Method
	}
	return redacted
}
