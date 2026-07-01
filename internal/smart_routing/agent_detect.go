package smartrouting

import "strings"

// Claude Code request kinds. These are the values emitted by
// DetectClaudeCodeRequestKind and accepted by the agent.claude_code SmartOp.
const (
	ClaudeCodeKindMain     = "main"
	ClaudeCodeKindSubagent = "subagent"
	ClaudeCodeKindCompact  = "compact"
)

// Default signal fragments used to fingerprint Claude Code request types.
// Sourced from Claude Code CLI prompt templates as of 2026-05.
const (
	defaultClaudeCodeCompactMarker  = "summary of the conversation"
	defaultClaudeCodeMainPreamble   = "You are Claude Code"
	defaultClaudeCodeSubagentMarker = "You are an agent"
)

// ClaudeCodeDetectConfig holds optional overrides for the string fragments used
// to identify Claude Code request kinds. An empty field means "use the built-in
// default", so callers only need to set the fields they want to change.
type ClaudeCodeDetectConfig struct {
	// CompactMarker is the substring searched in system/user messages to
	// identify a /compact summary request.
	// Default: "summary of the conversation"
	CompactMarker string `json:"compact_marker,omitempty" yaml:"compact_marker,omitempty"`

	// MainPreamble is the substring that marks the primary Claude Code agent
	// system prompt. Its presence also suppresses subagent classification.
	// Default: "You are Claude Code"
	MainPreamble string `json:"main_preamble,omitempty" yaml:"main_preamble,omitempty"`

	// SubagentMarker is the substring searched in system messages to identify
	// a Task-tool sub-agent request (only matched when MainPreamble is absent).
	// Default: "You are an agent"
	SubagentMarker string `json:"subagent_marker,omitempty" yaml:"subagent_marker,omitempty"`
}

func (c *ClaudeCodeDetectConfig) compactMarker() string {
	if c != nil && c.CompactMarker != "" {
		return c.CompactMarker
	}
	return defaultClaudeCodeCompactMarker
}

func (c *ClaudeCodeDetectConfig) mainPreamble() string {
	if c != nil && c.MainPreamble != "" {
		return c.MainPreamble
	}
	return defaultClaudeCodeMainPreamble
}

func (c *ClaudeCodeDetectConfig) subagentMarker() string {
	if c != nil && c.SubagentMarker != "" {
		return c.SubagentMarker
	}
	return defaultClaudeCodeSubagentMarker
}

// DetectClaudeCodeRequestKind inspects an already-built RequestContext and
// returns one of the ClaudeCodeKind* values. Callers must ensure the request
// scenario is claude_code before invoking; for other scenarios the result is
// not meaningful.
//
// cfg overrides the default detection markers; nil uses the built-in defaults.
// Precedence is compact → subagent → main (most-specific first).
func DetectClaudeCodeRequestKind(ctx *RequestContext, cfg *ClaudeCodeDetectConfig) string {
	if ctx == nil {
		return ClaudeCodeKindMain
	}
	if isClaudeCodeCompactRequest(ctx, cfg) {
		return ClaudeCodeKindCompact
	}
	if isClaudeCodeSubagentRequest(ctx, cfg) {
		return ClaudeCodeKindSubagent
	}
	return ClaudeCodeKindMain
}

func isClaudeCodeCompactRequest(ctx *RequestContext, cfg *ClaudeCodeDetectConfig) bool {
	marker := cfg.compactMarker()
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, marker) {
			return true
		}
	}
	for _, user := range ctx.UserMessages {
		if strings.Contains(user, marker) {
			return true
		}
	}
	return false
}

func isClaudeCodeSubagentRequest(ctx *RequestContext, cfg *ClaudeCodeDetectConfig) bool {
	preamble := cfg.mainPreamble()
	marker := cfg.subagentMarker()
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, preamble) {
			return false
		}
	}
	for _, sys := range ctx.SystemMessages {
		if strings.Contains(sys, marker) {
			return true
		}
	}
	return false
}
