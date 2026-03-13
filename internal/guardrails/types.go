package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
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
	Name       string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty" yaml:"arguments,omitempty"`
	Shell      *ShellCommand          `json:"shell,omitempty" yaml:"shell,omitempty"`
	Normalized *NormalizedCommand     `json:"normalized,omitempty" yaml:"normalized,omitempty"`
}

// ShellCommand is a lightweight structured view of a shell command string.
type ShellCommand struct {
	Raw       string               `json:"raw,omitempty" yaml:"raw,omitempty"`
	Commands  []ShellSimpleCommand `json:"commands,omitempty" yaml:"commands,omitempty"`
	Operators []string             `json:"operators,omitempty" yaml:"operators,omitempty"`
	Redirects []ShellRedirect      `json:"redirects,omitempty" yaml:"redirects,omitempty"`
}

// ShellSimpleCommand represents a single command in a shell pipeline/sequence.
type ShellSimpleCommand struct {
	Program string   `json:"program,omitempty" yaml:"program,omitempty"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

// ShellRedirect represents a redirection operator and target.
type ShellRedirect struct {
	Op     string `json:"op,omitempty" yaml:"op,omitempty"`
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
}

// NormalizedCommand is a tool-agnostic semantic view used for matching.
type NormalizedCommand struct {
	Kind       string                 `json:"kind,omitempty" yaml:"kind,omitempty"`
	Raw        string                 `json:"raw,omitempty" yaml:"raw,omitempty"`
	Terms      []string               `json:"terms,omitempty" yaml:"terms,omitempty"`
	Resources  []string               `json:"resources,omitempty" yaml:"resources,omitempty"`
	Actions    []string               `json:"actions,omitempty" yaml:"actions,omitempty"`
	Structured map[string]interface{} `json:"structured,omitempty" yaml:"structured,omitempty"`
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

// MatchText returns a stable text representation used by rule-based matching.
func (s *ShellCommand) MatchText() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	if s.Raw != "" {
		b.WriteString(" shell.raw: ")
		b.WriteString(s.Raw)
	}
	if len(s.Commands) > 0 {
		for i, cmd := range s.Commands {
			b.WriteString(fmt.Sprintf(" shell.command[%d]: %s", i, strings.TrimSpace(strings.Join(append([]string{cmd.Program}, cmd.Args...), " "))))
			if cmd.Program != "" {
				b.WriteString(fmt.Sprintf(" shell.program[%d]: %s", i, cmd.Program))
			}
			if len(cmd.Args) > 0 {
				b.WriteString(fmt.Sprintf(" shell.args[%d]: %s", i, strings.Join(cmd.Args, " ")))
			}
		}
	}
	if len(s.Operators) > 0 {
		b.WriteString(" shell.operators: ")
		b.WriteString(strings.Join(s.Operators, " "))
	}
	if len(s.Redirects) > 0 {
		parts := make([]string, 0, len(s.Redirects))
		for _, redirect := range s.Redirects {
			parts = append(parts, strings.TrimSpace(redirect.Op+" "+redirect.Target))
		}
		b.WriteString(" shell.redirects: ")
		b.WriteString(strings.Join(parts, " "))
	}
	return b.String()
}

// MatchText returns a stable semantic representation used by rule-based matching.
func (n *NormalizedCommand) MatchText() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	if n.Kind != "" {
		b.WriteString(" normalized.kind: ")
		b.WriteString(n.Kind)
	}
	if n.Raw != "" {
		b.WriteString(" normalized.raw: ")
		b.WriteString(n.Raw)
	}
	if len(n.Actions) > 0 {
		b.WriteString(" normalized.actions: ")
		b.WriteString(strings.Join(n.Actions, " "))
	}
	if len(n.Resources) > 0 {
		b.WriteString(" normalized.resources: ")
		b.WriteString(strings.Join(n.Resources, " "))
	}
	if len(n.Terms) > 0 {
		b.WriteString(" normalized.terms: ")
		b.WriteString(strings.Join(n.Terms, " "))
	}
	return b.String()
}

// AttachDerivedFields enriches the command with parsed shell metadata when applicable.
func (c *Command) AttachDerivedFields() {
	if c == nil {
		return
	}
	raw, ok := extractShellCommandText(c.Name, c.Arguments)
	if !ok || strings.TrimSpace(raw) == "" {
		return
	}
	if parsed := ParseShellCommand(raw); parsed != nil {
		c.Shell = parsed
		c.Normalized = normalizeShellCommand(parsed)
	}
}

func extractShellCommandText(name string, args map[string]interface{}) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	keys := []string{"command", "cmd", "script"}
	if normalizedName != "bash" && normalizedName != "sh" && normalizedName != "zsh" {
		keys = append(keys, "raw_command")
	}
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return text, true
		}
	}
	if raw, ok := args["_raw"].(string); ok {
		// Some streamed tool calls may temporarily fall back to a raw JSON blob
		// when incremental argument assembly fails. Try to recover the command
		// field from that raw payload before giving up on shell normalization.
		if text, ok := extractShellCommandFromRawJSON(raw); ok {
			return text, true
		}
	}
	return "", false
}

// ParseShellCommand tokenizes a shell command string into a lightweight structure.
func ParseShellCommand(raw string) *ShellCommand {
	tokens := shellSplit(raw)
	if len(tokens) == 0 {
		return nil
	}
	out := &ShellCommand{Raw: strings.TrimSpace(raw)}
	current := ShellSimpleCommand{}
	flush := func() {
		if current.Program == "" {
			return
		}
		out.Commands = append(out.Commands, current)
		current = ShellSimpleCommand{}
	}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case isShellOperator(tok):
			flush()
			out.Operators = append(out.Operators, tok)
		case isShellRedirect(tok):
			target := ""
			if i+1 < len(tokens) {
				target = tokens[i+1]
				i++
			}
			out.Redirects = append(out.Redirects, ShellRedirect{Op: tok, Target: target})
		default:
			if current.Program == "" {
				current.Program = tok
			} else {
				current.Args = append(current.Args, tok)
			}
		}
	}
	flush()
	if len(out.Commands) == 0 && len(out.Redirects) == 0 && len(out.Operators) == 0 {
		return nil
	}
	return out
}

func isShellOperator(tok string) bool {
	switch tok {
	case "|", "||", "&&", ";":
		return true
	default:
		return false
	}
}

func isShellRedirect(tok string) bool {
	switch tok {
	case ">", ">>", "<", "<<", "2>", "2>>", "&>", "1>", "1>>":
		return true
	default:
		return false
	}
}

func shellSplit(raw string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for i := 0; i < len(raw); i++ {
		r := rune(raw[i])
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if quote != 0 {
			if r == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = r
		default:
			if unicode.IsSpace(r) {
				flush()
				continue
			}
			if op, width := detectShellOperator(raw, i); width > 0 {
				flush()
				tokens = append(tokens, op)
				i += width - 1
				continue
			}
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func detectShellOperator(raw string, index int) (string, int) {
	if index >= len(raw) {
		return "", 0
	}
	rest := raw[index:]
	for _, op := range []string{"2>>", "1>>", ">>", "<<", "&&", "||", "2>", "1>", "&>", ">", "<", "|", ";"} {
		if strings.HasPrefix(rest, op) {
			return op, len(op)
		}
	}
	return "", 0
}

func extractShellCommandFromRawJSON(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	candidates := []string{raw}
	if start := strings.Index(raw, "{\""); start >= 0 && start < len(raw) {
		candidates = append(candidates, raw[start:])
	}

	for _, candidate := range candidates {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
			continue
		}
		for _, key := range []string{"command", "cmd", "script", "raw_command"} {
			if value, ok := parsed[key].(string); ok && strings.TrimSpace(value) != "" {
				return value, true
			}
		}
	}
	return "", false
}

func normalizeShellCommand(shell *ShellCommand) *NormalizedCommand {
	if shell == nil {
		return nil
	}
	n := &NormalizedCommand{
		Kind:       "shell",
		Raw:        shell.Raw,
		Structured: map[string]interface{}{},
	}

	terms := make([]string, 0)
	resources := make([]string, 0)
	actions := make([]string, 0)
	programs := make([]string, 0, len(shell.Commands))

	for _, cmd := range shell.Commands {
		if cmd.Program == "" {
			continue
		}
		programs = append(programs, cmd.Program)
		terms = appendUniqueString(terms, cmd.Program)
		actions = appendUniqueString(actions, normalizeShellAction(cmd.Program))

		for _, arg := range cmd.Args {
			terms = appendUniqueString(terms, arg)
			if isResourceLikeToken(arg) {
				resources = appendUniqueString(resources, arg)
			}
		}
	}

	for _, redirect := range shell.Redirects {
		if redirect.Op != "" {
			terms = appendUniqueString(terms, redirect.Op)
		}
		if redirect.Target != "" {
			terms = appendUniqueString(terms, redirect.Target)
			if isResourceLikeToken(redirect.Target) {
				resources = appendUniqueString(resources, redirect.Target)
			}
			actions = appendUniqueString(actions, "redirect")
		}
	}

	for _, op := range shell.Operators {
		terms = appendUniqueString(terms, op)
	}

	n.Terms = terms
	n.Resources = resources
	n.Actions = actions
	n.Structured["programs"] = programs
	n.Structured["operators"] = append([]string(nil), shell.Operators...)
	n.Structured["redirects"] = append([]ShellRedirect(nil), shell.Redirects...)
	return n
}

func normalizeShellAction(program string) string {
	switch program {
	case "cat", "less", "more", "head", "tail", "grep", "rg", "find", "ls", "stat":
		return "read"
	case "cp", "mv", "tee", "touch", "mkdir", "chmod", "chown", "sed", "awk":
		return "write"
	case "rm", "rmdir", "shred":
		return "delete"
	case "curl", "wget", "scp", "rsync":
		return "transfer"
	case "bash", "sh", "zsh", "python", "node", "ruby", "perl":
		return "execute"
	default:
		return "execute"
	}
}

func isResourceLikeToken(token string) bool {
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "~/") || strings.HasPrefix(token, "./") || strings.HasPrefix(token, "../") {
		return true
	}
	return strings.Contains(token, "/") || strings.Contains(token, ".")
}

func appendUniqueString(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
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
