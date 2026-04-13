// Package bash provides bash-specific policy logic on top of the core agentsec Policy.
package bash

import (
	"github.com/tingly-dev/tingly-box/agentsec"
)

// DefaultBashAllowlist is the built-in set of bash commands that are always
// permitted without user approval. This is provided as a string slice for
// backward compatibility with code that hasn't been updated to use Rules yet.
// Deprecated: Use DefaultRules() instead.
var DefaultBashAllowlist = []string{
	// File navigation
	"Bash(ls *)", "Bash(pwd)", "Bash(cd *)", "Bash(cat *)", "Bash(tree *)",
	// File operations
	"Bash(mkdir *)", "Bash(cp *)", "Bash(mv *)", "Bash(touch *)", "Bash(chmod *)",
	// Git operations
	"Bash(git *)",
	// Network (setup / fetching)
	"Bash(curl *)", "Bash(wget *)",
	// Utility
	"Bash(echo *)", "Bash(which *)", "Bash(env)", "Bash(head *)", "Bash(tail *)",
	"Bash(wc *)", "Bash(find *)", "Bash(grep *)",
}

// DefaultRules returns the built-in set of bash commands that are always
// permitted without user approval. These are safe, commonly-used utilities.
//
// Commands use the "Bash(cmd *)" format (PrefixRule) which allows the
// command with any arguments. This is the typical safe default for
// well-known utilities like ls, git, cat, etc.
func DefaultRules() []agentsec.Rule {
	return []agentsec.Rule{
		// File navigation
		agentsec.PrefixRule{Tool: "Bash", Prefix: "ls"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "pwd"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "cd"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "cat"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "tree"},
		// File operations
		agentsec.PrefixRule{Tool: "Bash", Prefix: "mkdir"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "cp"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "mv"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "touch"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "chmod"},
		// Git operations
		agentsec.PrefixRule{Tool: "Bash", Prefix: "git"},
		// Network (setup / fetching)
		agentsec.PrefixRule{Tool: "Bash", Prefix: "curl"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "wget"},
		// Utility
		agentsec.PrefixRule{Tool: "Bash", Prefix: "echo"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "which"},
		agentsec.ExactRule{Tool: "Bash", Input: "env"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "head"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "tail"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "wc"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "find"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "grep"},
	}
}

// DefaultPolicy creates a Policy with the default bash rules.
// This is a convenience function for creating a policy pre-configured
// with the standard safe bash commands.
func DefaultPolicy() *agentsec.Policy {
	return agentsec.NewPolicy(DefaultRules())
}

// NewPolicyWithDefaultRules creates a Policy with the default bash rules.
// This is a convenience function that combines DefaultRules() with NewPolicy().
func NewPolicyWithDefaultRules() *Policy {
	return NewPolicy(DefaultRules())
}

// MergeAllowlists merges two rule slices, removing duplicates (case-insensitive).
// The relative order of base entries is preserved.
// Deprecated: Use direct rule management instead.
func MergeAllowlists(base, extra []agentsec.Rule) []agentsec.Rule {
	seen := make(map[string]bool, len(base)+len(extra))
	result := make([]agentsec.Rule, 0, len(base)+len(extra))

	// Add base rules first
	for _, rule := range base {
		key := rule.String()
		if !seen[key] {
			seen[key] = true
			result = append(result, rule)
		}
	}

	// Add extra rules that aren't duplicates
	for _, rule := range extra {
		key := rule.String()
		if !seen[key] {
			seen[key] = true
			result = append(result, rule)
		}
	}

	return result
}
