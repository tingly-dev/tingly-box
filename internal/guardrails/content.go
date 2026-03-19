package guardrails

import (
	"encoding/json"
	"strings"
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role" yaml:"role"`
	Content string `json:"content" yaml:"content"`
}

// Command represents a model function-calling payload.
type Command struct {
	Name       string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty" yaml:"arguments,omitempty"`
	Shell      *ShellCommand          `json:"shell,omitempty" yaml:"shell,omitempty"`
	Normalized *NormalizedCommand     `json:"normalized,omitempty" yaml:"normalized,omitempty"`
}

// Content holds a single response text, optional command call, and message history.
type Content struct {
	Command  *Command  `json:"command,omitempty" yaml:"command,omitempty"`
	Text     string    `json:"text,omitempty" yaml:"text,omitempty"`
	Messages []Message `json:"messages,omitempty" yaml:"messages,omitempty"`
}

// Preview returns a short snippet for logging or UI messages.
func (c Content) Preview(limit int) string {
	if limit <= 0 {
		limit = 120
	}
	text := c.CombinedText()
	if text == "" {
		return ""
	}
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

// LatestPreview returns a short snippet biased toward the newest relevant input.
// Message history is append-only in most protocol adapters, so using the full
// combined text makes history rows all look the same. For history logging we
// prefer the current text payload, current command, or the last message only.
func (c Content) LatestPreview(limit int) string {
	if limit <= 0 {
		limit = 120
	}

	var text string
	switch {
	case c.Text != "":
		text = c.Text
	case c.Command != nil:
		text = Content{Command: c.Command}.CombinedText()
	case len(c.Messages) > 0:
		text = Content{Messages: []Message{c.Messages[len(c.Messages)-1]}}.CombinedText()
	default:
		return ""
	}

	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

// CombinedText returns a single string representation of the content.
func (c Content) CombinedText() string {
	return c.CombinedTextFor(nil)
}

// CombinedTextFor returns a string representation for selected content types.
func (c Content) CombinedTextFor(targets []ContentType) string {
	useAll := len(targets) == 0
	var b strings.Builder

	if c.Text != "" && (useAll || hasContentType(targets, ContentTypeText)) {
		b.WriteString(c.Text)
	} else if len(c.Messages) > 0 && (useAll || hasContentType(targets, ContentTypeMessages)) {
		for i, msg := range c.Messages {
			if msg.Role != "" {
				b.WriteString(msg.Role)
				b.WriteString(": ")
			}
			b.WriteString(msg.Content)
			if i < len(c.Messages)-1 {
				b.WriteString("\n")
			}
		}
	}

	if c.Command != nil && (useAll || hasContentType(targets, ContentTypeCommand)) {
		cmd := c.Command
		if cmd.Shell == nil || cmd.Normalized == nil {
			cloned := *cmd
			cloned.AttachDerivedFields()
			cmd = &cloned
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("command: ")
		b.WriteString(cmd.Name)
		if cmd.Normalized != nil {
			b.WriteString(cmd.Normalized.MatchText())
		} else if cmd.Shell != nil {
			b.WriteString(cmd.Shell.MatchText())
		} else if len(cmd.Arguments) > 0 {
			if payload, err := json.Marshal(cmd.Arguments); err == nil {
				b.WriteString(" arguments: ")
				b.Write(payload)
			}
		}
	}

	return b.String()
}

// Filter returns a copy of content with only selected types included.
func (c Content) Filter(targets []ContentType) Content {
	if len(targets) == 0 {
		return c
	}
	filtered := Content{}
	if hasContentType(targets, ContentTypeText) {
		filtered.Text = c.Text
	}
	if hasContentType(targets, ContentTypeMessages) {
		filtered.Messages = c.Messages
	}
	if hasContentType(targets, ContentTypeCommand) {
		filtered.Command = c.Command
	}
	return filtered
}

// HasAny reports whether content has any of the selected types populated.
func (c Content) HasAny(targets []ContentType) bool {
	if len(targets) == 0 {
		return c.Text != "" || len(c.Messages) > 0 || c.Command != nil
	}
	if hasContentType(targets, ContentTypeText) && c.Text != "" {
		return true
	}
	if hasContentType(targets, ContentTypeMessages) && len(c.Messages) > 0 {
		return true
	}
	if hasContentType(targets, ContentTypeCommand) && c.Command != nil {
		return true
	}
	return false
}

func hasContentType(list []ContentType, target ContentType) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
