package typ

import "strings"

// ContextWindow1MTag is the suffix that marks a model name as requesting the
// 1M context window. It is a client convention (Claude Code / Claude Desktop
// read the model name, perceive the [1m] suffix, and send the context-1m beta
// header). tingly-box carries it directly on a rule's request_model — toggling
// 1M renames the rule (e.g. "ds" -> "ds[1m]"). See .design/one-m-context.md.
const ContextWindow1MTag = "[1m]"

// StripContextWindow1M removes a trailing [1m] suffix from a model name,
// returning the canonical base name. Safe to call on names without the tag.
// Used by routing to match [1m]-tolerantly regardless of which side carries
// the suffix.
func StripContextWindow1M(model string) string {
	return strings.TrimSuffix(model, ContextWindow1MTag)
}
