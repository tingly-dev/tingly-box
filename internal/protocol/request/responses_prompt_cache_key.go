package request

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol/metaid"
)

// responsesPromptCacheKey derives a stable prompt_cache_key from an Anthropic
// request's metadata.user_id.
//
// Anthropic has no equivalent field: its prompt cache is addressed purely by
// the request prefix. OpenAI's is not — prompt_cache_key is the affinity hint
// that routes successive requests of one conversation to the same cache-warm
// backend, and a native Codex client always sends one. Converted Anthropic
// traffic used to arrive without it, so a byte-identical prefix could still
// miss the cache depending on where the request landed.
//
// Claude Code's metadata.user_id carries a per-session id (see
// metaid.ParseMetadataUserID), which is exactly the affinity scope wanted: stable
// for the life of a conversation, distinct across conversations. Only that id
// is forwarded, never the device or account fields. A user_id in some other
// shape is hashed, so an unrecognized format still yields a stable key without
// leaking whatever it contains upstream.
func responsesPromptCacheKey(rawUserID string) param.Opt[string] {
	if rawUserID == "" {
		return param.Opt[string]{}
	}
	if m := metaid.ParseMetadataUserID(rawUserID); m != nil && m.SessionID != "" {
		return param.NewOpt(m.SessionID)
	}
	sum := sha256.Sum256([]byte(rawUserID))
	return param.NewOpt(hex.EncodeToString(sum[:16]))
}
