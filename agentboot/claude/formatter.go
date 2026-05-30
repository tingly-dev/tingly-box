package claude

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// Formatter converts messages to structured text
type Formatter interface {
	Format(msg Message) string
}

// TextFormatter implements Formatter using built-in formatting
type TextFormatter struct {
	IncludeTimestamp bool
	Verbose          bool
	ShowToolDetails  bool
	mu               sync.RWMutex
	// toolNameByID correlates tool_use IDs to tool names so that result
	// rendering can dispatch to the right per-tool renderer. Entries are
	// removed once the corresponding result is rendered.
	toolNameByID map[string]string
	// seenAssistantToolIDs records IDs of tool_use blocks already rendered
	// inside an AssistantMessage so the SDK's standalone *ToolUseMessage
	// duplicate is suppressed (returns "" and is dropped by the caller).
	seenAssistantToolIDs map[string]struct{}
}

// NewTextFormatter creates a new text formatter
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{}
}

func (f *TextFormatter) rememberToolName(id, name string) {
	if id == "" || name == "" {
		return
	}
	f.mu.Lock()
	if f.toolNameByID == nil {
		f.toolNameByID = map[string]string{}
	}
	f.toolNameByID[id] = name
	f.mu.Unlock()
}

func (f *TextFormatter) lookupToolName(id string) string {
	if id == "" {
		return ""
	}
	f.mu.RLock()
	name := f.toolNameByID[id]
	f.mu.RUnlock()
	return name
}

func (f *TextFormatter) consumeToolName(id string) string {
	if id == "" {
		return ""
	}
	f.mu.Lock()
	name := f.toolNameByID[id]
	if name != "" {
		delete(f.toolNameByID, id)
	}
	f.mu.Unlock()
	return name
}

func (f *TextFormatter) markAssistantToolID(id string) {
	if id == "" {
		return
	}
	f.mu.Lock()
	if f.seenAssistantToolIDs == nil {
		f.seenAssistantToolIDs = map[string]struct{}{}
	}
	f.seenAssistantToolIDs[id] = struct{}{}
	f.mu.Unlock()
}

func (f *TextFormatter) wasAssistantToolID(id string) bool {
	if id == "" {
		return false
	}
	f.mu.RLock()
	_, ok := f.seenAssistantToolIDs[id]
	f.mu.RUnlock()
	return ok
}

// Format formats a message
func (f *TextFormatter) Format(msg Message) string {
	if msg == nil {
		return ""
	}

	switch m := msg.(type) {
	case *SystemMessage:
		return f.formatSystem(m)
	case *AssistantMessage:
		return f.formatAssistant(m)
	case *UserMessage:
		return f.formatUser(m)
	case *ToolUseMessage:
		return f.formatToolUse(m)
	case *ToolResultMessage:
		return f.formatToolResult(m)
	case *StreamEventMessage:
		return f.formatStreamEvent(m)
	case *ResultMessage:
		return f.formatResult(m)
	default:
		return fmt.Sprintf("[UNKNOWN] %s", msg.GetType())
	}
}

func (f *TextFormatter) formatSystem(m *SystemMessage) string {
	switch m.SubType {
	case SDKTaskStartedMessage:
		return f.formatTaskStarted(m)
	case SystemSubtypeTaskCompleted:
		return f.formatTaskCompleted(m)
	case SDKTaskNotificationMessage:
		return f.formatTaskNotification(m)
	case SystemSubtypeInit:
		var b strings.Builder
		b.WriteString("[SYSTEM] ")
		b.WriteString(m.SubType)
		b.WriteString(" Session: ")
		b.WriteString(m.SessionID)
		if f.IncludeTimestamp && !m.Timestamp.IsZero() {
			b.WriteString(" at ")
			b.WriteString(m.Timestamp.Format("2006-01-02 15:04:05"))
		}
		return b.String()
	case SystemSubtypeAPIRetry:
		return f.formatRetryNotice(m, "Retrying API request")
	case SystemSubtypeRateLimit:
		return f.formatRetryNotice(m, "Rate limited, waiting")
	default:
		logrus.Debugf("system message, subtype: %s", m.SubType)
		return ""
	}
}

// formatRetryNotice renders an api_retry/rate_limit system notice. The CLI
// emits these while an upstream call is being retried; surfacing them tells the
// user why the agent is pausing instead of leaving a silent gap. lead is the
// human-readable action ("Retrying API request", "Rate limited, waiting").
func (f *TextFormatter) formatRetryNotice(m *SystemMessage, lead string) string {
	var b strings.Builder
	b.WriteString("[RETRY] ")
	b.WriteString(lead)
	if n := m.retryAttempt(); n > 0 {
		fmt.Fprintf(&b, " (attempt %d)", n)
	}
	if d := m.retryDelayMS(); d > 0 {
		fmt.Fprintf(&b, ", retry in %s", formatRetryDelay(d))
	}
	if reason := m.retryReason(); reason != "" {
		b.WriteString(": ")
		b.WriteString(reason)
	}
	return b.String()
}

