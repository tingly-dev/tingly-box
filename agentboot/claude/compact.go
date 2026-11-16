package claude

import (
	"errors"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Compaction and context size, as Claude Code 2.1.259 reports them (see
// .research/slash-commands-claude-code.md). A compaction is the user message
// "/compact [instructions]"; its progress arrives as
//
//	system/status{status:"compacting"}
//	system/status{status:null, compact_result:"success"|"failed", compact_error}
//	system/compact_boundary{compact_metadata:{trigger, pre_tokens, post_tokens, duration_ms}}
//	user{isSynthetic:true}   (the summary the conversation continues from)
//
// An automatic compaction emits the same events inside an ordinary turn.

const (
	systemSubtypeStatus          = "status"
	systemSubtypeCompactBoundary = "compact_boundary"

	statusCompacting    = "compacting"
	compactResultFailed = "failed"

	// ControlRequestSubtypeGetContextUsage answers with the context size.
	ControlRequestSubtypeGetContextUsage = "get_context_usage"
)

// CompactPrompt implements [agentboot.Compactor]: the user message that makes
// Claude Code compact, with optional instructions for the summary. It is on the
// agent, which is where the engine looks before any process exists.
func (a *Agent) CompactPrompt(instructions string) string {
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		return "/compact " + instructions
	}
	return "/compact"
}

// ControlResponseBody implements [agentboot.ControlResponseBodyDecoder].
func (t *Transport) ControlResponseBody(ev protocol.Event) map[string]any {
	resp, _ := ev.Data["response"].(map[string]any)
	body, _ := resp["response"].(map[string]any)
	return body
}

// DecodeContextUsage implements [agentboot.ContextUsageReader].
func (t *Transport) DecodeContextUsage(body map[string]any) (agentboot.ContextUsage, error) {
	used, okUsed := number(body["totalTokens"])
	max, okMax := number(body["maxTokens"])
	if !okUsed || !okMax {
		return agentboot.ContextUsage{}, errors.New("claude: get_context_usage answer has no totalTokens/maxTokens")
	}
	threshold, _ := number(body["autoCompactThreshold"])
	if enabled, ok := body["isAutoCompactEnabled"].(bool); ok && !enabled {
		threshold = 0
	}
	return agentboot.ContextUsage{Used: used, Max: max, AutoCompactAt: threshold}, nil
}

func number(v any) (int64, bool) {
	f, ok := v.(float64)
	return int64(f), ok
}

// compactTracker turns the compaction events of a stream into neutral
// [agentboot.CompactEvent]s. The boundary is held back until the summary
// message that follows it (or the first other event) so the done event can
// carry the summary.
type compactTracker struct {
	pending *agentboot.CompactEvent
}

func (c *compactTracker) observe(ev protocol.Event) []any {
	var out []any
	if c.pending != nil {
		if summary, ok := syntheticSummary(ev); ok {
			c.pending.Result.Summary = summary
			c.pending.Result.Reported.Summary = true
		}
		out = append(out, *c.pending)
		wasSummary := c.pending.Result.Reported.Summary
		c.pending = nil
		if wasSummary {
			return out
		}
	}
	if ev.Type != SDKSystemMessage {
		return out
	}
	switch subtype, _ := ev.Data["subtype"].(string); subtype {
	case systemSubtypeStatus:
		if status, _ := ev.Data["status"].(string); status == statusCompacting {
			out = append(out, agentboot.CompactEvent{Phase: agentboot.CompactStarted})
		}
		if result, _ := ev.Data["compact_result"].(string); result == compactResultFailed {
			reason, _ := ev.Data["compact_error"].(string)
			out = append(out, agentboot.CompactEvent{Phase: agentboot.CompactFailed, Reason: reason})
		}
	case systemSubtypeCompactBoundary:
		c.pending = boundaryEvent(ev)
	}
	return out
}

func boundaryEvent(ev protocol.Event) *agentboot.CompactEvent {
	meta, _ := ev.Data["compact_metadata"].(map[string]any)
	res := agentboot.CompactResult{Trigger: agentboot.CompactManual}
	if trigger, _ := meta["trigger"].(string); trigger == string(agentboot.CompactAuto) {
		res.Trigger = agentboot.CompactAuto
	}
	before, okBefore := number(meta["pre_tokens"])
	after, okAfter := number(meta["post_tokens"])
	res.TokensBefore, res.TokensAfter, res.Reported.Tokens = before, after, okBefore && okAfter
	if ms, ok := number(meta["duration_ms"]); ok {
		res.DurationMS = ms
	}
	return &agentboot.CompactEvent{Phase: agentboot.CompactDone, Result: res}
}

// syntheticSummary reads the text of the synthetic user message Claude Code
// inserts after a compaction.
func syntheticSummary(ev protocol.Event) (string, bool) {
	if ev.Type != SDKUserMessage {
		return "", false
	}
	if synthetic, _ := ev.Data["isSynthetic"].(bool); !synthetic {
		return "", false
	}
	msg, _ := ev.Data["message"].(map[string]any)
	if text, ok := msg["content"].(string); ok {
		return text, true
	}
	blocks, _ := msg["content"].([]any)
	var parts []string
	for _, b := range blocks {
		if block, ok := b.(map[string]any); ok {
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n"), len(parts) > 0
}
