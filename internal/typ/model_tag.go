package typ

import "strings"

// ContextWindow1MTag is the suffix appended to a model name to request the
// 1M context window. It is a Claude Code client convention: Claude Code reads
// the model name from its env, perceives the [1m] suffix, and sends the
// context-1m beta header. tingly-box treats it as part of the rule's
// request_model so the suffix flows end to end (rule → generated env → wire →
// exact rule match). See .design/one-m-context.md.
const ContextWindow1MTag = "[1m]"

// HasContextWindow1M reports whether a model name carries the [1m] suffix.
func HasContextWindow1M(model string) bool {
	return strings.HasSuffix(model, ContextWindow1MTag)
}

// StripContextWindow1M removes a trailing [1m] suffix from a model name,
// returning the canonical base name. Safe to call on names without the tag.
func StripContextWindow1M(model string) string {
	return strings.TrimSuffix(model, ContextWindow1MTag)
}

// WithContextWindow1M adds or removes the [1m] suffix on a model name. It is
// idempotent: enabling never double-appends, disabling always strips.
func WithContextWindow1M(model string, on bool) string {
	base := StripContextWindow1M(model)
	if on {
		return base + ContextWindow1MTag
	}
	return base
}
