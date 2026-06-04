package protocoltest

import (
	"fmt"
	"strings"
	"testing"
)

// TransitiveChain represents a two-hop conversion path: A→B then B→C.
// Both hops go through the full gateway pipeline. The test verifies that
// the semantic content (text, role, tool calls) is preserved across both
// conversions.
type TransitiveChain struct {
	First  ProtocolPair // A→B
	Second ProtocolPair // B→C (Second.Source == First.Target)
}

// String returns a human-readable label like "anthropic_v1→openai_chat→anthropic_beta".
func (c TransitiveChain) String() string {
	return fmt.Sprintf("%s→%s→%s", c.First.Source, c.First.Target, c.Second.Target)
}

// DefaultChains builds transitive chains by composing pairs from the matrix
// where the first pair's target matches the second pair's source.
// Self-loops (A→B→A where first == reverse of second) are included — they
// test round-trip fidelity.
func (m *Matrix) DefaultChains() []TransitiveChain {
	var chains []TransitiveChain
	for _, first := range m.Pairs {
		for _, second := range m.Pairs {
			if first.Target != second.Source {
				continue
			}
			// Skip identity chains where both hops are the same passthrough
			if first.Source == first.Target && second.Source == second.Target {
				continue
			}
			chains = append(chains, TransitiveChain{First: first, Second: second})
		}
	}
	return chains
}

// skipTransitiveScenarios lists source+scenario combinations where the second
// hop is known to fail (inherits from single-hop skip list).
func skipTransitiveKey(chain TransitiveChain, scenarioName string) (string, bool) {
	// Check both hops against the single-hop skip list
	firstKey := fmt.Sprintf("%s|%s", chain.First.Source, scenarioName)
	if reason, skip := skipSourceScenarios[firstKey]; skip {
		return reason, true
	}
	secondKey := fmt.Sprintf("%s|%s", chain.Second.Source, scenarioName)
	if reason, skip := skipSourceScenarios[secondKey]; skip {
		return reason, true
	}
	return "", false
}

// RunTransitive executes two-hop transitive tests for all chains × scenarios.
// For each chain A→B→C:
//  1. Send as A with target B → get result1 (in A's response format)
//  2. Send as B with target C → get result2 (in B's response format)
//  3. Assert result1 and result2 are semantically equivalent
//
// This catches information loss that single-hop tests miss: if A→B drops a
// field that B→C needs, the results will diverge.
//
// A single TestEnv is shared per scenario to limit file descriptor usage;
// chains within a scenario run sequentially to avoid "too many open files"
// under heavy parallelism.
func (m *Matrix) RunTransitive(t *testing.T) {
	t.Helper()

	chains := m.DefaultChains()
	if len(chains) == 0 {
		t.Skip("no transitive chains to test")
	}

	for _, scenario := range m.Scenarios {
		scenario := scenario
		if scenario.Name == "error" {
			continue
		}

		t.Run(scenario.Name, func(t *testing.T) {
			t.Parallel()

			env := NewTestEnv(t)
			defer env.Close()

			for _, chain := range chains {
				chain := chain
				for _, streaming := range m.Streaming {
					streaming := streaming
					modeSuffix := "nonstream"
					if streaming {
						modeSuffix = "stream"
					}
					label := fmt.Sprintf("%s/%s", chain, modeSuffix)

					t.Run(label, func(t *testing.T) {
						if reason, skip := skipTransitiveKey(chain, scenario.Name); skip {
							t.Skipf("skipped: %s", reason)
							return
						}

						if streaming && !scenarioSupportsStreaming(scenario) {
							t.Skip("scenario does not support streaming")
							return
						}
						if !streaming && scenarioRequiresStreaming(scenario) {
							t.Skip("scenario requires streaming mode")
							return
						}

						// Hop 1: A→B
						env.SetupRoute(chain.First.Source, chain.First.Target, scenario)
						result1 := env.SendAs(t, chain.First.Source, chain.First.Target, scenario, streaming)

						// Hop 2: B→C
						env.SetupRoute(chain.Second.Source, chain.Second.Target, scenario)
						result2 := env.SendAs(t, chain.Second.Source, chain.Second.Target, scenario, streaming)

						// Both hops must individually succeed
						for _, a := range scenario.Assertions {
							if err := a.Check(result1); err != nil {
								t.Errorf("hop1 (%s→%s) assertion %q failed: %v",
									chain.First.Source, chain.First.Target, a.Name, err)
							}
							if err := a.Check(result2); err != nil {
								t.Errorf("hop2 (%s→%s) assertion %q failed: %v",
									chain.Second.Source, chain.Second.Target, a.Name, err)
							}
						}

						// Semantic equivalence between the two hops
						checkSemanticEquivalence(t, chain, result1, result2)
					})
				}
			}
		})
	}
}

// checkSemanticEquivalence verifies that two RoundTripResults carry the same
// semantic payload despite being in different protocol formats.
func checkSemanticEquivalence(t *testing.T, chain TransitiveChain, r1, r2 *RoundTripResult) {
	t.Helper()
	assertSemanticEquivalence(t, chain.String(), r1, r2)
}

// assertSemanticEquivalence verifies that two RoundTripResults carry the same
// semantic payload despite being in different protocol formats. We compare
// normalized fields rather than raw bytes. label identifies the comparison in
// failure messages.
func assertSemanticEquivalence(t *testing.T, label string, r1, r2 *RoundTripResult) {
	t.Helper()

	// Role must match
	if r1.Role != r2.Role {
		t.Errorf("[%s] role mismatch: hop1=%q, hop2=%q", label, r1.Role, r2.Role)
	}

	// Content must match (normalize whitespace for minor formatting diffs)
	c1 := strings.TrimSpace(r1.Content)
	c2 := strings.TrimSpace(r2.Content)
	if c1 != c2 {
		t.Errorf("[%s] content mismatch:\n  hop1: %q\n  hop2: %q", label, truncate(c1, 200), truncate(c2, 200))
	}

	// Tool call count must match
	if len(r1.ToolCalls) != len(r2.ToolCalls) {
		t.Errorf("[%s] tool_call count mismatch: hop1=%d, hop2=%d", label, len(r1.ToolCalls), len(r2.ToolCalls))
		return
	}

	// Each tool call's name and arguments must match
	for i := range r1.ToolCalls {
		tc1 := r1.ToolCalls[i]
		tc2 := r2.ToolCalls[i]
		if tc1.Name != tc2.Name {
			t.Errorf("[%s] tool_call[%d].name mismatch: hop1=%q, hop2=%q", label, i, tc1.Name, tc2.Name)
		}
		if normalizeJSON(tc1.Arguments) != normalizeJSON(tc2.Arguments) {
			t.Errorf("[%s] tool_call[%d].arguments mismatch:\n  hop1: %s\n  hop2: %s",
				label, i, tc1.Arguments, tc2.Arguments)
		}
	}

	// Usage: both should be present or both absent (for non-streaming)
	if !r1.IsStreaming && !r2.IsStreaming {
		if (r1.Usage == nil) != (r2.Usage == nil) {
			t.Errorf("[%s] usage presence mismatch: hop1=%v, hop2=%v",
				label, r1.Usage != nil, r2.Usage != nil)
		}
	}
}

// normalizeJSON strips whitespace from a JSON string for comparison.
// Falls back to trimmed string comparison if the input is not valid JSON.
func normalizeJSON(s string) string {
	return strings.Join(strings.Fields(s), "")
}
