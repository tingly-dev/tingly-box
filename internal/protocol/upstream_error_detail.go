package protocol

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// maxClientDetailLen caps the vendor message relayed to a client so a
// misbehaving upstream cannot push an arbitrarily large body through us.
const maxClientDetailLen = 512

// maxLogDetailLen caps the raw upstream body kept for the server log.
const maxLogDetailLen = 4096

// SDK Error() text is not a stable contract: openai-go now returns only the
// HTTP status line (no URL, no provider fields), and anthropic-sdk-go may
// follow. The client-facing message is therefore assembled here from typed
// fields, with an explicit allowlist: status, type, code and the vendor's
// human-readable message (plus request/workspace ids for vendor support).
// The request URL, param, headers and raw body never reach the client; they
// belong in the server log (see UpstreamLogDetail).

// safeOpenAIMessage builds the client-safe description of an openai.Error.
// openai.Error also carries OpenAI-compatible third-party vendors (DeepSeek,
// OpenRouter, self-hosted gateways), so the prefix is deliberately neutral:
// naming "OpenAI" would mislabel them, and naming the vendor or its host
// would expose routing details. Vendor identity stays in the server log.
func safeOpenAIMessage(e *openai.Error) string {
	var b strings.Builder
	b.WriteString("Upstream error: ")
	b.WriteString(statusLine(e.StatusCode))
	var attrs []string
	if e.Type != "" {
		attrs = append(attrs, "type="+e.Type)
	}
	if e.Code != "" {
		attrs = append(attrs, "code="+e.Code)
	}
	if len(attrs) > 0 {
		b.WriteString(" (" + strings.Join(attrs, ", ") + ")")
	}
	if msg := truncateClientDetail(e.Message); msg != "" {
		b.WriteString(": " + msg)
	}
	return b.String()
}

func statusLine(code int) string {
	if text := http.StatusText(code); text != "" {
		return fmt.Sprintf("%d %s", code, text)
	}
	return fmt.Sprintf("%d", code)
}

func truncateClientDetail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxClientDetailLen {
		return s
	}
	cut := maxClientDetailLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// safeAnthropicMessage builds the client message for an anthropic.Error from
// typed fields, in the same shape the SDK's Error() has today with the URL
// replaced by "REDACTED" (clients already receive the vendor's error body).
// Pinning the shape here means a future anthropic-sdk-go change to Error()
// (as openai-go just made) cannot silently alter or leak anything; moving to
// the terser allowlist form used for OpenAI is a separate, deliberate change.
func safeAnthropicMessage(e *anthropic.Error) string {
	method := http.MethodPost
	if e.Request != nil && e.Request.Method != "" {
		method = e.Request.Method
	}
	msg := fmt.Sprintf("%s %q: %s", method, "REDACTED", statusLine(e.StatusCode))
	if e.RequestID != "" {
		msg += fmt.Sprintf(" (Request-ID: %s)", e.RequestID)
	}
	if e.WorkspaceID != "" {
		msg += fmt.Sprintf(" (Workspace-ID: %s)", e.WorkspaceID)
	}
	if body := e.RawJSON(); body != "" {
		msg += " " + body
	}
	return msg
}

// UpstreamLogDetail returns the full diagnostic text for the server log: the
// fields withheld from clients (param, raw body). It returns "" when err
// carries nothing beyond its own Error(). Never send this to a client.
func UpstreamLogDetail(err error) string {
	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		return fmt.Sprintf("openai status=%d param=%q body=%s", oaiErr.StatusCode, oaiErr.Param, truncateLogDetail(oaiErr.RawJSON()))
	}
	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		return fmt.Sprintf("anthropic status=%d request_id=%q body=%s", anthropicErr.StatusCode, anthropicErr.RequestID, truncateLogDetail(anthropicErr.RawJSON()))
	}
	return ""
}

func truncateLogDetail(s string) string {
	if len(s) <= maxLogDetailLen {
		return s
	}
	return s[:maxLogDetailLen] + "…(truncated)"
}

// LoggableUpstreamError wraps err so its Error() text, which gin and the
// access-log middleware record, includes UpstreamLogDetail. errors.As/Is
// still see the original. Returns err unchanged when there is no detail.
func LoggableUpstreamError(err error) error {
	if err == nil {
		return nil
	}
	detail := UpstreamLogDetail(err)
	if detail == "" {
		return err
	}
	return &loggableUpstreamError{err: err, detail: detail}
}

type loggableUpstreamError struct {
	err    error
	detail string
}

func (e *loggableUpstreamError) Error() string { return e.err.Error() + " | " + e.detail }
func (e *loggableUpstreamError) Unwrap() error { return e.err }
