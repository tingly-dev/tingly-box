package protocoltest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ProtocolPair is one (source → target) conversion path to validate.
// The matrix is built from an explicit list of pairs rather than the
// Cartesian product of all sources × all targets: many cells of that
// product map to the same dispatch path (e.g. target=anthropic_v1 and
// target=anthropic_beta both pick APIStyleAnthropic; OpenAI Chat vs.
// Responses targets are picked by ResolveOpenAIEndpoint, not the matrix)
// and listing pairs keeps the matrix in lock-step with the actual
// dispatch graph documented in internal/protocol/README.md.
type ProtocolPair struct {
	Source protocol.APIType
	Target protocol.APIType
}

// String returns "source|target" for use as a map key or label.
func (p ProtocolPair) String() string {
	return fmt.Sprintf("%s|%s", p.Source, p.Target)
}

// Matrix defines the set of (source, target) pairs, scenarios, and
// streaming modes to validate.
type Matrix struct {
	Pairs      []ProtocolPair
	Scenarios  []Scenario
	Streaming  []bool
	RecordDir  string // Optional directory for recording requests/responses
	BatchCount int    // Number of times to run each test
	MCPEnabled bool   // Enable MCP feature flag in test env
	Client     Client // Client driver (nil = raw HTTP default)
}

// DefaultPairs is the canonical list of (source → target) conversion
// paths the matrix exercises. Adding a new dispatch path means appending
// to this list.
//
// Notes:
//   - target=anthropic_v1 is intentionally absent. The harness picks
//     providers by APIStyle and both Anthropic types map to the same
//     style, so anthropic_beta as the target already exercises both
//     Anthropic V1 passthrough (when source is V1) and the Beta
//     conversions (when source is non-Anthropic). See
//     internal/protocol/README.md.
//   - Anthropic↔Anthropic cross-version (v1↔beta) is rejected by the
//     transform layer and not represented here.
//   - Google targets and the google→google passthrough are not yet
//     supported by the harness's virtual provider plumbing.
func DefaultPairs() []ProtocolPair {
	return []ProtocolPair{
		// Anthropic V1 source
		{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta},   // V1 passthrough (provider APIStyle=Anthropic)
		{protocol.TypeAnthropicV1, protocol.TypeOpenAIChat},      // V1 → OpenAI Chat
		{protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses}, // V1 → OpenAI Responses

		// Anthropic Beta source
		{protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta},   // Beta passthrough
		{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat},      // Beta → OpenAI Chat
		{protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses}, // Beta → OpenAI Responses

		// OpenAI Chat source
		{protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta},   // Chat → Anthropic Beta
		{protocol.TypeOpenAIChat, protocol.TypeOpenAIChat},      // Chat passthrough
		{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses}, // Chat → Responses

		// OpenAI Responses source
		{protocol.TypeOpenAIResponses, protocol.TypeAnthropicBeta},   // Responses → Anthropic Beta
		{protocol.TypeOpenAIResponses, protocol.TypeOpenAIChat},      // Responses → Chat
		{protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses}, // Responses passthrough
	}
}

// DefaultMatrix returns the full validation matrix covering every
// supported (source, target) pair, all built-in scenarios, and both
// streaming modes.
func DefaultMatrix() *Matrix {
	return &Matrix{
		Pairs:     DefaultPairs(),
		Scenarios: AllScenarios(),
		Streaming: []bool{false, true},
	}
}

// clone returns a shallow copy of the matrix.
func (m *Matrix) clone() *Matrix {
	cp := *m
	return &cp
}

// OnlyScenarios returns a copy of the Matrix filtered to only the named scenarios.
func (m *Matrix) OnlyScenarios(names ...string) *Matrix {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	filtered := make([]Scenario, 0, len(names))
	for _, s := range m.Scenarios {
		if nameSet[s.Name] {
			filtered = append(filtered, s)
		}
	}

	out := m.clone()
	out.Scenarios = filtered
	return out
}

// OnlySources returns a copy of the Matrix filtered to pairs whose source
// matches one of the given protocols.
func (m *Matrix) OnlySources(sources ...string) *Matrix {
	sourceSet := make(map[protocol.APIType]bool, len(sources))
	for _, s := range sources {
		sourceSet[protocol.APIType(s)] = true
	}

	filtered := make([]ProtocolPair, 0, len(m.Pairs))
	for _, p := range m.Pairs {
		if sourceSet[p.Source] {
			filtered = append(filtered, p)
		}
	}

	out := m.clone()
	out.Pairs = filtered
	return out
}

