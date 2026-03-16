package guardrails

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

var shellCommandArgumentKeys = []string{"command", "cmd", "script"}

var genericCommandArgumentKeys = []string{"command", "cmd", "script", "raw_command"}

var shellOperators = []string{"|", "||", "&&", ";"}

// Order matters here: multi-character operators must be checked first.
var shellTokenOperators = []string{"2>>", "1>>", ">>", "<<", "&&", "||", "2>", "1>", "&>", ">", "<", "|", ";"}

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
	for _, key := range commandArgumentKeys(name) {
		if text, ok := stringArgument(args, key); ok {
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
	parser := newShellCommandParser(strings.TrimSpace(raw))

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case isShellOperator(tok):
			parser.flushCurrent()
			parser.out.Operators = append(parser.out.Operators, tok)
		case isShellRedirect(tok):
			target := ""
			if i+1 < len(tokens) {
				target = tokens[i+1]
				i++
			}
			parser.out.Redirects = append(parser.out.Redirects, ShellRedirect{Op: tok, Target: target})
		default:
			parser.pushWord(tok)
		}
	}
	parser.flushCurrent()
	if len(parser.out.Commands) == 0 && len(parser.out.Redirects) == 0 && len(parser.out.Operators) == 0 {
		return nil
	}
	return parser.out
}

func isShellOperator(tok string) bool {
	for _, op := range shellOperators {
		if tok == op {
			return true
		}
	}
	return false
}

func isShellRedirect(tok string) bool {
	for _, op := range shellTokenOperators {
		switch op {
		case ">", ">>", "<", "<<", "2>", "2>>", "&>", "1>", "1>>":
			if tok == op {
				return true
			}
		}
	}
	return false
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
	for _, op := range shellTokenOperators {
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
		for _, key := range genericCommandArgumentKeys {
			if value, ok := stringArgument(parsed, key); ok {
				return value, true
			}
		}
	}
	return "", false
}

type shellCommandParser struct {
	out     *ShellCommand
	current ShellSimpleCommand
}

func newShellCommandParser(raw string) *shellCommandParser {
	return &shellCommandParser{
		out: &ShellCommand{Raw: raw},
	}
}

func (p *shellCommandParser) flushCurrent() {
	if p.current.Program == "" {
		return
	}
	p.out.Commands = append(p.out.Commands, p.current)
	p.current = ShellSimpleCommand{}
}

func (p *shellCommandParser) pushWord(token string) {
	if p.current.Program == "" {
		p.current.Program = token
		return
	}
	p.current.Args = append(p.current.Args, token)
}

func commandArgumentKeys(name string) []string {
	if isShellCommandName(name) {
		return shellCommandArgumentKeys
	}
	return genericCommandArgumentKeys
}

func isShellCommandName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash", "sh", "zsh":
		return true
	default:
		return false
	}
}

func stringArgument(args map[string]interface{}, key string) (string, bool) {
	value, ok := args[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}
