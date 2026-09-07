package agentrun

import (
	"encoding/json"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// maxToolOutput caps a tool result stored in the log; the full output stays
// in Claude Code's own session file.
const maxToolOutput = 4 * 1024

// converter turns agentboot's Claude messages into session events. It
// dedupes tool_use blocks, which can arrive both inside an assistant message
// and as a standalone message depending on the CLI version.
type converter struct {
	sessionID string
	seenTools map[string]bool
}

func newConverter(sessionID string) *converter {
	return &converter{sessionID: sessionID, seenTools: map[string]bool{}}
}

func (c *converter) message(raw any) []managedagent.Event {
	switch m := raw.(type) {
	case *claude.AssistantMessage:
		return c.assistant(m)
	case *claude.ToolUseMessage:
		if c.seenTools[m.ToolUseID] {
			return nil
		}
		c.seenTools[m.ToolUseID] = true
		payload, _ := json.Marshal(m.Input)
		return []managedagent.Event{c.ev(managedagent.EventToolUse, m.Name, m.ToolUseID, payload)}
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
		return []managedagent.Event{c.ev(managedagent.EventToolResult, out, m.ToolUseID, payload)}
	case *claude.ResultMessage:
		// Terminal for the transport; usage is folded from Result.Events
		// after Wait (see foldResult). Kept for completeness if a future
		// transport forwards it.
		if m.IsError && m.Result != "" {
			return []managedagent.Event{c.ev(managedagent.EventError, m.Result, "", nil)}
		}
		return nil
	case *claude.SystemMessage:
		if m.SubType == claude.SystemSubtypeInit {
			return []managedagent.Event{c.ev(managedagent.EventSystem, "claude code session "+m.SessionID, "", nil)}
		}
		return nil
	}
	return nil
}

func (c *converter) assistant(m *claude.AssistantMessage) []managedagent.Event {
	var out []managedagent.Event
	var text strings.Builder
	flush := func() {
		if t := strings.TrimSpace(text.String()); t != "" {
			out = append(out, c.ev(managedagent.EventAssistantMessage, t, "", nil))
		}
		text.Reset()
	}
	for _, block := range m.Message.Content {
		switch block.Type {
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
			out = append(out, c.ev(managedagent.EventToolUse, block.Name, block.ID, json.RawMessage(block.Input)))
		}
	}
	flush()
	if m.Error != "" {
		out = append(out, c.ev(managedagent.EventError, m.Error, "", nil))
	}
	return out
}

func (c *converter) ev(kind managedagent.EventKind, text, reqID string, payload json.RawMessage) managedagent.Event {
	if len(payload) == 0 || string(payload) == "null" {
		payload = nil
	}
	return managedagent.Event{SessionID: c.sessionID, Kind: kind, Text: text, RequestID: reqID, Payload: payload}
}