// OnlyTargets returns a copy of the Matrix filtered to pairs whose target
// matches one of the given protocols.
func (m *Matrix) OnlyTargets(targets ...string) *Matrix {
	targetSet := make(map[protocol.APIType]bool, len(targets))
	for _, t := range targets {
		targetSet[protocol.APIType(t)] = true
	}

	filtered := make([]ProtocolPair, 0, len(m.Pairs))
	for _, p := range m.Pairs {
		if targetSet[p.Target] {
			filtered = append(filtered, p)
		}
	}

	out := m.clone()
	out.Pairs = filtered
	return out
}

// OnlyStreaming returns a copy of the Matrix filtered to only streaming or non-streaming tests.
// If streaming is true, only streaming tests are included. If false, only non-streaming tests.
func (m *Matrix) OnlyStreaming(streaming bool) *Matrix {
	out := m.clone()
	out.Streaming = []bool{streaming}
	return out
}

// WithRecordDir returns a copy of the Matrix with the record directory set.
// If recordDir is empty, recording is disabled.
func (m *Matrix) WithRecordDir(recordDir string) *Matrix {
	out := m.clone()
	out.RecordDir = recordDir
	return out
}

// WithBatchCount returns a copy of the Matrix with the batch count set.
func (m *Matrix) WithBatchCount(count int) *Matrix {
	out := m.clone()
	out.BatchCount = count
	return out
}

// WithMCPEnabled returns a copy of the Matrix with the MCP feature flag enabled.
func (m *Matrix) WithMCPEnabled() *Matrix {
	out := m.clone()
	out.MCPEnabled = true
	return out
}

// WithClient returns a copy of the Matrix that drives requests through the
// given client driver (official SDKs, subprocess drivers) instead of the
// default raw HTTP client.
func (m *Matrix) WithClient(c Client) *Matrix {
	out := m.clone()
	out.Client = c
	return out
}

// testEnvOpts returns the TestEnvOptions to apply when creating a TestEnv for this matrix.
func (m *Matrix) testEnvOpts() []TestEnvOption {
	var opts []TestEnvOption
	opts = append(opts, NewTestEnvOptionWithRecordDir(m.RecordDir))
	if m.MCPEnabled {
		opts = append(opts, NewTestEnvOptionWithMCP())
	}
	if m.Client != nil {
		opts = append(opts, NewTestEnvOptionWithClient(m.Client))
	}
	return opts
}

// skipSourceScenarios is the single registry of source+scenario combinations
// that are known to be broken. Every entry is a real gateway defect, not a
// test artifact; remove it when the defect is fixed. All tiers derive their
// skips from this map (the matrix directly, replay via KnownDefectReason), so
// closing a defect is a one-line deletion.
var skipSourceScenarios = map[string]string{
	// openai_responses source: tool_call conversion from provider back to Responses format loses tool calls
	"openai_responses|tool_use":           "Responses API source: tool_use conversion incomplete",
	"openai_responses|streaming_tool_use": "Responses API source: streaming tool_use conversion incomplete",
}

// KnownDefectReason reports whether a (source protocol, scenario) combination
// is in the known-defect registry, and why. Consumers outside the matrix
// (e.g. `harness replay`) use this to skip runs that exercise a documented
// gateway bug instead of keeping their own copy of the list.
func KnownDefectReason(source protocol.APIType, scenarioName string) (string, bool) {
	reason, skip := skipSourceScenarios[fmt.Sprintf("%s|%s", source, scenarioName)]
	return reason, skip
}

// clientSkipScenarios lists client|source|scenario[|mode] combinations that are
// known incompatibilities between a specific client driver and a scenario.
// Keeping them here makes them visible skips instead of silent failures;
// every entry should describe the real finding it papers over.
var clientSkipScenarios = map[string]string{}

// clientSkipReason returns a skip reason when the matrix's client driver
// cannot run the given source/scenario combination.
func (m *Matrix) clientSkipReason(source protocol.APIType, scenarioName string) (string, bool) {
	if m.Client == nil {
		return "", false
	}
	if !m.Client.Supports(source) {
		return fmt.Sprintf("client %q does not support source protocol %s", m.Client.Name(), source), true
	}
	key := fmt.Sprintf("%s|%s|%s", m.Client.Name(), source, scenarioName)
	if reason, ok := clientSkipScenarios[key]; ok {
		return reason, true
	}
	return "", false
}

// RunFull executes both single-hop and two-hop tests under t, organized as
// two named sub-sections:
//
//   - "single_hop": every (source→target) pair × scenario × streaming mode
//   - "two_hop":    every (A→B→C) transitive chain × scenario × streaming mode
//
// Run each section independently with -run TestFoo/single_hop or /two_hop.
func (m *Matrix) RunFull(t *testing.T) {
	t.Helper()
	t.Run("single_hop", func(t *testing.T) {
		t.Helper()
		m.Run(t)
	})
	t.Run("two_hop", func(t *testing.T) {
		t.Helper()
		m.RunTransitive(t)
	})
}

