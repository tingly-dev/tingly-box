package protocol

import (
	"errors"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tidwall/gjson"
	"google.golang.org/genai"
)

// UpstreamFailure is the client-facing status, code and message for a failed
// upstream call. See ClassifyUpstreamFailure.
type UpstreamFailure struct {
	Status  int
	Message string
	Code    string
}

// CodeOr retains machine-readable provider errors that clients use for
// recovery (notably Codex's context-overflow trimming during local compaction).
func (f UpstreamFailure) CodeOr(fallback string) string {
	if f.Code != "" {
		return f.Code
	}
	return fallback
}

// ClassifyUpstreamFailure classifies err once into both the HTTP status to
// answer the client with, its machine-readable code and a client-safe message.
// The status and message
// rules are documented on UpstreamStatus and UpstreamMessage respectively.
// Prefer this over calling those two separately when a call site needs both,
// so err is only classified once (see .design/logging.md §3 for why this
// exists and which call sites need only one of the two).
func ClassifyUpstreamFailure(err error, fallbackStatus int) UpstreamFailure {
	if err == nil {
		return UpstreamFailure{Status: fallbackStatus}
	}

	// An upstream that answered with nothing usable failed as a gateway
	// would: 502 unless the cause carries the provider's own status.
	var empty *EmptyResponseError
	if errors.As(err, &empty) {
		fallbackStatus = http.StatusBadGateway
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		failure := withSDKStatus(fallbackStatus, oaiErr.StatusCode, safeOpenAIMessage(oaiErr))
		failure.Code = oaiErr.Code
		return failure
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		failure := withSDKStatus(fallbackStatus, anthropicErr.StatusCode, safeAnthropicMessage(anthropicErr))
		detail := gjson.Get(anthropicErr.RawJSON(), "error")
		failure.Code = detail.Get("code").String()
		// Anthropic uses an invalid_request_error message instead of a separate
		// context code. Map only its explicit overflow signature.
		if (anthropicErr.StatusCode == 400 || anthropicErr.StatusCode == 200) && detail.Get("type").String() == "invalid_request_error" && strings.HasPrefix(strings.ToLower(detail.Get("message").String()), "prompt is too long:") {
			failure.Status = http.StatusBadRequest
			failure.Code = "context_length_exceeded"
		}
		return failure
	}
	var streamErr *openaistream.StreamError
	if errors.As(err, &streamErr) {
		detail := gjson.GetBytes(streamErr.Event.Data, "error")
		failure := UpstreamFailure{Status: fallbackStatus, Code: detail.Get("code").String(), Message: truncateClientDetail(detail.Get("message").String())}
		if failure.Message == "" {
			failure.Message = "upstream stream failed"
		}
		// An SSE error has no failing HTTP status. Context overflow has a
		// defined 400 contract; 500 prevents the CLI from trimming history.
		if failure.Code == "context_length_exceeded" {
			failure.Status = http.StatusBadRequest
		}
		return failure
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

// EmptyResponseError marks an upstream call that ended without a usable
// answer, such as a stream that closed before any output. It classifies as
// 502 (see ClassifyUpstreamFailure), so failover retries another candidate.
type EmptyResponseError struct {
	Err error
}

func (e *EmptyResponseError) Error() string {
	return "upstream returned an empty response: " + e.Err.Error()
}

func (e *EmptyResponseError) Unwrap() error { return e.Err }

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
// SDK-typed error is assembled from its typed fields (see
// safeOpenAIMessage / safeAnthropicMessage) because the SDKs' own Error()
// text is not a stable contract. The URL, param and raw body stay out; use
// UpstreamLogDetail for the server log. A thin wrapper over
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
