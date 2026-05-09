package smartrouting

import "strings"

// Claude Code request kinds. These are the values emitted by
// DetectClaudeCodeRequestKind and accepted by the agent.claude_code SmartOp.
const (
	ClaudeCodeKindMain     = "main"
	ClaudeCodeKindSubagent = "subagent"
	ClaudeCodeKindCompact  = "compact"
)

// Signal fragments used to fingerprint Claude Code request types.
// Sourced from Claude Code CLI prompt templates as of 2026-05.
const (
	// Compact: Claude Code's /compact emits a system prompt asking the model to
	// produce a structured summary of the prior conversation. The phrase
	// "summary of the conversation" appears prominently and is rare in normal
	// chat traffic.
	claudeCodeCompactMarker = "summary of the conversation"

	// Main: the primary Claude Code agent's system prompt opens with a fixed
	// identity preamble. Presence of this string indicates the main agent loop.
	claudeCodeMainPreamble = "You are Claude Code"

	// Subagent: the Task tool injects a fresh sub-agent system prompt that
	// opens with "You are an agent" (and notably does NOT include the main
	// preamble). This is also stable across versions because it's part of the
	// Task tool description embedded in Claude Code's binary.
	claudeCodeSubagentMarker = "You are an agent"
)

// DetectClaudeCodeRequestKind inspects an already-built RequestContext and
// returns one of the ClaudeCodeKind* values. Callers must ensure the request
// scenario is claude_code before invoking; for other scenarios the result is
// not meaningful.
//
// Precedence is compact → subagent → main (most-specific first).
func DetectClaudeCodeRequestKind(ctx *RequestContext) string {
	if ctx == nil {
		return ClaudeCodeKindMain
	}
	if isClaudeCodeCompactRequest(ctx) {
		return ClaudeCodeKindCompact
	}
	if isClaudeCodeSubagentRequest(ctx) {
		return ClaudeCodeKindSubagent
	}
	return ClaudeCodeKindMain
}

func isClaudeCodeCompactRequest(ctx *RequestContext) bool {
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, claudeCodeCompactMarker) {
			return true
		}
	}
	for _, user := range ctx.UserMessages {
		if strings.Contains(user, claudeCodeCompactMarker) {
			return true
		}
	}
	return false
}

func isClaudeCodeSubagentRequest(ctx *RequestContext) bool {
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, claudeCodeMainPreamble) {
			return false
		}
	}
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, claudeCodeSubagentMarker) {
			return true
		}
	}
	return false
}
