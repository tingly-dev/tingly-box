package protocoltest

import (
	"fmt"
	"strings"
	"testing"
	"time"
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
		if scenario.SkipTransitive {
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
// semantic payload despite being in different protocol formats.
func assertSemanticEquivalence(t *testing.T, label string, r1, r2 *RoundTripResult) {
	t.Helper()
	for _, e := range semanticEquivalenceErrors(label, r1, r2) {
		t.Errorf("%s: %s", e.Assertion, e.Error)
	}
}

// normalizeJSON strips whitespace from a JSON string for comparison.
// Falls back to trimmed string comparison if the input is not valid JSON.
func normalizeJSON(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// ExecuteAllTransitive runs two-hop chain tests without requiring testing.T.
// It is the CLI-compatible counterpart of RunTransitive, returning []TestResult.
// Name format: "scenario/A→B→C/mode".
func (m *Matrix) ExecuteAllTransitive() []TestResult {
	var results []TestResult
	chains := m.DefaultChains()

	for _, scenario := range m.Scenarios {
		if scenario.SkipTransitive {
			continue
		}

		env, err := NewTestEnvForCLI(m.testEnvOpts()...)
		if err != nil {
			for _, chain := range chains {
				for _, streaming := range m.Streaming {
					results = append(results, TestResult{
						Name:      chain.TestName(scenario.Name, streaming),
						Scenario:  scenario.Name,
						Source:    chain.First.Source,
						Target:    chain.Second.Target,
						Streaming: streaming,
						Passed:    false,
						Errors:    []AssertionError{{Assertion: "setup", Error: fmt.Sprintf("failed to create test env: %v", err)}},
					})
				}
			}
			continue
		}

		for _, chain := range chains {
			for _, streaming := range m.Streaming {
				result := m.executeTransitiveChain(env, scenario, chain, streaming)
				results = append(results, result)
			}
		}
		env.Close()
	}
	return results
}

// executeTransitiveChain runs a single two-hop chain for a given scenario/mode.
func (m *Matrix) executeTransitiveChain(env *TestEnv, scenario Scenario, chain TransitiveChain, streaming bool) TestResult {
	name := chain.TestName(scenario.Name, streaming)
	base := TestResult{
		Name:      name,
		Scenario:  scenario.Name,
		Source:    chain.First.Source,
		Target:    chain.Second.Target,
		Streaming: streaming,
	}

	if reason, skip := skipTransitiveKey(chain, scenario.Name); skip {
		base.Skipped = true
		base.SkipReason = reason
		return base
	}
	if reason, skip := streamingSkipReason(scenario, streaming); skip {
		base.Skipped = true
		base.SkipReason = reason
		return base
	}

	start := time.Now()

	env.SetupRoute(chain.First.Source, chain.First.Target, scenario)
	r1, err := env.SendAsCLI(chain.First.Source, chain.First.Target, scenario, streaming)
	if err != nil {
		base.Passed = false
		base.Errors = []AssertionError{{Assertion: "hop1:send", Error: err.Error()}}
		base.Duration = time.Since(start)
		return base
	}

	env.SetupRoute(chain.Second.Source, chain.Second.Target, scenario)
	r2, err := env.SendAsCLI(chain.Second.Source, chain.Second.Target, scenario, streaming)
	if err != nil {
		base.Passed = false
		base.Errors = []AssertionError{{Assertion: "hop2:send", Error: err.Error()}}
		base.Duration = time.Since(start)
		return base
	}

	var errs []AssertionError
	for _, a := range scenario.Assertions {
		if checkErr := a.Check(r1); checkErr != nil {
			errs = append(errs, AssertionError{
				Assertion: "hop1:" + a.Name,
				Error:     checkErr.Error(),
				Context:   truncate(string(r1.RawBody), 300),
			})
		}
		if checkErr := a.Check(r2); checkErr != nil {
			errs = append(errs, AssertionError{
				Assertion: "hop2:" + a.Name,
				Error:     checkErr.Error(),
				Context:   truncate(string(r2.RawBody), 300),
			})
		}
	}
	errs = append(errs, semanticEquivalenceErrors(chain.String(), r1, r2)...)

	base.Passed = len(errs) == 0
	base.Errors = errs
	base.Duration = time.Since(start)
	base.HTTPStatus = r2.HTTPStatus
	base.Response = r2
	return base
}

// semanticEquivalenceErrors returns AssertionErrors for any semantic divergence
// between hop1 and hop2 results. It is the non-testing.T version of assertSemanticEquivalence.
func semanticEquivalenceErrors(label string, r1, r2 *RoundTripResult) []AssertionError {
	var errs []AssertionError
	add := func(name, msg string) {
		errs = append(errs, AssertionError{Assertion: "equiv:" + name, Error: fmt.Sprintf("[%s] %s", label, msg)})
	}

	if r1.Role != r2.Role {
		add("role", fmt.Sprintf("hop1=%q hop2=%q", r1.Role, r2.Role))
	}
	c1, c2 := strings.TrimSpace(r1.Content), strings.TrimSpace(r2.Content)
	if c1 != c2 {
		add("content", fmt.Sprintf("hop1=%q hop2=%q", truncate(c1, 200), truncate(c2, 200)))
	}
	if len(r1.ToolCalls) != len(r2.ToolCalls) {
		add("tool_call_count", fmt.Sprintf("hop1=%d hop2=%d", len(r1.ToolCalls), len(r2.ToolCalls)))
	} else {
		for i := range r1.ToolCalls {
			if r1.ToolCalls[i].Name != r2.ToolCalls[i].Name {
				add(fmt.Sprintf("tool_call[%d].name", i), fmt.Sprintf("hop1=%q hop2=%q", r1.ToolCalls[i].Name, r2.ToolCalls[i].Name))
			}
			if normalizeJSON(r1.ToolCalls[i].Arguments) != normalizeJSON(r2.ToolCalls[i].Arguments) {
				add(fmt.Sprintf("tool_call[%d].arguments", i), fmt.Sprintf("hop1=%s hop2=%s", r1.ToolCalls[i].Arguments, r2.ToolCalls[i].Arguments))
			}
		}
	}
	if !r1.IsStreaming && !r2.IsStreaming {
		if (r1.Usage == nil) != (r2.Usage == nil) {
			add("usage_presence", fmt.Sprintf("hop1=%v hop2=%v", r1.Usage != nil, r2.Usage != nil))
		}
	}
	return errs
}

// TestName builds a TestResult name for this chain in a given scenario/mode.
func (c TransitiveChain) TestName(scenario string, streaming bool) string {
	return fmt.Sprintf("%s/%s/%s", scenario, c, streamMode(streaming))
}
