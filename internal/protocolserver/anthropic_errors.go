package protocolserver

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Anthropic error conventions for the Anthropic-protocol routes, from the
// Claude Code gateway protocol (embedded in the CLI, see
// .research/claude-code/2.1.328.md §5):
//
//   - error bodies use the Anthropic envelope {"type":"error","error":{...}};
//     the gateway's own errors historically omitted the outer "type";
//   - a terminal error carries `x-should-retry: false`; without it Claude Code
//     keeps retrying a 401 with backoff for about three minutes and a 501 once
//     more before it gives up.
//
// ~60 call sites write ErrorResponse (plus the auth middleware), so the
// conventions are applied once, at the response writer, instead of at each site.

// anthropicTerminalStatus reports statuses a retry cannot fix. 408/409/429 and
// 5xx stay retryable (the SDK retries them), except 501 which means "not
// supported here".
func anthropicTerminalStatus(code int) bool {
	switch {
	case code == http.StatusNotImplemented:
		return true
	case code >= 400 && code < 500:
		return code != http.StatusRequestTimeout && code != http.StatusConflict && code != http.StatusTooManyRequests
	}
	return false
}

// anthropicErrorConventions wraps the response writer; it must run before the
// auth middleware so authentication failures follow the same conventions.
func anthropicErrorConventions(c *gin.Context) {
	c.Writer = &anthropicErrorWriter{ResponseWriter: c.Writer}
	c.Next()
}

type anthropicErrorWriter struct {
	gin.ResponseWriter
	status int
}

func (w *anthropicErrorWriter) WriteHeader(code int) {
	w.status = code
	if anthropicTerminalStatus(code) && w.Header().Get("x-should-retry") == "" {
		w.Header().Set("x-should-retry", "false")
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *anthropicErrorWriter) Write(p []byte) (int, error) {
	if patched, ok := w.envelope(p); ok {
		n, err := w.ResponseWriter.Write(patched)
		if err == nil && n == len(patched) {
			n = len(p)
		}
		return n, err
	}
	return w.ResponseWriter.Write(p)
}

func (w *anthropicErrorWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// envelope adds the outer "type":"error" to a single-write JSON error body
// that has an "error" object but no "type". Anything else is left alone.
func (w *anthropicErrorWriter) envelope(p []byte) ([]byte, bool) {
	status := w.status
	if status == 0 {
		status = w.ResponseWriter.Status()
	}
	if status < 400 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		return nil, false
	}
	if !gjson.GetBytes(p, "error").IsObject() || gjson.GetBytes(p, "type").Exists() {
		return nil, false
	}
	// c.JSON ends the body with no newline, but be careful not to corrupt a
	// body that is not a single JSON object.
	if !bytes.HasPrefix(bytes.TrimSpace(p), []byte("{")) {
		return nil, false
	}
	out, err := sjson.SetBytes(p, "type", "error")
	if err != nil {
		return nil, false
	}
	return out, true
}
