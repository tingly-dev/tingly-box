package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const (
	maxErrorBodyBytes   = 1 << 20
	maxErrorMessageRune = 500
)

// errorEnvelopeTransport guarantees a non-2xx upstream response reaches the
// OpenAI / Anthropic SDK in the envelope it hard-codes: the SDKs decode the
// "error" value straight into a typed struct, so {"error":"text"},
// {"detail":...}, {"message":...}, plain text and HTML bodies (Codex, proxies,
// gateways, TB's own relay) all fail with a Go JSON decode error and the real
// status and message are lost. Well-formed envelopes, successes and encoded
// bodies pass through untouched. See .design/upstream-error-normalization.md.
type errorEnvelopeTransport struct {
	inner http.RoundTripper
	style protocol.APIStyle
}

func wrapWithErrorEnvelope(inner http.RoundTripper, style protocol.APIStyle) http.RoundTripper {
	return &errorEnvelopeTransport{inner: inner, style: style}
}

func (t *errorEnvelopeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode < 400 || resp.Body == nil {
		return resp, err
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		return resp, nil
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil || len(body) > maxErrorBodyBytes {
		// Too large or broken to reason about: hand the SDK what we read.
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), errReader{readErr}))
		return resp, nil
	}

	if !t.decodesAsEnvelope(body) {
		body = t.envelope(resp.StatusCode, body)
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Del("Content-Length")
		resp.ContentLength = int64(len(body))
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	return 0, r.err
}

// decodesAsEnvelope reports whether the SDK would accept body as-is: an
// "error" object that its own error type decodes.
func (t *errorEnvelopeTransport) decodesAsEnvelope(body []byte) bool {
	var top struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &top) != nil || !bytes.HasPrefix(bytes.TrimSpace(top.Error), []byte("{")) {
		return false
	}
	if t.style == protocol.APIStyleAnthropic {
		var e anthropic.Error
		return e.UnmarshalJSON(body) == nil
	}
	var e openai.Error
	return e.UnmarshalJSON(top.Error) == nil
}

func (t *errorEnvelopeTransport) envelope(status int, body []byte) []byte {
	message, typ := describeErrorBody(body)
	if message == "" {
		message = http.StatusText(status)
	}
	if message == "" {
		message = "upstream returned HTTP " + strconv.Itoa(status)
	}
	errObj := map[string]string{"message": message, "type": typ}
	var out any = map[string]any{"error": errObj}
	if t.style == protocol.APIStyleAnthropic {
		out = map[string]any{"type": "error", "error": errObj}
	}
	b, _ := json.Marshal(out)
	return b
}

var (
	htmlTag    = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]*>`)
	whitespace = regexp.MustCompile(`\s+`)
)

// describeErrorBody extracts the human message (and, when the body carried
// one, the error type) from any of the shapes upstreams answer with.
func describeErrorBody(body []byte) (message, errType string) {
	errType = "upstream_error"
	var v any
	if json.Unmarshal(body, &v) != nil {
		return cleanMessage(htmlTag.ReplaceAllString(string(body), " ")), errType
	}
	if m, ok := v.(map[string]any); ok {
		if eo, ok := m["error"].(map[string]any); ok {
			if s, ok := eo["type"].(string); ok && s != "" {
				errType = s
			}
		}
	}
	message = cleanMessage(messageOf(v))
	if message == "" && v != nil {
		message = cleanMessage(string(bytes.TrimSpace(body)))
	}
	return message, errType
}

var messageKeys = []string{"error", "message", "detail", "msg", "error_description", "errors"}

func messageOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		for _, k := range messageKeys {
			if s := messageOf(x[k]); s != "" {
				return s
			}
		}
	case []any:
		var parts []string
		for _, e := range x {
			if s := messageOf(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

func cleanMessage(s string) string {
	s = strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > maxErrorMessageRune {
		s = string(r[:maxErrorMessageRune]) + "…"
	}
	return s
}
