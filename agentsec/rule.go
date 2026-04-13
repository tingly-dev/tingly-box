package agentsec

import (
	"fmt"
	"strings"
)

// Rule defines a permission rule that can match tool invocations.
// Implementations must be safe for concurrent use.
type Rule interface {
	// Matches reports whether this rule permits the given tool invocation.
	// tool: the tool name (e.g., "Bash", "Read", "Write")
	// input: the tool's input (command for Bash, path for Read/Write)
	Matches(tool, input string) bool

	// String returns the canonical serialized form of this rule.
	// This string is used for JSON persistence and display to users.
	// Format: "ToolName(pattern)" or "ToolName" for match-all.
	String() string
}

// ExactRule matches a tool invocation exactly (no additional input allowed).
// Example: ExactRule{Tool: "Bash", Input: "git"} matches "git" but not "git status"
type ExactRule struct {
	Tool  string
	Input string
}

// Matches returns true if tool and input exactly match this rule (case-insensitive).
func (r ExactRule) Matches(tool, input string) bool {
	return strings.EqualFold(r.Tool, tool) && strings.EqualFold(r.Input, input)
}

// String returns the canonical serialized form: "Tool(input)".
func (r ExactRule) String() string {
	return fmt.Sprintf("%s(%s)", r.Tool, r.Input)
}

// PrefixRule matches a tool invocation with a prefix.
// The " *" suffix indicates "this prefix, optionally followed by arguments".
// Example: PrefixRule{Tool: "Bash", Prefix: "git"} matches "git", "git status", etc.
// It does NOT match "gitdiff" (no space delimiter).
type PrefixRule struct {
	Tool   string
	Prefix string
}

// Matches returns true if the tool matches and input equals the prefix or starts with prefix + space.
func (r PrefixRule) Matches(tool, input string) bool {
	if !strings.EqualFold(r.Tool, tool) {
		return false
	}
	inputLower := strings.ToLower(input)
	prefixLower := strings.ToLower(r.Prefix)

	// Exact prefix match (no arguments) or prefix + space + arguments
	return inputLower == prefixLower || strings.HasPrefix(inputLower, prefixLower+" ")
}

// String returns the canonical serialized form: "Tool(prefix *)".
func (r PrefixRule) String() string {
	return fmt.Sprintf("%s(%s *)", r.Tool, r.Prefix)
}

// AnyToolRule matches all invocations of a specific tool, regardless of input.
// Example: AnyToolRule{Tool: "Read"} matches any Read(...) invocation
type AnyToolRule struct {
	Tool string
}

// Matches returns true if the tool name matches (case-insensitive), ignoring input.
func (r AnyToolRule) Matches(tool, input string) bool {
	return strings.EqualFold(r.Tool, tool)
}

// String returns the canonical serialized form: just the tool name.
func (r AnyToolRule) String() string {
	return r.Tool
}

// ParseRule converts a string representation to a Rule.
// This is the primary entry point for creating Rules from persisted storage.
//
// Formats:
//
//	"Bash(git)"      → ExactRule{Tool: "Bash", Input: "git"}
//	"Bash(git *)"    → PrefixRule{Tool: "Bash", Prefix: "git"}
//	"Bash"           → AnyToolRule{Tool: "Bash"}
//	"Bash(rm -rf *)" → PrefixRule{Tool: "Bash", Prefix: "rm -rf"}
func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("agentsec: empty rule string")
	}

	open := strings.IndexByte(s, '(')
	if open < 0 {
		// Bare tool name → AnyToolRule
		if !isValidToolName(s) {
			return nil, fmt.Errorf("agentsec: invalid tool name %q", s)
		}
		return AnyToolRule{Tool: s}, nil
	}

	if s[len(s)-1] != ')' {
		return nil, fmt.Errorf("agentsec: rule %q missing closing ')'", s)
	}

	toolName := s[:open]
	pattern := s[open+1 : len(s)-1]

	if !isValidToolName(toolName) {
		return nil, fmt.Errorf("agentsec: invalid tool name %q in rule %q", toolName, s)
	}
	if pattern == "" {
		return nil, fmt.Errorf("agentsec: empty pattern in rule %q — use bare %q for match-all", s, toolName)
	}

	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return PrefixRule{Tool: toolName, Prefix: prefix}, nil
	}

	return ExactRule{Tool: toolName, Input: pattern}, nil
}

// isValidToolName checks that s is a non-empty identifier [A-Za-z][A-Za-z0-9_]*.
func isValidToolName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
				return false
			}
		} else {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}
