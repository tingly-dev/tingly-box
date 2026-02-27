package claude

import (
	"embed"
	"fmt"
	"strings"
	"sync"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Formatter converts messages to structured text
type Formatter interface {
	Format(msg Message) string
}

// TextFormatter implements Formatter using Go templates
type TextFormatter struct {
	IncludeTimestamp bool
	Verbose          bool
	ShowToolDetails  bool
	customTemplates  map[string]*template.Template
	mu               sync.RWMutex
}

// NewTextFormatter creates a new text formatter with default templates
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{
		customTemplates: make(map[string]*template.Template),
	}
}

// Format formats a message using sprintf for better performance
func (f *TextFormatter) Format(msg Message) string {
	if msg == nil {
		return ""
	}

	// Check for custom template first
	f.mu.RLock()
	customTmpl, hasCustom := f.customTemplates[msg.GetType()]
	f.mu.RUnlock()

	if hasCustom {
		return f.formatWithTemplate(customTmpl, msg)
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

// formatWithTemplate formats a message using a template (for custom templates)
func (f *TextFormatter) formatWithTemplate(tmpl *template.Template, msg Message) string {
	var buf strings.Builder

	data := map[string]interface{}{
		"Message":          msg,
		"IncludeTimestamp": f.IncludeTimestamp,
		"Verbose":          f.Verbose,
		"ShowToolDetails":  f.ShowToolDetails,
	}

	// Add message-specific fields
	switch m := msg.(type) {
	case *SystemMessage:
		data["Type"] = m.Type
		data["SubType"] = m.SubType
		data["SessionID"] = m.SessionID
		data["Timestamp"] = m.Timestamp
	case *AssistantMessage:
		data["Type"] = m.Type
		data["Message"] = m.Message
		data["ParentToolUseID"] = m.ParentToolUseID
		data["SessionID"] = m.SessionID
		data["UUID"] = m.UUID
		data["Timestamp"] = m.Timestamp
	case *UserMessage:
		data["Type"] = m.Type
		data["Message"] = m.Message
		data["ParentToolUseID"] = m.ParentToolUseID
		data["SessionID"] = m.SessionID
		data["Timestamp"] = m.Timestamp
	case *ToolUseMessage:
		data["Type"] = m.Type
		data["Name"] = m.Name
		data["Input"] = m.Input
		data["ToolUseID"] = m.ToolUseID
		data["SessionID"] = m.SessionID
		data["Timestamp"] = m.Timestamp
	case *ToolResultMessage:
		data["Type"] = m.Type
		data["Output"] = m.Output
		data["Content"] = m.Content
		data["ToolUseID"] = m.ToolUseID
		data["IsError"] = m.IsError
		data["SessionID"] = m.SessionID
		data["Timestamp"] = m.Timestamp
	case *StreamEventMessage:
		data["Type"] = m.Type
		data["Event"] = m.Event
		data["SessionID"] = m.SessionID
		data["Timestamp"] = m.Timestamp
	case *ResultMessage:
		data["Type"] = m.Type
		data["SubType"] = m.SubType
		data["Result"] = m.Result
		data["TotalCostUSD"] = m.TotalCostUSD
		data["IsError"] = m.IsError
		data["DurationMS"] = m.DurationMS
		data["DurationAPIMS"] = m.DurationAPIMS
		data["NumTurns"] = m.NumTurns
		data["Usage"] = m.Usage
		data["SessionID"] = m.SessionID
		data["PermissionDenials"] = m.PermissionDenials
		data["Timestamp"] = m.Timestamp
	default:
		data["Type"] = msg.GetType()
		data["Data"] = msg.GetRawData()
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("[ERROR] execute: %v", err)
	}

	return buf.String()
}

func (f *TextFormatter) formatSystem(m *SystemMessage) string {
	var b strings.Builder
	b.WriteString("[SYSTEM]")
	if m.SubType != "" {
		b.WriteString(" ")
		b.WriteString(m.SubType)
	}
	b.WriteString(" Session: ")
	b.WriteString(m.SessionID)
	if f.IncludeTimestamp && !m.Timestamp.IsZero() {
		b.WriteString(" at ")
		b.WriteString(m.Timestamp.Format("2006-01-02 15:04:05"))
	}
	return b.String()
}

func (f *TextFormatter) formatAssistant(m *AssistantMessage) string {
	var b strings.Builder

	if m.Message.ID != "" {
		b.WriteString("[ASSISTANT] ")
		b.WriteString(m.Message.ID)
		b.WriteString("\n")
	} else {
		b.WriteString("[ASSISTANT]")
	}

	for _, content := range m.Message.Content {
		switch content.Type {
		case "text":
			if content.Text != "" {
				b.WriteString(content.Text)
				b.WriteString("\n")
			}
		case "tool_use":
			if f.ShowToolDetails {
				b.WriteString("[TOOL] ")
				b.WriteString(content.Name)
				switch content.Name {
				case "Bash":
					b.WriteString("\n")
					b.WriteString(fmt.Sprintf("%s", content.Input))
				}
				b.WriteString("\n")
			}
		case "thinking":
			if f.Verbose && content.Thinking != "" {
				b.WriteString("[THINKING] ")
				b.WriteString(content.Thinking)
				b.WriteString("\n")
			}
		case "web_search_tool_result":
			if f.ShowToolDetails && content.ToolUseID != "" {
				b.WriteString("[TOOL_RESULT] ")
				b.WriteString(content.ToolUseID)
				b.WriteString("\n")
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
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
	var b strings.Builder
	b.WriteString("[TOOL_USE] ")
	b.WriteString(m.ToolUseID)
	b.WriteString(" (")
	b.WriteString(m.Name)
	b.WriteString(")")

	if m.Input != nil && len(m.Input) > 0 {
		b.WriteString("\nInput: ")
		for key, value := range m.Input {
			b.WriteString(key)
			b.WriteString("=")
			b.WriteString(fmt.Sprintf("%v", value))
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (f *TextFormatter) formatToolResult(m *ToolResultMessage) string {
	var b strings.Builder
	b.WriteString("[TOOL_RESULT] ")
	b.WriteString(m.ToolUseID)
	b.WriteString(" [")
	if m.IsError {
		b.WriteString("ERROR")
	} else {
		b.WriteString("SUCCESS")
	}
	b.WriteString("]")

	if m.Output != "" {
		b.WriteString("\n")
		b.WriteString(m.Output)
	} else if m.Content != nil {
		for _, c := range m.Content {
			if tr, ok := c.(*ToolResultContentBlock); ok && tr.Content != "" {
				b.WriteString("\n")
				b.WriteString(tr.Content)
				break
			}
		}
	}
	return b.String()
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

	if m.Result != "" {
		b.WriteString("\n")
		b.WriteString(m.Result)
	}

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

// SetTemplate sets a custom template for a message type
func (f *TextFormatter) SetTemplate(msgType string, tmpl string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	parsed, err := template.New(msgType).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	if f.customTemplates == nil {
		f.customTemplates = make(map[string]*template.Template)
	}
	f.customTemplates[msgType] = parsed
	return nil
}

// SetTemplateFromFile sets a custom template from a file
func (f *TextFormatter) SetTemplateFromFile(msgType, filename string) error {
	content, err := templateFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read template file: %w", err)
	}
	return f.SetTemplate(msgType, string(content))
}

// getTemplate returns the template for a message type
func (f *TextFormatter) getTemplate(msgType string) (*template.Template, error) {
	f.mu.RLock()

	// Check for custom template override
	if tmpl, ok := f.customTemplates[msgType]; ok {
		f.mu.RUnlock()
		return tmpl, nil
	}
	f.mu.RUnlock()

	// Load default template from embedded FS
	templateName := templateNameForType(msgType)
	content, err := templateFS.ReadFile(templateName)
	if err != nil {
		return nil, fmt.Errorf("load template %s: %w", templateName, err)
	}

	return template.New(msgType).Parse(string(content))
}

// templateNameForType returns the template filename for a message type
func templateNameForType(msgType string) string {
	return fmt.Sprintf("templates/%s.tmpl", msgType)
}

// SetIncludeTimestamp sets whether to include timestamps in output
func (f *TextFormatter) SetIncludeTimestamp(include bool) {
	f.IncludeTimestamp = include
}

// SetVerbose sets verbose mode for detailed output
func (f *TextFormatter) SetVerbose(verbose bool) {
	f.Verbose = verbose
}

// SetShowToolDetails sets whether to show tool details
func (f *TextFormatter) SetShowToolDetails(show bool) {
	f.ShowToolDetails = show
}
