// Package smart_compact provides conversation compression strategies and
// transformers for Anthropic requests, consumed by the virtual compact models
// (vmodel/anthropic: "compact-round-only", "compact-round-files",
// "claude-code-compact", "claude-code-strategy").
//
// The package includes strategy implementations (conversation replay,
// document, round-only, round-files) and transform.Transform implementations:
//
//	NewRoundOnlyTransform() - keeps only user/assistant text
//	NewRoundFilesTransform() - keeps text + virtual file tools
//	NewConversationReplayTransformer() - replay-based compression
//	NewConversationDocumentTransformer() - document-based compression
//	NewDeduplicationTransform() - removes duplicate tool calls
//	NewPurgeErrorsTransform() - removes errored tool inputs
//	NewXMLCompactTransform() - XML-document compaction
//
// Strategies compress conversation rounds by removing thinking blocks,
// tool calls, and tool results while preserving the essential flow
// of user requests and assistant responses.
//
// The flag-driven thinking trim (the SmartCompact scenario flag) is NOT
// here: it lives in internal/server/transform.ThinkingCompactTransform,
// since it is a server-domain transform rather than a vmodel compression
// strategy.
package smart_compact
