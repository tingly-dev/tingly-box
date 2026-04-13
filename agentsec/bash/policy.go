// Package bash provides bash-specific policy logic on top of the core agentsec Policy.
package bash

import (
	"github.com/tingly-dev/tingly-box/agentsec"
)

// Decision is the outcome of bash policy evaluation.
// It extends the core Decision with bash-specific information.
type Decision struct {
	// Allow is true if the command is permitted by the allowlist.
	Allow bool

	// Remember indicates whether this command can be persisted to the allowlist.
	// If false, the command contains shell operators (pipes, &&, etc.) and
	// should never be auto-approved or stored in the allowlist.
	Remember bool
}

// Policy evaluates bash commands against an allowlist with bash-specific
// logic for chaining detection and base command extraction.
type Policy struct {
	policy *agentsec.Policy
}

// NewPolicy creates a BashPolicy with the given rules.
// The rules are typically created via DefaultRules() or loaded from storage.
func NewPolicy(rules []agentsec.Rule) *Policy {
	return &Policy{
		policy: agentsec.NewPolicy(rules),
	}
}

// NewPolicyWithDefault creates a BashPolicy with the default bash rules.
// This is the most common way to create a BashPolicy.
func NewPolicyWithDefault() *Policy {
	return &Policy{
		policy: DefaultPolicy(),
	}
}

// Evaluate returns the decision for the given bash command string.
//
// Logic:
// 1. If the command contains shell operators (|, &&, etc.) → return Decision{Allow: false, Remember: false}
// 2. Extract the base command (e.g., "git status" → "git")
// 3. Check if base command matches any rule
// 4. Return appropriate Decision
func (p *Policy) Evaluate(command string) Decision {
	// Check for chaining operators first
	if HasChaining(command) {
		return Decision{
			Allow:    false,
			Remember: false, // Chained commands must never be persisted
		}
	}

	// Extract base command for rule matching
	baseCmd := ExtractBaseCommand(command)

	// Evaluate against rules
	coreDecision := p.policy.Evaluate("Bash", baseCmd)

	// Map core Decision to bash Decision
	switch coreDecision {
	case agentsec.DecisionAllow:
		return Decision{
			Allow:    true,
			Remember: true, // Can be persisted to allowlist
		}
	case agentsec.DecisionDeny:
		return Decision{
			Allow:    false,
			Remember: false,
		}
	default: // DecisionRequireApproval
		return Decision{
			Allow:    false,
			Remember: true, // User approval can be persisted
		}
	}
}

// AddRule returns a new BashPolicy with the additional rule appended.
// The original BashPolicy is unchanged (immutable pattern).
func (p *Policy) AddRule(rule agentsec.Rule) *Policy {
	return &Policy{
		policy: p.policy.AddRule(rule),
	}
}

// Rules returns a copy of the underlying policy's rules.
func (p *Policy) Rules() []agentsec.Rule {
	return p.policy.Rules()
}

// UnderlyingPolicy returns the core agentsec.Policy for advanced use cases.
// Most callers should use Evaluate() instead.
func (p *Policy) UnderlyingPolicy() *agentsec.Policy {
	return p.policy
}
