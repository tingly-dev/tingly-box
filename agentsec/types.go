// Package agentsec provides command-level security primitives for agent tool execution.
//
// It defines the PermissionRule format, approval contracts, and policy-checking
// logic used to gate tool execution. This package has no dependency on any specific
// agent (SmartGuide, ClaudeCode, etc.) and is intended to be a shared foundation
// analogous to agentboot.
//
// Rule format: "ToolName(pattern)" or bare "ToolName"
//
//	"Bash(git)"       — allow git with any arguments
//	"Bash(npm *)"     — allow npm with any arguments (canonical persisted form)
//	"Bash(rm -rf *)"  — allow rm -rf with any path
//	"Read(./src/**)"  — allow reading files under ./src/
//	"Read"            — allow reading any file
package agentsec

import "context"

// ApprovalCallback is called when a tool call is not auto-approved by the allowlist.
// Returns (true, nil) if the user approves, (false, nil) if denied,
// or (false, err) if the approval process itself failed.
type ApprovalCallback func(ctx context.Context, req ApprovalRequest) (approved bool, err error)

// ApprovalRequest carries the details of a tool call that requires user approval.
type ApprovalRequest struct {
	// Command is the base command name for Bash tools (e.g. "npm").
	Command string

	// Args are the command arguments (e.g. ["run", "dev"]).
	Args []string

	// Reason is a human-readable explanation of why approval is needed.
	Reason string

	// IsChained is true when the command string contains shell operators
	// (|, &&, ||, ;, $(...), backtick). Chained commands are never
	// allowlist-eligible; callers should suppress "Always Allow" storage
	// when this flag is set.
	IsChained bool
}

// AllowRule represents a bash command that the user approved, to be persisted
// as a PermissionRule in the tool allowlist.
//
//   - HasArgs=false → stored as "Bash(cmd)"   (exact match: command with no arguments)
//   - HasArgs=true  → stored as "Bash(cmd *)" (prefix match: command with any arguments)
//
// Use Rule() to get the PermissionRule, then String() to get the storable string.
type AllowRule struct {
	// Command is the base command name (e.g. "npm").
	Command string

	// HasArgs indicates the original invocation had arguments.
	// true  → store "Bash(cmd *)" so future invocations with args are also allowed.
	// false → store "Bash(cmd)"   so only the bare command (no args) is allowed.
	HasArgs bool
}

// Rule returns the PermissionRule for this allow entry.
//
//	AllowRule{"npm", true}.Rule()   → PermissionRule{"Bash","npm *"}  → "Bash(npm *)"
//	AllowRule{"make", false}.Rule() → PermissionRule{"Bash","make"}   → "Bash(make)"
func (a AllowRule) Rule() PermissionRule {
	return NewBashRule(a.Command, a.HasArgs)
}

// String returns the canonical serialized form, ready to store in ToolAllowlist.
func (a AllowRule) String() string {
	return a.Rule().String()
}
