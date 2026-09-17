// Package ids mints the identifiers tingly-box puts on protocol objects it
// synthesizes on behalf of an upstream that speaks a different protocol:
// Responses API response and output item ids, Anthropic message ids, and a
// tool call id when the upstream omitted one.
//
// The gateway serves the standard wire form first and adapts providers
// behind it, so every minted id has the canonical OpenAI shape
// "<prefix>_<32 lowercase hex>" (a random uuid v4) regardless of which
// provider produced the content. Random ids are also what OpenAI itself
// issues; clients such as Codex persist them and replay them verbatim, so an
// id is random only at the moment it is minted and stable ever after.
//
// The shape satisfies every constraint OpenAI enforces when such ids are
// replayed into a native Responses request: type prefix (fc_, msg_, rs_),
// charset [A-Za-z0-9_-], at most 64 characters, unique within the input.
// Neither timestamps (collide within a second) nor upstream-derived ids
// (leak the provider's shape, can exceed 64 chars) meet all of them.
//
// See .design/protocol-responses.md §2 for the discussion behind these rules.
package ids

import (
	"encoding/hex"

	"github.com/google/uuid"
)

// Prefixes of the ids the gateway mints.
const (
	PrefixResponse     = "resp"
	PrefixMessage      = "msg"
	PrefixFunctionCall = "fc"
	PrefixReasoning    = "rs"
	PrefixCall         = "call"
)

var generate = randomHex

func randomHex() string {
	id := uuid.New()
	return hex.EncodeToString(id[:])
}

// New returns "<prefix>_<32 hex>".
func New(prefix string) string {
	return prefix + "_" + generate()
}

// Response mints a Responses API response id (resp_...).
func Response() string { return New(PrefixResponse) }

// Message mints a message item id (msg_...). It doubles as the Anthropic
// message id, whose canonical form uses the same prefix.
func Message() string { return New(PrefixMessage) }

// FunctionCall mints a function_call item id (fc_...). It is distinct from
// the call_id, which is the correlation key and always comes from upstream
// when the upstream provides one.
func FunctionCall() string { return New(PrefixFunctionCall) }

// Reasoning mints a reasoning item id (rs_...).
func Reasoning() string { return New(PrefixReasoning) }

// Call mints a tool call correlation id (call_...) for an upstream tool call
// that arrived without one.
func Call() string { return New(PrefixCall) }

// SetGeneratorForTest replaces the random suffix generator, so tests and
// recording replays get a deterministic sequence. It returns a restore func.
func SetGeneratorForTest(fn func() string) (restore func()) {
	prev := generate
	generate = fn
	return func() { generate = prev }
}