// Run executes all matrix combinations as subtests under t. Each combination
// runs the same executeTest implementation the CLI path uses — the testing.T
// layer only provisions an env per subtest and reports the TestResult, so the
// skip logic and assertion loop exist exactly once.
func (m *Matrix) Run(t *testing.T) {
	t.Helper()

	for _, scenario := range m.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			for _, pair := range m.Pairs {
				t.Run(string(pair.Source), func(t *testing.T) {
					t.Run(string(pair.Target), func(t *testing.T) {
						for _, streaming := range m.Streaming {
							t.Run(streamMode(streaming), func(t *testing.T) {
								t.Parallel()

								env, err := NewTestEnvForCLI(m.testEnvOpts()...)
								if err != nil {
									t.Fatalf("create test env: %v", err)
								}
								defer env.Close()

								reportTestResult(t, m.executeTest(env, scenario, pair.Source, pair.Target, streaming))
							})
						}
					})
				})
			}
		})
	}
}

// reportTestResult surfaces a CLI-shaped TestResult under t — the bridge that
// lets the go-test entry points share the CLI execution path instead of
// re-implementing setup, skip logic, and assertion loops.
func reportTestResult(t *testing.T, r *TestResult) {
	t.Helper()
	if r == nil {
		return
	}
	if r.Skipped {
		t.Skipf("skipped: %s", r.SkipReason)
	}
	for _, e := range r.Errors {
		if e.Context != "" {
			t.Errorf("%s: %s\n  body: %s", e.Assertion, e.Error, e.Context)
		} else {
			t.Errorf("%s: %s", e.Assertion, e.Error)
		}
	}
}

// streamMode returns "stream" or "nonstream" for use in test names.
func streamMode(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "nonstream"
}

// streamingSkipReason returns a non-empty reason string when a scenario/mode
// combination should be skipped due to streaming incompatibility.
func streamingSkipReason(scenario Scenario, streaming bool) (string, bool) {
	if streaming && !scenarioSupportsStreaming(scenario) {
		return "scenario does not support streaming", true
	}
	if !streaming && scenarioRequiresStreaming(scenario) {
		return "scenario requires streaming mode", true
	}
	return "", false
}

// scenarioSupportsStreaming returns true if the scenario has streaming mock responses.
func scenarioSupportsStreaming(s Scenario) bool {
	for _, builder := range s.MockResponses {
		if builder.Stream != nil {
			return true
		}
	}
	return false
}