// formatRetryDelay renders a millisecond delay as a compact human string.
func formatRetryDelay(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func (f *TextFormatter) formatTaskStarted(m *SystemMessage) string {
	var b strings.Builder
	b.WriteString("[SUBAGENT] ")
	if m.Description != "" {
		b.WriteString(m.Description)
	} else if m.Prompt != "" {
		prompt := m.Prompt
		if len(prompt) > 100 {
			prompt = prompt[:100] + "..."
		}
		b.WriteString(prompt)
	}
	if m.TaskType != "" {
		b.WriteString(" (")
		b.WriteString(m.TaskType)
		b.WriteString(")")
	}
	return b.String()
}

func (f *TextFormatter) formatTaskCompleted(m *SystemMessage) string {
	var b strings.Builder
	b.WriteString("[SUBAGENT DONE]")
	if m.Description != "" {
		b.WriteString(" ")
		b.WriteString(m.Description)
	}
	return b.String()
}

func (f *TextFormatter) formatTaskNotification(m *SystemMessage) string {
	var b strings.Builder
	b.WriteString("[SUBAGENT] Done")
	if m.Description != "" {
		b.WriteString("\n")
		b.WriteString(m.Description)
	}
	return b.String()
}

func (f *TextFormatter) formatAssistant(m *AssistantMessage) string {
	var sections []string
	var tools []ToolUseRef

	flushTools := func() {
		if len(tools) == 0 {
			return
		}
		if rendered := renderToolUseGroup(tools, f.ShowToolDetails); rendered != "" {
			sections = append(sections, rendered)
		}
		tools = tools[:0]
	}

	for _, content := range m.Message.Content {
		switch content.Type {
		case ContentBlockTypeText:
			flushTools()
			if content.Text != "" {
				sections = append(sections, strings.TrimRight(content.Text, "\n"))
			}
		case ContentBlockTypeToolUse:
			f.rememberToolName(content.ID, content.Name)
			f.markAssistantToolID(content.ID)
			tools = append(tools, ToolUseRef{
				ID:    content.ID,
				Name:  content.Name,
				Input: inputFromRaw(content.Input),
			})
		case ContentBlockTypeThinking:
			flushTools()
			if f.Verbose && content.Thinking != "" {
				sections = append(sections, "[THINKING] "+content.Thinking)
			}
		}
	}
	flushTools()

	if m.IsError() {
		sections = append(sections, fmt.Sprintf("[ASSISTANT ERROR: %s]", m.Error))
	}

	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n")
}

func (f *TextFormatter) formatUser(m *UserMessage) string {
	if m.Message == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("[USER] ")
	b.WriteString(m.Message)
	if m.ParentToolUseID != nil && *m.ParentToolUseID != "" {
		b.WriteString(" (in reply to ")
		b.WriteString(*m.ParentToolUseID)
		b.WriteString(")")
	}
	return b.String()
}

func (f *TextFormatter) formatToolUse(m *ToolUseMessage) string {
	if m == nil {
		return ""
	}
	// Suppress duplicates: if the same tool_use ID was already rendered as
	// part of an AssistantMessage bundle, drop this standalone event.
	if f.wasAssistantToolID(m.ToolUseID) {
		return ""
	}
	f.rememberToolName(m.ToolUseID, m.Name)
	return renderToolUse(m.Name, m.Input, f.ShowToolDetails)
}

func (f *TextFormatter) formatToolResult(m *ToolResultMessage) string {
	if m == nil {
		return ""
	}
	name := f.consumeToolName(m.ToolUseID)
	return renderToolResult(name, m)
}

func (f *TextFormatter) formatStreamEvent(m *StreamEventMessage) string {
	var b strings.Builder
	b.WriteString("[STREAM]")
	if m.Event.Type != "" {
		b.WriteString(" ")
		b.WriteString(m.Event.Type)
	}

	if m.Event.Delta != nil {
		switch delta := m.Event.Delta.(type) {
		case *TextDelta:
			b.WriteString(" +")
			b.WriteString(delta.Text)
		case *InputJSONDelta:
			b.WriteString(" +JSON: ")
			b.WriteString(delta.PartialJSON)
		}
	}
	return b.String()
}

func (f *TextFormatter) formatResult(m *ResultMessage) string {
	var b strings.Builder
	b.WriteString("[RESULT] ")
	if m.IsError {
		b.WriteString("ERROR")
	} else {
		b.WriteString("SUCCESS")
	}

	if m.DurationMS > 0 {
		b.WriteString("\nDuration: ")
		b.WriteString(fmt.Sprintf("%dms", m.DurationMS))
		if m.DurationAPIMS > 0 {
			b.WriteString(" (API: ")
			b.WriteString(fmt.Sprintf("%dms", m.DurationAPIMS))
			b.WriteString(")")
		}
	}

	if m.TotalCostUSD > 0 {
		b.WriteString("\nCost: $")
		b.WriteString(fmt.Sprintf("%.4f", m.TotalCostUSD))
	}

	if m.Usage.InputTokens > 0 || m.Usage.OutputTokens > 0 {
		b.WriteString("\nTokens: ")
		b.WriteString(fmt.Sprintf("%d", m.Usage.InputTokens))
		b.WriteString(" in, ")
		b.WriteString(fmt.Sprintf("%d", m.Usage.OutputTokens))
		b.WriteString(" out")
		if m.Usage.CacheReadInputTokens > 0 {
			b.WriteString(" (cache: ")
			b.WriteString(fmt.Sprintf("%d", m.Usage.CacheReadInputTokens))
			b.WriteString(")")
		}
	}

	// FIXME: since last assistant return result, we do not repeat here
	//if m.Result != "" {
	//	b.WriteString("\n")
	//	b.WriteString(m.Result)
	//}

	if len(m.PermissionDenials) > 0 {
		b.WriteString("\nPermission Denials:")
		for _, pd := range m.PermissionDenials {
			b.WriteString("\n  - ")
			b.WriteString(pd.RequestID)
			b.WriteString(": ")
			b.WriteString(pd.Reason)
		}
	}

	return b.String()
}

// SetIncludeTimestamp sets whether to include timestamps in output
func (f *TextFormatter) SetIncludeTimestamp(include bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.IncludeTimestamp = include
}

// SetVerbose sets verbose mode for detailed output
func (f *TextFormatter) SetVerbose(verbose bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Verbose = verbose
}

// SetShowToolDetails sets whether to show tool details
func (f *TextFormatter) SetShowToolDetails(show bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ShowToolDetails = show
}
