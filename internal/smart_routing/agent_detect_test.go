package smartrouting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectClaudeCodeRequestKind_Main(t *testing.T) {
	ctx := &RequestContext{
		SystemMessages: []string{"You are Claude Code, Anthropic's official CLI for Claude. Help the user with software engineering."},
		UserMessages:   []string{"refactor this function"},
	}
	require.Equal(t, ClaudeCodeKindMain, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_Subagent(t *testing.T) {
	ctx := &RequestContext{
		SystemMessages: []string{"You are an agent for a coding assistant. Investigate this question and report back."},
		UserMessages:   []string{"find all callers of foo()"},
	}
	require.Equal(t, ClaudeCodeKindSubagent, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_Compact(t *testing.T) {
	ctx := &RequestContext{
		SystemMessages: []string{"Your task is to create a detailed summary of the conversation so far."},
		UserMessages:   []string{"<conversation history dump>"},
	}
	require.Equal(t, ClaudeCodeKindCompact, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_CompactInUserMessage(t *testing.T) {
	// Some compact templates carry the marker in the user message instead of system.
	ctx := &RequestContext{
		SystemMessages: []string{},
		UserMessages:   []string{"Please produce a summary of the conversation that captures key decisions."},
	}
	require.Equal(t, ClaudeCodeKindCompact, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_EmptyContext(t *testing.T) {
	ctx := &RequestContext{}
	require.Equal(t, ClaudeCodeKindMain, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_NilContext(t *testing.T) {
	require.Equal(t, ClaudeCodeKindMain, DetectClaudeCodeRequestKind(nil, nil))
}

func TestDetectClaudeCodeRequestKind_PriorityCompactOverSubagent(t *testing.T) {
	// Both markers present — compact wins (most specific).
	ctx := &RequestContext{
		SystemMessages: []string{
			"You are an agent. Now your task is to produce a summary of the conversation.",
		},
	}
	require.Equal(t, ClaudeCodeKindCompact, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_MainPreambleSuppressesSubagent(t *testing.T) {
	// If the main Claude Code preamble is present, "You are an agent" elsewhere
	// in the system messages should NOT trigger subagent classification (the
	// main agent commonly references subagents in its tool descriptions).
	ctx := &RequestContext{
		SystemMessages: []string{
			"You are Claude Code, Anthropic's official CLI for Claude.",
			"You can dispatch tasks to a subagent: 'You are an agent...'",
		},
	}
	require.Equal(t, ClaudeCodeKindMain, DetectClaudeCodeRequestKind(ctx, nil))
}

func TestDetectClaudeCodeRequestKind_CustomMarkers(t *testing.T) {
	cfg := &ClaudeCodeDetectConfig{
		MainPreamble:   "You are MyBot",
		SubagentMarker: "You are a sub-bot",
		CompactMarker:  "compact summary requested",
	}

	t.Run("custom main preamble", func(t *testing.T) {
		ctx := &RequestContext{
			SystemMessages: []string{"You are MyBot, helping the user."},
		}
		require.Equal(t, ClaudeCodeKindMain, DetectClaudeCodeRequestKind(ctx, cfg))
	})

	t.Run("custom subagent marker", func(t *testing.T) {
		ctx := &RequestContext{
			SystemMessages: []string{"You are a sub-bot assigned to this task."},
		}
		require.Equal(t, ClaudeCodeKindSubagent, DetectClaudeCodeRequestKind(ctx, cfg))
	})

	t.Run("custom compact marker", func(t *testing.T) {
		ctx := &RequestContext{
			SystemMessages: []string{"compact summary requested for this session"},
		}
		require.Equal(t, ClaudeCodeKindCompact, DetectClaudeCodeRequestKind(ctx, cfg))
	})

	t.Run("default markers no longer match with custom config", func(t *testing.T) {
		// Default "You are Claude Code" should NOT suppress subagent when a custom
		// main preamble is configured.
		ctx := &RequestContext{
			SystemMessages: []string{
				"You are Claude Code, Anthropic's official CLI.",
				"You are a sub-bot assigned to a task.",
			},
		}
		// "You are Claude Code" != cfg.MainPreamble, so subagent check runs and matches.
		require.Equal(t, ClaudeCodeKindSubagent, DetectClaudeCodeRequestKind(ctx, cfg))
	})
}
