package desk

import (
	"encoding/json"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// maxToolOutput caps a tool result stored in the transcript; the full
// output stays in Claude Code's own on-disk session file.
const maxToolOutput = 4 * 1024

// converter turns agentboot's Claude messages into transcript entries. It
// dedupes tool_use blocks, which can arrive both inside an assistant
// message and as a standalone message depending on the CLI version.
//
// It also tallies the turn's token usage for the "usage" entry written when
// the turn ends (see turnUsage).
type converter struct {
	seenTools map[string]bool

	model         string                     // requested model id, from the session init
	calls         map[string]anthropic.Usage // per API call (message id): one call arrives as several assistant events
	contextTokens int64                      // prompt size of the latest main-thread call
}

func newConverter() *converter {
	return &converter{seenTools: map[string]bool{}, calls: map[string]anthropic.Usage{}}
}

// turnUsage is the payload of a "usage" transcript entry: one per turn.
// Token counts are summed over the turn's API calls as the gateway returned
// them. Claude Code's own cost figure is left out: it prices every call at
// Anthropic list prices, which is wrong whenever a profile routes elsewhere.
type turnUsage struct {
	Model            string `json:"model,omitempty"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	// ContextTokens is how full the context was on the turn's last
	// main-thread call; ContextWindow is the model's window, when known.
	ContextTokens int64 `json:"context_tokens"`
	ContextWindow int   `json:"context_window,omitempty"`
	DurationMS    int64 `json:"duration_ms,omitempty"`
}

func (c *converter) messages(raw any) []session.Message {
	switch m := raw.(type) {
	case *claude.AssistantMessage:
		return c.assistant(m)
	case *claude.ToolUseMessage:
		if c.seenTools[m.ToolUseID] {
			return nil
		}
		c.seenTools[m.ToolUseID] = true
		payload, _ := json.Marshal(m.Input)
		return []session.Message{c.msg("tool_use", m.Name, m.ToolUseID, payload)}
	case *claude.ToolResultMessage:
		out := m.Output
		if out == "" && len(m.Content) > 0 {
			var parts []string
			for _, b := range m.Content {
				if tb, ok := b.(*claude.TextBlock); ok && tb.Text != "" {
					parts = append(parts, tb.Text)
				}
			}
			out = strings.Join(parts, "\n")
		}
		if len(out) > maxToolOutput {
			out = out[:maxToolOutput] + "\n… (truncated)"
		}
		payload, _ := json.Marshal(map[string]any{"is_error": m.IsError})
		return []session.Message{c.msg("tool_result", out, m.ToolUseID, payload)}
	case *claude.ResultMessage:
		var out []session.Message
		if m.IsError && m.Result != "" {
			out = append(out, c.msg("error", m.Result, "", nil))
		}
		if u, ok := c.usage(m); ok {
			payload, _ := json.Marshal(u)
			out = append(out, c.msg("usage", "", "", payload))
		}
		return out
	case *claude.SystemMessage:
		if m.SubType == claude.SystemSubtypeInit {
			if model, ok := m.Raw["model"].(string); ok {
				c.model = model
			}
			return []session.Message{c.msg("system", "claude code session "+m.SessionID, "", nil)}
		}
		return nil
	}
	return nil
}

// usage totals the turn's API calls. It reports nothing for a turn that made
// no call (one that failed before reaching the model).
func (c *converter) usage(res *claude.ResultMessage) (turnUsage, bool) {
	if len(c.calls) == 0 {
		return turnUsage{}, false
	}
	u := turnUsage{Model: c.model, ContextTokens: c.contextTokens, DurationMS: res.DurationMS}
	for _, call := range c.calls {
		u.InputTokens += call.InputTokens
		u.OutputTokens += call.OutputTokens
		u.CacheReadTokens += call.CacheReadInputTokens
		u.CacheWriteTokens += call.CacheCreationInputTokens
	}
	// A resident process sends its init once, so later turns name the main
	// model only through modelUsage: take the one that did the most work.
	if u.Model == "" {
		var most int
		for id, mu := range res.ModelUsage {
			if mu.OutputTokens >= most {
				u.Model, most = id, mu.OutputTokens
			}
		}
	}
	u.ContextWindow = res.ModelUsage[u.Model].ContextWindow
	return u, true
}

func (c *converter) assistant(m *claude.AssistantMessage) []session.Message {
	if id := m.Message.ID; id != "" {
		c.calls[id] = m.Message.Usage
	}
	if m.ParentToolUseID == nil {
		usage := m.Message.Usage
		if n := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens; n > 0 {
			c.contextTokens = n
		}
	}
	var out []session.Message
	var text strings.Builder
	flush := func() {
		if t := strings.TrimSpace(text.String()); t != "" {
			out = append(out, session.Message{Role: "assistant", Content: t, Timestamp: time.Now()})
		}
		text.Reset()
	}
	for _, block := range m.Message.Content {
		switch block.Type {
		case claude.ContentBlockTypeThinking:
			flush()
			if t := strings.TrimSpace(block.Thinking); t != "" {
				out = append(out, c.msg("thinking", t, "", nil))
			}
		case claude.ContentBlockTypeText:
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(block.Text)
		case claude.ContentBlockTypeToolUse, claude.ContentBlockTypeServerToolUse:
			flush()
			if c.seenTools[block.ID] {
				continue
			}
			c.seenTools[block.ID] = true
			out = append(out, c.msg("tool_use", block.Name, block.ID, json.RawMessage(block.Input)))
		}
	}
	flush()
	if m.Error != "" {
		out = append(out, c.msg("error", m.Error, "", nil))
	}
	return out
}

func (c *converter) msg(kind, text, reqID string, payload json.RawMessage) session.Message {
	if len(payload) == 0 || string(payload) == "null" {
		payload = nil
	}
	return session.Message{Kind: kind, Content: text, RequestID: reqID, Payload: payload, Timestamp: time.Now()}
}
