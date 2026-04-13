package agentsec

import "strings"

// DefaultBashAllowlist is the built-in set of bash commands that are always
// permitted without user approval. Rules use "Bash(cmd *)" to allow the base
// command with any arguments, which is the typical safe default for these
// well-known utilities.
var DefaultBashAllowlist = []string{
	// File navigation
	"Bash(ls *)", "Bash(pwd)", "Bash(cd *)", "Bash(cat *)", "Bash(tree *)",
	// File operations
	"Bash(mkdir *)", "Bash(rm *)", "Bash(cp *)", "Bash(mv *)", "Bash(touch *)", "Bash(chmod *)",
	// Git operations
	"Bash(git *)",
	// Network (setup / fetching)
	"Bash(curl *)", "Bash(wget *)",
	// Utility
	"Bash(echo *)", "Bash(which *)", "Bash(env)", "Bash(head *)", "Bash(tail *)",
	"Bash(wc *)", "Bash(find *)", "Bash(grep *)",
}

// MergeAllowlists returns a new slice containing all entries from base followed
// by any entries in extra that are not already present (case-insensitive dedup).
// The relative order of base entries is preserved.
func MergeAllowlists(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	result := make([]string, 0, len(base)+len(extra))
	for _, cmd := range append(base, extra...) {
		key := strings.ToLower(cmd)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, cmd)
		}
	}
	return result
}

// IsBashCommandAllowed reports whether baseCmd is permitted by any rule in
// the allowlist. Rules must be in "Bash(...)" format.
func IsBashCommandAllowed(rules []string, baseCmd string) bool {
	for _, ruleStr := range rules {
		r, err := ParseRule(ruleStr)
		if err != nil {
			continue
		}
		if r.Matches("Bash", baseCmd) {
			return true
		}
	}
	return false
}
