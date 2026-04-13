package agentsec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PermissionRule is a parsed allow rule that scopes a glob pattern to a specific tool.
//
// Canonical string form:
//
//	"Bash(git *)"  → ToolName="Bash", Pattern="git *"
//	"Read"         → ToolName="Read", Pattern=""  (matches any Read invocation)
//	"Bash(git)"    → ToolName="Bash", Pattern="git"
//
// For Bash rules, Pattern is matched against the full command string.
// An empty Pattern matches any invocation of the named tool.
type PermissionRule struct {
	ToolName string // e.g. "Bash", "Read", "Write"
	Pattern  string // e.g. "git *", "rm -rf *", "" = match all
}

// String returns the canonical serialized form of the rule.
//
//	PermissionRule{"Bash", "git *"} → "Bash(git *)"
//	PermissionRule{"Read", ""}      → "Read"
func (r PermissionRule) String() string {
	if r.Pattern == "" {
		return r.ToolName
	}
	return r.ToolName + "(" + r.Pattern + ")"
}

// ParseRule parses a rule string into a PermissionRule.
//
//	"Bash(git *)"  → PermissionRule{"Bash", "git *"}, nil
//	"Read"         → PermissionRule{"Read", ""}, nil
//	"Bash()"       → error (empty pattern — use bare "Bash" for match-all)
//	""             → error
func ParseRule(s string) (PermissionRule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PermissionRule{}, fmt.Errorf("agentsec: empty rule string")
	}

	open := strings.IndexByte(s, '(')
	if open < 0 {
		// Bare tool name, e.g. "Bash" or "Read"
		if !isValidToolName(s) {
			return PermissionRule{}, fmt.Errorf("agentsec: invalid tool name %q in rule %q", s, s)
		}
		return PermissionRule{ToolName: s}, nil
	}

	if s[len(s)-1] != ')' {
		return PermissionRule{}, fmt.Errorf("agentsec: rule %q missing closing ')'", s)
	}

	toolName := s[:open]
	pattern := s[open+1 : len(s)-1]

	if !isValidToolName(toolName) {
		return PermissionRule{}, fmt.Errorf("agentsec: invalid tool name %q in rule %q", toolName, s)
	}
	if pattern == "" {
		return PermissionRule{}, fmt.Errorf("agentsec: empty pattern in rule %q — use bare %q for match-all", s, toolName)
	}

	return PermissionRule{ToolName: toolName, Pattern: pattern}, nil
}

// NewBashRule creates a PermissionRule for a bash command.
// hasArgs=true → pattern "cmd *" (canonical persisted form when the invocation had arguments)
// hasArgs=false → pattern "cmd" (exact base-command match)
func NewBashRule(cmd string, hasArgs bool) PermissionRule {
	if hasArgs {
		return PermissionRule{ToolName: "Bash", Pattern: cmd + " *"}
	}
	return PermissionRule{ToolName: "Bash", Pattern: cmd}
}

// Matches reports whether this rule permits the given tool invocation.
//
//   - toolName: the name of the tool being called (e.g. "Bash", "Read")
//   - input: the tool's primary input string (full command for Bash, path for Read/Write)
//
// For Bash rules the full command string is matched (minus shell operators — those
// are blocked by BashPolicy before Matches is ever called).
func (r PermissionRule) Matches(toolName, input string) bool {
	if !strings.EqualFold(r.ToolName, toolName) {
		return false
	}
	if r.Pattern == "" {
		return true // bare rule: match any invocation
	}
	return matchesPattern(r.Pattern, input)
}

// matchesPattern reports whether input is matched by pattern.
//
// Matching rules:
//
//	"git"          → exact match only: input must equal "git" (no args)
//	"git *"        → prefix match: "git", "git status", "git commit -m ..."
//	"git commit *" → "git commit", "git commit -m ..."
//	"git commit"   → exact: only "git commit" (no further args)
//	"rm -rf *"     → "rm -rf", "rm -rf /tmp/foo"
//
// The trailing " *" suffix is the only wildcard shorthand; it means
// "this prefix, optionally followed by a space and any arguments".
// Patterns with other glob chars (?, [) use filepath.Match semantics.
func matchesPattern(pattern, input string) bool {
	patLower := strings.ToLower(strings.TrimSpace(pattern))
	inLower := strings.ToLower(strings.TrimSpace(input))

	// Wildcard suffix " *": prefix + optional args
	if strings.HasSuffix(patLower, " *") {
		prefix := strings.TrimSuffix(patLower, " *")
		return inLower == prefix || strings.HasPrefix(inLower, prefix+" ")
	}

	// No glob chars: exact match only
	if !strings.ContainsAny(patLower, "*?[") {
		return inLower == patLower
	}

	// General glob (filepath.Match semantics)
	matched, err := filepath.Match(patLower, inLower)
	return err == nil && matched
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
