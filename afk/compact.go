package afk

import (
	"github.com/anthropics/anthropic-sdk-go/v2"
)

// Server-side compaction, from the Beta Messages API.
//
// Once the prompt crosses the trigger, the API summarizes the older part of the
// conversation itself and returns a compaction block in the response. We keep
// sending the full history — the block is what tells the API which part of it to
// replace on the next request — so nothing here rewrites or drops messages.
//
// This is the whole mechanism. There is no in-process summarizer: doing it
// ourselves meant an extra model call per compaction, a summary written by a
// prompt we maintain, and a boundary-selection rule that had to avoid cutting a
// tool_result away from its tool_use. All of that is the platform's job.
const (
	// betaCompaction enables it. Passed as a literal because the vendored SDK
	// ships the request types (BetaCompact20260112EditParam) ahead of a named
	// constant for the flag; AnthropicBeta is an alias for string, so this is
	// the same thing a constant would be.
	betaCompaction = "compact-2026-01-12"

	// DefaultServerCompactTrigger is the input-token count at which the API
	// compacts. Set explicitly rather than inheriting the API's own default so
	// the number @tb runs at is visible here and does not move underneath us.
	DefaultServerCompactTrigger = 120_000
)

// compactionInstructions steers what the summary keeps. The default summary is
// written for a reader resuming the conversation; these are the specifics a
// tool-using agent needs and would otherwise have to re-derive.
const compactionInstructions = `Preserve what the user asked for and why, decisions and conclusions reached,
files and directories touched, commands run and their outcomes, current working
state, and anything still outstanding. Drop pleasantries and redundant tool output.`

// serverCompactionEdit builds the context-management edit that asks the API to
// compact on our behalf.
func serverCompactionEdit(trigger int64) anthropic.BetaContextManagementConfigParam {
	return anthropic.BetaContextManagementConfigParam{
		Edits: []anthropic.BetaContextManagementConfigEditUnionParam{{
			OfCompact20260112: &anthropic.BetaCompact20260112EditParam{
				Instructions: anthropic.String(compactionInstructions),
				Trigger:      anthropic.BetaInputTokensTriggerParam{Value: trigger},
				// PauseAfterCompaction stays false: compaction should be
				// invisible to the run, not a step that has to be resumed.
			},
		}},
	}
}

// serverCompactTrigger returns the configured trigger, or the default.
func (e *Engine) serverCompactTrigger() int64 {
	if e.serverTrigger > 0 {
		return e.serverTrigger
	}
	return DefaultServerCompactTrigger
}
