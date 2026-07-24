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

// skipTransitiveKey reports whether either hop of a chain is in the
// single-hop known-defect list for the scenario.
func skipTransitiveKey(chain TransitiveChain, scenarioName string) (string, bool) {
	if reason, skip := KnownDefectReason(chain.First.Source, scenarioName); skip {
		return reason, true
	}
	return KnownDefectReason(chain.Second.Source, scenarioName)
}

// RunTransitive executes two-hop transitive tests for all chains × scenarios
// as subtests under t. For each chain A→B→C:
//  1. Send as A with target B → get result1 (in A's response format)
//  2. Send as B with target C → get result2 (in B's response format)
//  3. Assert result1 and result2 are semantically equivalent
//
// This catches information loss that single-hop tests miss: if A→B drops a
// field that B→C needs, the results will diverge.
//
// Each chain runs the same executeTransitiveChain implementation the CLI
// path uses; the testing.T layer (runPerScenario) only provisions one env per
// scenario and reports the TestResult.
func (m *Matrix) RunTransitive(t *testing.T) {
	t.Helper()

	chains := m.DefaultChains()
	if len(chains) == 0 {
		t.Skip("no transitive chains to test")
	}

	m.runPerScenario(t, skipTransitiveScenario, func(t *testing.T, env *TestEnv, scenario Scenario) {
		for _, chain := range chains {
			for _, streaming := range m.Streaming {
				t.Run(fmt.Sprintf("%s/%s", chain, streamMode(streaming)), func(t *testing.T) {
					result := m.executeTransitiveChain(env, scenario, chain, streaming)
					reportTestResult(t, &result)
				})
			}
		}
	})
}

// skipTransitiveScenario is the scenario-level opt-out shared by the
// transitive and idempotent sections: error / truncation scenarios produce no
// comparable output across hops.
func skipTransitiveScenario(s Scenario) bool {
	return s.SkipTransitive
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
	chains := m.DefaultChains()
	return m.executePerScenario(skipTransitiveScenario, func(s Scenario) []scenarioCombo {
		var combos []scenarioCombo
		for _, chain := range chains {
			for _, streaming := range m.Streaming {
				combos = append(combos, scenarioCombo{
					meta: chain.baseResult(s.Name, streaming),
					run: func(env *TestEnv) TestResult {
						return m.executeTransitiveChain(env, s, chain, streaming)
					},
				})
			}
		}
		return combos
	})
}

// baseResult returns a TestResult pre-filled with the fields that identify
// this chain in a given scenario/mode.
func (c TransitiveChain) baseResult(scenarioName string, streaming bool) TestResult {
	return TestResult{
		Name:      c.TestName(scenarioName, streaming),
		Scenario:  scenarioName,
		Source:    c.First.Source,
		Target:    c.Second.Target,
		Streaming: streaming,
	}
}

// executeTransitiveChain runs a single two-hop chain for a given scenario/mode.
func (m *Matrix) executeTransitiveChain(env *TestEnv, scenario Scenario, chain TransitiveChain, streaming bool) TestResult {
	base := chain.baseResult(scenario.Name, streaming)

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
