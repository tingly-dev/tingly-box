package protocoltest

import "testing"

// TestThinkingLimits is the go-test entry point of the thinking × output-limit
// combination suite (thinking_limits.go): one parallel subtest per route, each
// reporting every failing combination it ran.
func TestThinkingLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("combination suite: run without -short or via `harness matrix --mode=thinking_limits`")
	}
	for _, g := range thinkingLimitsGroups() {
		name := string(g.source) + "->" + string(g.target) + "/rule=" + ruleLabel(g.rule)
		if g.tokenFlag != tokenFieldAsSent {
			name += "+" + g.tokenFlag
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range runThinkingLimitsGroup(g) {
				if r.Failed() {
					t.Errorf("%s: %s", r.Name, thinkingLimitsFailureSummary(r))
				}
			}
		})
	}
}

func ruleLabel(rule string) string {
	if rule == "" {
		return "client"
	}
	return rule
}
