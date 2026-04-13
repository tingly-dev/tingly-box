// Package bash provides bash-specific policy logic on top of the core agentsec Policy.
// It handles bash-specific concerns like shell operator detection and base command extraction.
package bash

import (
	"strings"
)

// HasChaining reports whether a command string contains shell operators
// that compose or chain multiple sub-commands.
//
// Detected operators: | && || ; $( ` (backtick)
//
// Commands containing these operators should always require user approval
// and should NOT be stored in allowlists (unsafe for auto-approval).
func HasChaining(command string) bool {
	for i := 0; i < len(command); i++ {
		switch command[i] {
		case '|', ';', '`':
			return true
		case '&':
			if i+1 < len(command) && command[i+1] == '&' {
				return true
			}
		case '$':
			if i+1 < len(command) && command[i+1] == '(' {
				return true
			}
		}
	}
	return false
}

// ExtractBaseCommand returns the leading token of a command string.
// It handles the common subshell prefix form "(cmd ...)" by extracting
// the inner command.
//
// Examples:
//
//	"git status"         → "git"
//	"(git status)"       → "git"
//	"sudo git status"    → "sudo"
//	"npm run dev"        → "npm"
func ExtractBaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)

	// Handle subshell: (cmd ...)
	if len(trimmed) > 0 && trimmed[0] == '(' {
		// Find closing paren or space, extract inner command
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] == ')' || trimmed[i] == ' ' || trimmed[i] == '\t' {
				inner := strings.TrimSpace(trimmed[1:i])
				return ExtractBaseCommand(inner)
			}
		}
		// No closing paren, extract everything after '('
		if len(trimmed) > 1 {
			return ExtractBaseCommand(strings.TrimSpace(trimmed[1:]))
		}
	}

	// Normal: extract first word
	for i, r := range trimmed {
		if r == ' ' || r == '\t' {
			return strings.ToLower(trimmed[:i])
		}
	}
	return strings.ToLower(trimmed)
}
