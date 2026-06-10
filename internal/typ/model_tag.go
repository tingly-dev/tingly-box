package typ

import "strings"

// ContextWindow1MTag is the suffix that marks a model name as requesting the
// 1M context window. It is a client convention (Claude Code / Claude Desktop
// read the model name, perceive the [1m] suffix, and send the context-1m beta
// header), not a tingly-box concept: rules store a clean request_model plus
// the Context1M flag, and the suffix is appended only when rendering the
// client-facing config (env / inferenceModels). See .design/one-m-context.md.
const ContextWindow1MTag = "[1m]"

// StripContextWindow1M removes a trailing [1m] suffix from a model name,
// returning the canonical base name. Safe to call on names without the tag.
// Used by routing to match [1m]-tolerantly regardless of which side carries
// the suffix.
func StripContextWindow1M(model string) string {
	return strings.TrimSuffix(model, ContextWindow1MTag)
}

// WithContextWindow1M appends the [1m] suffix to a model name (idempotent).
// Used when rendering client-facing configs for rules with Context1M set.
func WithContextWindow1M(model string) string {
	return StripContextWindow1M(model) + ContextWindow1MTag
}