// scenarioRequiresStreaming returns true if the scenario has streaming-specific assertions.
func scenarioRequiresStreaming(s Scenario) bool {
	for _, a := range s.Assertions {
		if strings.Contains(a.Name, "stream_event_count") {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ExecuteAll runs all matrix combinations and returns structured results.
// This is a pure function that can be called from both tests and CLI.
// It does not use testing.T, making it suitable for standalone execution.
//
// One TestEnv is created per scenario and reused for all of its combinations
// — routes are keyed by (source, target, scenario), so combinations within a
// scenario cannot interfere, and env boots stay off the hot path.
func (m *Matrix) ExecuteAll() []TestResult {
	var results []TestResult

	// For each scenario, create one TestEnv and reuse it for all combinations
	for _, scenario := range m.Scenarios {
		scenario := scenario

		// Create TestEnv for this scenario
		env, err := NewTestEnvForCLI(m.testEnvOpts()...)
		if err != nil {
			// All tests for this scenario fail with setup error
			for _, pair := range m.Pairs {
				for _, streaming := range m.Streaming {
					results = append(results, TestResult{
						Name:      m.buildTestName(scenario.Name, pair.Source, pair.Target, streaming),
						Scenario:  scenario.Name,
						Source:    pair.Source,
						Target:    pair.Target,
						Streaming: streaming,
						Passed:    false,
						Errors: []AssertionError{{
							Assertion: "setup",
							Error:     fmt.Sprintf("failed to create test env: %v", err),
						}},
					})
				}
			}
			continue
		}
		defer env.Close()

		// Run all combinations for this scenario
		for _, pair := range m.Pairs {
			for _, streaming := range m.Streaming {
				if result := m.executeTest(env, scenario, pair.Source, pair.Target, streaming); result != nil {
					results = append(results, *result)
				}
			}
		}

		// Close env explicitly after processing all combinations for this scenario
		env.Close()
	}

	return results
}

// executeTest executes a single test with the given environment.
// Returns nil if the test should be skipped.
func (m *Matrix) executeTest(env *TestEnv, scenario Scenario, source, target protocol.APIType, streaming bool) *TestResult {
	srcScenarioKey := fmt.Sprintf("%s|%s", source, scenario.Name)
	if reason, skip := skipSourceScenarios[srcScenarioKey]; skip {
		return &TestResult{
			Name:       m.buildTestName(scenario.Name, source, target, streaming),
			Scenario:   scenario.Name,
			Source:     source,
			Target:     target,
			Streaming:  streaming,
			Skipped:    true,
			SkipReason: reason,
		}
	}

	if reason, skip := m.clientSkipReason(source, scenario.Name); skip {
		return &TestResult{
			Name:       m.buildTestName(scenario.Name, source, target, streaming),
			Scenario:   scenario.Name,
			Source:     source,
			Target:     target,
			Streaming:  streaming,
			Skipped:    true,
			SkipReason: reason,
		}
	}

	if reason, skip := streamingSkipReason(scenario, streaming); skip {
		return &TestResult{
			Name:       m.buildTestName(scenario.Name, source, target, streaming),
			Scenario:   scenario.Name,
			Source:     source,
			Target:     target,
			Streaming:  streaming,
			Skipped:    true,
			SkipReason: reason,
		}
	}

	// Execute test (with batch if requested)
	if m.BatchCount > 1 {
		return m.executeBatch(env, scenario, source, target, streaming)
	}

	result := m.executeOneWithEnv(env, scenario, source, target, streaming)
	return &result
}

// executeOneWithEnv runs a single test combination using the provided TestEnv.
// Used by ExecuteAll for optimized batch testing.
func (m *Matrix) executeOneWithEnv(env *TestEnv, s Scenario, source, target protocol.APIType, streaming bool) TestResult {
	start := time.Now()

	env.SetupRoute(source, target, s)
	result, err := env.SendAsCLI(source, target, s, streaming)
	if err != nil {
		return TestResult{
			Name:      m.buildTestName(s.Name, source, target, streaming),
			Scenario:  s.Name,
			Source:    source,
			Target:    target,
			Streaming: streaming,
			Passed:    false,
			Errors: []AssertionError{{
				Assertion: "send",
				Error:     fmt.Sprintf("failed to send request: %v", err),
			}},
			Duration: time.Since(start),
		}
	}

	// Check assertions
	var errors []AssertionError
	passed := true
	for _, a := range s.Assertions {
		if err := a.Check(result); err != nil {
			passed = false
			errors = append(errors, AssertionError{
				Assertion: a.Name,
				Error:     err.Error(),
				Context:   truncate(string(result.RawBody), 300),
			})
		}
	}

	return TestResult{
		Name:       m.buildTestName(s.Name, source, target, streaming),
		Scenario:   s.Name,
		Source:     source,
		Target:     target,
		Streaming:  streaming,
		Passed:     passed,
		Errors:     errors,
		Duration:   time.Since(start),
		HTTPStatus: result.HTTPStatus,
		Response:   result,
	}
}

// buildTestName constructs a test name from its components.
func (m *Matrix) buildTestName(scenario string, source, target protocol.APIType, streaming bool) string {
	return fmt.Sprintf("%s/%s/%s/%s", scenario, source, target, streamMode(streaming))
}

// executeBatch runs a test multiple times and aggregates the results.
func (m *Matrix) executeBatch(env *TestEnv, scenario Scenario, source, target protocol.APIType, streaming bool) *TestResult {
	name := m.buildTestName(scenario.Name, source, target, streaming)

	var results []TestResult
	var totalDur time.Duration
	var minDur, maxDur time.Duration
	passedCount := 0
	uniqueErrors := make(map[string]bool)

	// Run the test N times
	for i := 0; i < m.BatchCount; i++ {
		result := m.executeOneWithEnv(env, scenario, source, target, streaming)
		results = append(results, result)

		totalDur += result.Duration
		if i == 0 || result.Duration < minDur {
			minDur = result.Duration
		}
		if i == 0 || result.Duration > maxDur {
			maxDur = result.Duration
		}

		if result.Passed {
			passedCount++
		} else {
			for _, e := range result.Errors {
				uniqueErrors[e.Error] = true
			}
		}
	}

	// Build aggregated result
	var errorList []string
	for err := range uniqueErrors {
		errorList = append(errorList, err)
	}

	avgDur := totalDur / time.Duration(m.BatchCount)
	allPassed := passedCount == m.BatchCount

	// Use the last result's response for debugging (or first if preferred)
	var response *RoundTripResult
	if len(results) > 0 {
		response = results[len(results)-1].Response
	}

	return &TestResult{
		Name:        name,
		Scenario:    scenario.Name,
		Source:      source,
		Target:      target,
		Streaming:   streaming,
		Passed:      allPassed,
		Skipped:     false,
		Duration:    avgDur, // Use average for primary duration
		BatchCount:  m.BatchCount,
		BatchPassed: passedCount,
		BatchMinDur: minDur,
		BatchAvgDur: avgDur,
		BatchMaxDur: maxDur,
		BatchErrors: errorList,
		HTTPStatus:  0, // Not meaningful for batch
		Response:    response,
	}
}
