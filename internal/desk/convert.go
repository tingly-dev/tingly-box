package desk

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// maxToolOutput caps a tool result stored in the transcript; the full
// output stays in Claude Code's own on-disk session file.
const maxToolOutput = 4 * 1024

// converter turns agentboot's Claude messages into transcript entries. It
// dedupes tool_use blocks, which can arrive both inside an assistant
// message and as a standalone message depending on the CLI version.
type converter struct {
	seenTools map[string]bool
}

func newConverter() *converter { return &converter{seenTools: map[string]bool{}} }

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
		if m.IsError && m.Result != "" {
			return []session.Message{c.msg("error", m.Result, "", nil)}
		}
		return nil
	case *claude.SystemMessage:
		if m.SubType == claude.SystemSubtypeInit {
			return []session.Message{c.msg("system", "claude code session "+m.SessionID, "", nil)}
		}
		return nil
	}
	return nil
}

func (c *converter) assistant(m *claude.AssistantMessage) []session.Message {
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
