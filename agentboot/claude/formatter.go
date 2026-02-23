package claude

import (
	"bytes"
	"embed"
	"fmt"
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

// Format formats a message using its template
func (f *TextFormatter) Format(msg Message) string {
	if msg == nil {
		return ""
	}

	tmpl, err := f.getTemplate(msg.GetType())
	if err != nil {
		return fmt.Sprintf("[ERROR] template: %v", err)
	}

	var buf bytes.Buffer

	// Create template data with formatter options
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
		// For unknown types, try to use raw data
		data["Type"] = msg.GetType()
		data["Data"] = msg.GetRawData()
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("[ERROR] execute: %v", err)
	}

	return buf.String()
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
