package protocol

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/openai/openai-go/v3"
)

// maxClientDetailLen caps the vendor message relayed to a client so a
// misbehaving upstream cannot push an arbitrarily large body through us.
const maxClientDetailLen = 512

// SDK Error() text is not a stable contract: openai-go now returns only the
// HTTP status line (no URL, no provider fields). The client-facing message
// is therefore assembled here from typed fields, with an explicit allowlist:
// status, type, code and the vendor's human-readable message. The request
// URL, param, headers and raw body never reach the client; they belong in
// the server log (see UpstreamLogDetail).

// safeOpenAIMessage builds the client-safe description of an openai.Error.
func safeOpenAIMessage(e *openai.Error) string {
	var b strings.Builder
	b.WriteString("OpenAI API error: ")
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
