package desk

import (
	"encoding/json"
	"regexp"
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
	// toolParent attributes a tool_result to the subagent whose tool_use it
	// answers: results don't carry parent_tool_use_id themselves.
	toolParent map[string]string
	// taskTool maps a background task to the tool call that started it, for
	// the events (task_updated) that name only the task.
	taskTool map[string]string

	model         string                     // requested model id, from the session init
	calls         map[string]anthropic.Usage // per API call (message id): one call arrives as several assistant events
	contextTokens int64                      // prompt size of the latest main-thread call
}

func newConverter() *converter {
	return &converter{seenTools: map[string]bool{}, toolParent: map[string]string{}, taskTool: map[string]string{}, calls: map[string]anthropic.Usage{}}
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
		entry := c.msg("tool_result", out, m.ToolUseID, payload)
		entry.Parent = c.toolParent[m.ToolUseID]
		entries := []session.Message{entry}
		// A call that started a background task says where its output
		// goes ("Output is being written to: …", "output_file: …"); the
		// task's own events only name that file once it has finished.
		if match := taskOutputRe.FindStringSubmatch(out); match != nil {
			ev, _ := json.Marshal(taskEvent{Event: "output_file", TaskID: match[2], OutputFile: match[1]})
			c.taskTool[match[2]] = m.ToolUseID
			entries = append(entries, c.msg("task", "", m.ToolUseID, ev))
		}
		return entries
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
		if t, ok := c.task(m); ok {
			return []session.Message{t}
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
	// model only through modelUsage: take the one that did the most work,
	// ties broken by id so the pick doesn't follow map order.
	if u.Model == "" {
		most := -1
		for id, mu := range res.ModelUsage {
			if mu.OutputTokens > most || (mu.OutputTokens == most && id < u.Model) {
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
	parent := ""
	if m.ParentToolUseID != nil {
		parent = *m.ParentToolUseID
	}
	if parent == "" {
		usage := m.Message.Usage
		if n := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens; n > 0 {
			c.contextTokens = n
		}
	}
	var out []session.Message
	var text strings.Builder
	flush := func() {
		if t := strings.TrimSpace(text.String()); t != "" {
			out = append(out, session.Message{Role: "assistant", Content: t, Parent: parent, Timestamp: time.Now()})
		}
		text.Reset()
	}
	for _, block := range m.Message.Content {
		switch block.Type {
		case claude.ContentBlockTypeThinking:
			flush()
			if t := strings.TrimSpace(block.Thinking); t != "" {
				entry := c.msg("thinking", t, "", nil)
				entry.Parent = parent
				out = append(out, entry)
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
			c.toolParent[block.ID] = parent
			entry := c.msg("tool_use", block.Name, block.ID, json.RawMessage(block.Input))
			entry.Parent = parent
			out = append(out, entry)
		}
	}
	flush()
	if m.Error != "" {
		entry := c.msg("error", m.Error, "", nil)
		entry.Parent = parent
		out = append(out, entry)
	}
	return out
}

// taskEvent is the payload of a "task" transcript entry: one lifecycle event
// of a subagent or background shell command, as Claude Code reports it in
// its system messages (task_started, task_progress, task_updated,
// task_notification, background_tasks_changed). The entry's RequestID is the
// tool call that started the task, so the page can put its state on that
// call. Only the fields that event carries are set.
type taskEvent struct {
	Event        string      `json:"event"`
	TaskID       string      `json:"task_id,omitempty"`
	TaskType     string      `json:"task_type,omitempty"` // local_agent, local_bash
	Description  string      `json:"description,omitempty"`
	SubagentType string      `json:"subagent_type,omitempty"`
	Background   *bool       `json:"background,omitempty"`
	Status       string      `json:"status,omitempty"` // completed, stopped, failed…
	Summary      string      `json:"summary,omitempty"`
	LastTool     string      `json:"last_tool,omitempty"`
	OutputFile   string      `json:"output_file,omitempty"`
	Usage        *taskUsage  `json:"usage,omitempty"`
	Tasks        []taskBrief `json:"tasks,omitempty"` // background_tasks_changed: the full live set
}

type taskUsage struct {
	TotalTokens int64 `json:"total_tokens"`
	ToolUses    int64 `json:"tool_uses"`
	DurationMS  int64 `json:"duration_ms"`
}

type taskBrief struct {
	TaskID      string `json:"task_id"`
	TaskType    string `json:"task_type,omitempty"`
	Description string `json:"description,omitempty"`
}

// task turns a task lifecycle system message into a "task" entry.
func (c *converter) task(m *claude.SystemMessage) (session.Message, bool) {
	raw := m.Raw
	ev := taskEvent{Event: m.SubType, TaskID: m.TaskID, TaskType: m.TaskType, Description: m.Description}
	switch m.SubType {
	case claude.SystemSubtypeTaskStarted:
		ev.SubagentType, _ = raw["subagent_type"].(string)
		if bg, ok := raw["is_backgrounded"].(bool); ok {
			ev.Background = &bg
		}
		if m.ToolUseID != "" {
			c.taskTool[m.TaskID] = m.ToolUseID
		}
	case claude.SystemSubtypeTaskProgress:
		ev.LastTool, _ = raw["last_tool_name"].(string)
		ev.Usage = taskUsageOf(raw["usage"])
	case claude.SystemSubtypeTaskUpdated:
		if patch, ok := raw["patch"].(map[string]any); ok {
			ev.Status, _ = patch["status"].(string)
		}
	case claude.SystemSubtypeTaskNotification, claude.SystemSubtypeTaskCompleted:
		ev.Status, _ = raw["status"].(string)
		ev.Summary, _ = raw["summary"].(string)
		ev.OutputFile, _ = raw["output_file"].(string)
		ev.Usage = taskUsageOf(raw["usage"])
	case claude.SystemSubtypeBackgroundTasksChanged:
		ev.Tasks = []taskBrief{}
		if list, ok := raw["tasks"].([]any); ok {
			for _, item := range list {
				if t, ok := item.(map[string]any); ok {
					b := taskBrief{}
					b.TaskID, _ = t["task_id"].(string)
					b.TaskType, _ = t["task_type"].(string)
					b.Description, _ = t["description"].(string)
					ev.Tasks = append(ev.Tasks, b)
				}
			}
		}
	default:
		return session.Message{}, false
	}
	toolUseID := m.ToolUseID
	if toolUseID == "" {
		toolUseID = c.taskTool[m.TaskID]
	}
	payload, _ := json.Marshal(ev)
	content := ev.Summary
	if content == "" {
		content = ev.Description
	}
	return c.msg("task", content, toolUseID, payload), true
}

// taskOutputRe finds a background task's output file in the result of the
// call that started it: /…/tasks/<task_id>.output.
var taskOutputRe = regexp.MustCompile(`(/\S*/tasks/([A-Za-z0-9_-]+)\.output)`)

func taskUsageOf(v any) *taskUsage {
	u, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	num := func(k string) int64 {
		f, _ := u[k].(float64)
		return int64(f)
	}
	return &taskUsage{TotalTokens: num("total_tokens"), ToolUses: num("tool_uses"), DurationMS: num("duration_ms")}
}

func (c *converter) msg(kind, text, reqID string, payload json.RawMessage) session.Message {
	if len(payload) == 0 || string(payload) == "null" {
		payload = nil
	}
	return session.Message{Kind: kind, Content: text, RequestID: reqID, Payload: payload, Timestamp: time.Now()}
}
