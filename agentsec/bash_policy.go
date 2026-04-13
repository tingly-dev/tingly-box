package agentsec

import (
	"strings"
)

// PolicyDecision is the result of a policy check on a bash command string.
type PolicyDecision int

const (
	// PolicyAllow means the command is on the allowlist and may execute directly.
	PolicyAllow PolicyDecision = iota

	// PolicyRequireApproval means the command is not on the allowlist; the caller
	// must request user approval before executing.
	PolicyRequireApproval

	// PolicyRequireApprovalNoRemember is like PolicyRequireApproval but the
	// command contains shell-chaining operators (pipes, &&, etc.) — user approval
	// is still required, but "Always Allow" must NOT be persisted for it.
	PolicyRequireApprovalNoRemember
)

// IsChained reports whether the decision implies a chained/piped command.
func (d PolicyDecision) IsChained() bool {
	return d == PolicyRequireApprovalNoRemember
}

// NeedsApproval reports whether the decision requires user approval.
func (d PolicyDecision) NeedsApproval() bool {
	return d == PolicyRequireApproval || d == PolicyRequireApprovalNoRemember
}

// BashPolicy evaluates a full bash command string against a list of PermissionRules
// and returns the appropriate PolicyDecision. It is stateless and agent-agnostic.
//
// Rules must be in "Bash(...)" format (e.g. "Bash(git)", "Bash(npm *)").
type BashPolicy struct {
	rules []string // raw rule strings, parsed on each Evaluate call
}

// NewBashPolicy creates a BashPolicy from the given rule strings.
// Rules should use the "Bash(...)" format; malformed rules are silently skipped.
func NewBashPolicy(rules []string) *BashPolicy {
	return &BashPolicy{rules: rules}
}

// Evaluate returns the PolicyDecision for the given full command string.
//
//  1. If the command contains shell operators (|, &&, etc.) → PolicyRequireApprovalNoRemember
//  2. If the base command is covered by a Bash rule → PolicyAllow
//  3. Otherwise → PolicyRequireApproval
func (p *BashPolicy) Evaluate(command string) PolicyDecision {
	if HasPipeOrChaining(command) {
		return PolicyRequireApprovalNoRemember
	}
	base := ExtractBaseCommand(command)
	if IsBashCommandAllowed(p.rules, base) {
		return PolicyAllow
	}
	return PolicyRequireApproval
}

// ExtractBaseCommand returns the leading token of a command string, handling
// the common subshell prefix form "(cmd ...)".
func ExtractBaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	// Subshell: (cmd ...)
	if len(trimmed) > 0 && trimmed[0] == '(' {
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] == ')' || trimmed[i] == ' ' || trimmed[i] == '\t' {
				inner := strings.TrimSpace(trimmed[1:i])
				return ExtractBaseCommand(inner)
			}
		}
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
