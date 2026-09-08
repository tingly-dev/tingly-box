package client

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenCodeSessionHeader is the conversation identifier OpenCode Zen requires
// on every request. Without it the upstream rejects the call with HTTP 400
// `MissingSessionID` ("Request is missing x-opencode-session and cannot be
// routed efficiently"), so the gateway must supply one on the
// Tingly-Box → OpenCode hop; the client's own header never survives, because
// the SDK builds a fresh outbound request (#1713).
const OpenCodeSessionHeader = "x-opencode-session"

// openCodeSessionPrefix marks a value as minted by Tingly-Box. Purely for
// legibility in upstream logs — the value itself is opaque.
const openCodeSessionPrefix = "tb-"

// openCodeRoundTripper is the OpenCode Zen vendor layer: it stamps the
// conversation identifier the upstream requires. Same shape as the Codex and
// Kimi round-trippers — one vendor handshake concern, applied by the vendor's
// own client constructor rather than by the generic chain.
//
// It fills an absent header only, so a value pinned through the rule's
// extra_headers still wins (the rule-flag layer sits above it).
type openCodeRoundTripper struct {
	http.RoundTripper
}

// IsOpenCodeZen reports whether apiBase points at OpenCode Zen. Both products
// live on the same host and both route through the session header: "/zen/v1"
// (pay-as-you-go) and "/zen/go/v1" (the Go subscription), plus their
// Anthropic-style bases without the "/v1" suffix.
//
// Matching goes through ops.SplitProviderHostPath for the same reason the
// request-side vendor transforms do: a relay whose URL merely mentions the
// host in its path is not OpenCode.
func IsOpenCodeZen(apiBase string) bool {
	host, path := ops.SplitProviderHostPath(apiBase)
	return host == "opencode.ai" && strings.HasPrefix(path, "/zen")
}

func (t *openCodeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.RoundTripper
	if inner == nil {
		inner = http.DefaultTransport
	}
	if req.Header.Get(OpenCodeSessionHeader) != "" {
		return inner.RoundTrip(req)
	}

	// Clone before mutating so a retry never races on shared headers.
	req = req.Clone(req.Context())
	req.Header.Set(OpenCodeSessionHeader, openCodeSessionValue(typ.GetSessionID(req.Context())))
	return inner.RoundTrip(req)
}

// openCodeSessionValue turns the request's resolved session into an opaque,
// stable per-conversation token.
//
// Stability is the point: Zen uses the header for backend affinity, and with
// it prompt-cache warmth. One constant for every conversation would clear the
// 400 while collapsing them onto one affinity scope; a fresh value per
// request would clear it while defeating cache reuse entirely.
//
// The hash matters too: the resolved session falls back to the client IP, and
// can be a user id carried in Anthropic metadata — neither is something to
// hand an upstream verbatim.
//
// A request with no resolved session gets a random value rather than a shared
// constant: it still reaches the upstream, and unrelated conversations stay
// off one scope. Every dispatch path puts the session in the context, so this
// is the probe/diagnostic case, not the request path.
func openCodeSessionValue(session typ.SessionID) string {
	if session.Value == "" {
		return openCodeSessionPrefix + uuid.NewString()
	}
	sum := sha256.Sum256([]byte(session.Value))
	return openCodeSessionPrefix + hex.EncodeToString(sum[:16])
}
