package protocoltest

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// The rule-flag behavior suite itself lives in flags.go (shared with the CLI
// `harness matrix --mode=flags`). This file is just the go-test entry points.
// See .design/rule-flag-testing.md.

// TestRuleFlags drives every rule flag through the real gateway and asserts its
// observable effect. *testing.T satisfies the flagTB the cases use.
func TestRuleFlags(t *testing.T) {
	for _, fc := range ruleFlagCases() {
		fc := fc
		t.Run(fc.key, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			fc.run(t, env)
		})
	}
}

// TestRuleFlagRegistry_FullyCovered locks the contract that every flag in the
// canonical registry has a behavior test, and that no test references a flag key
// that no longer exists. Adding a flag to RuleFlagRegistry() without a case here
// fails this test.
func TestRuleFlagRegistry_FullyCovered(t *testing.T) {
	known := map[string]bool{}
	for _, spec := range typ.RuleFlagRegistry() {
		known[spec.Key] = true
	}
	covered := map[string]bool{}
	for _, fc := range ruleFlagCases() {
		if !known[fc.key] {
			t.Errorf("flag case %q does not match any typ.RuleFlagRegistry() key", fc.key)
		}
		if covered[fc.key] {
			t.Errorf("duplicate flag case for %q", fc.key)
		}
		covered[fc.key] = true
	}
	for key := range known {
		if !covered[key] {
			t.Errorf("rule flag %q has no behavior test in ruleFlagCases() — add one", key)
		}
	}
}
