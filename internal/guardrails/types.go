package guardrails

import (
	"context"
	"encoding/json"
	"strings"
)

// Verdict is the overall decision from a rule or engine.
type Verdict string

const (
	VerdictAllow  Verdict = "allow"
	VerdictReview Verdict = "review"
	VerdictRedact Verdict = "redact"
	VerdictBlock  Verdict = "block"
)

// CombineStrategy controls how multiple rule verdicts are merged.
type CombineStrategy string

const (
	StrategyMostSevere CombineStrategy = "most_severe"
	StrategyBlockOnAny CombineStrategy = "block_on_any"
)

// ErrorStrategy controls the fallback verdict when a rule fails.
type ErrorStrategy string

const (
	ErrorStrategyAllow  ErrorStrategy = "allow"
	ErrorStrategyReview ErrorStrategy = "review"
	ErrorStrategyBlock  ErrorStrategy = "block"
)

// Direction indicates whether the input is a request or response.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// ContentType identifies a portion of Content.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeMessages ContentType = "messages"
	ContentTypeCommand  ContentType = "command"
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role" yaml:"role"`
	Content string `json:"content" yaml:"content"`
}

// Command represents a model function-calling payload.
type Command struct {
	Name      string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Arguments map[string]interface{} `json:"arguments,omitempty" yaml:"arguments,omitempty"`
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
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("command: ")
		b.WriteString(c.Command.Name)
		if len(c.Command.Arguments) > 0 {
			if payload, err := json.Marshal(c.Command.Arguments); err == nil {
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

// Input is the normalized data sent to guardrails.
type Input struct {
	Scenario  string                 `json:"scenario,omitempty" yaml:"scenario,omitempty"`
	Model     string                 `json:"model,omitempty" yaml:"model,omitempty"`
	Direction Direction              `json:"direction" yaml:"direction"`
	Tags      []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Content   Content                `json:"content" yaml:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Text returns the combined text for guardrails matching.
func (i Input) Text() string {
	return i.Content.CombinedText()
}

// RuleType identifies a rule implementation.
type RuleType string

// RuleResult captures a single rule decision.
type RuleResult struct {
	RuleID   string                 `json:"rule_id" yaml:"rule_id"`
	RuleName string                 `json:"rule_name" yaml:"rule_name"`
	RuleType RuleType               `json:"rule_type" yaml:"rule_type"`
	Verdict  Verdict                `json:"verdict" yaml:"verdict"`
	Reason   string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence map[string]interface{} `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// RuleError captures an evaluation failure for a rule.
type RuleError struct {
	RuleID   string   `json:"rule_id" yaml:"rule_id"`
	RuleName string   `json:"rule_name" yaml:"rule_name"`
	RuleType RuleType `json:"rule_type" yaml:"rule_type"`
	Error    string   `json:"error" yaml:"error"`
}

// Result is the aggregated guardrails decision.
type Result struct {
	Verdict Verdict      `json:"verdict" yaml:"verdict"`
	Reasons []RuleResult `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	Errors  []RuleError  `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Rule evaluates a single guardrail policy.
type Rule interface {
	ID() string
	Name() string
	Type() RuleType
	Enabled() bool
	Evaluate(ctx context.Context, input Input) (RuleResult, error)
}

// Guardrails is the interface for evaluating input.
type Guardrails interface {
	Evaluate(ctx context.Context, input Input) (Result, error)
}
