package protocol_validate

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Matrix defines the cross-product of source protocols, target protocols,
// scenarios, and streaming modes to validate.
type Matrix struct {
	Sources    []protocol.APIType
	Targets    []protocol.APIType
	Scenarios  []Scenario
	Streaming  []bool
	RecordDir  string // Optional directory for recording requests/responses
	ServerMode string // Server reuse mode: auto, all, pair
	BatchCount int    // Number of times to run each test
}

// ServerMode constants
const (
	ServerModeAuto = "auto" // Per-scenario (default)
	ServerModeAll  = "all"  // Single server for all tests
	ServerModePair = "pair" // Per source-target pair
)

// DefaultMatrix returns the full validation matrix covering all supported
// protocol combinations, all built-in scenarios, and both streaming modes.
func DefaultMatrix() *Matrix {
	return &Matrix{
		Sources: []protocol.APIType{
			protocol.TypeAnthropicV1,
			protocol.TypeAnthropicBeta,
			protocol.TypeOpenAIChat,
			protocol.TypeOpenAIResponses,
		},
		Targets: []protocol.APIType{
			protocol.TypeAnthropicV1,
			protocol.TypeAnthropicBeta,
			protocol.TypeOpenAIChat,
			protocol.TypeOpenAIResponses,
			protocol.TypeGoogle,
		},
		Scenarios: AllScenarios(),
		Streaming: []bool{false, true},
	}
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

	return &Matrix{
		Sources:    m.Sources,
		Targets:    m.Targets,
		Scenarios:  filtered,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: m.BatchCount,
	}
}

// OnlySources returns a copy of the Matrix filtered to only the specified source protocols.
func (m *Matrix) OnlySources(sources ...string) *Matrix {
	sourceSet := make(map[protocol.APIType]bool, len(sources))
	for _, s := range sources {
		sourceSet[protocol.APIType(s)] = true
	}

	filtered := make([]protocol.APIType, 0, len(sources))
	for _, s := range m.Sources {
		if sourceSet[s] {
			filtered = append(filtered, s)
		}
	}

	return &Matrix{
		Sources:    filtered,
		Targets:    m.Targets,
		Scenarios:  m.Scenarios,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: m.BatchCount,
	}
}

// OnlyTargets returns a copy of the Matrix filtered to only the specified target protocols.
func (m *Matrix) OnlyTargets(targets ...string) *Matrix {
	targetSet := make(map[protocol.APIType]bool, len(targets))
	for _, t := range targets {
		targetSet[protocol.APIType(t)] = true
	}

	filtered := make([]protocol.APIType, 0, len(targets))
	for _, t := range m.Targets {
		if targetSet[t] {
			filtered = append(filtered, t)
		}
	}

	return &Matrix{
		Sources:    m.Sources,
		Targets:    filtered,
		Scenarios:  m.Scenarios,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: m.BatchCount,
	}
}

// OnlyStreaming returns a copy of the Matrix filtered to only streaming or non-streaming tests.
// If streaming is true, only streaming tests are included. If false, only non-streaming tests.
func (m *Matrix) OnlyStreaming(streaming bool) *Matrix {
	filtered := []bool{streaming}
	return &Matrix{
		Sources:    m.Sources,
		Targets:    m.Targets,
		Scenarios:  m.Scenarios,
		Streaming:  filtered,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: m.BatchCount,
	}
}

// WithRecordDir returns a copy of the Matrix with the record directory set.
// If recordDir is empty, recording is disabled.
func (m *Matrix) WithRecordDir(recordDir string) *Matrix {
	return &Matrix{
		Sources:    m.Sources,
		Targets:    m.Targets,
		Scenarios:  m.Scenarios,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: m.BatchCount,
	}
}

// WithServerMode returns a copy of the Matrix with the server reuse mode set.
func (m *Matrix) WithServerMode(mode string) *Matrix {
	return &Matrix{
		Sources:    m.Sources,
		Targets:    m.Targets,
		Scenarios:  m.Scenarios,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: mode,
		BatchCount: m.BatchCount,
	}
}

// WithBatchCount returns a copy of the Matrix with the batch count set.
func (m *Matrix) WithBatchCount(count int) *Matrix {
	return &Matrix{
		Sources:    m.Sources,
		Targets:    m.Targets,
		Scenarios:  m.Scenarios,
		Streaming:  m.Streaming,
		RecordDir:  m.RecordDir,
		ServerMode: m.ServerMode,
		BatchCount: count,
	}
}

// skipPairs lists source→target combinations that are known to be unsupported.
// Tests for these pairs are skipped, not failed.
var skipPairs = map[string]string{
	// OpenAI Responses → Google: not yet implemented
	"openai_responses|google": "Responses API to Google not yet implemented",
	// Google target: vendor_adjust transform does not support OpenAI/Anthropic→Google yet
	"anthropic_v1|google":   "Anthropic→Google target not yet implemented",
	"anthropic_beta|google": "Anthropic→Google target not yet implemented",
	"openai_chat|google":    "OpenAI Chat→Google target not yet implemented",
}

// skipSourceScenarios lists source+scenario combinations that are known to be broken.
var skipSourceScenarios = map[string]string{
	// openai_responses source: tool_call conversion from provider back to Responses format loses tool calls
	"openai_responses|tool_use":           "Responses API source: tool_use conversion incomplete",
	"openai_responses|streaming_tool_use": "Responses API source: streaming tool_use conversion incomplete",
}

// Run executes all matrix combinations as subtests under t.
// Each combination runs in its own TestEnv so state is isolated.
func (m *Matrix) Run(t *testing.T) {
	t.Helper()

	for _, scenario := range m.Scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			for _, source := range m.Sources {
				source := source
				t.Run(string(source), func(t *testing.T) {
					for _, target := range m.Targets {
						target := target
						t.Run(string(target), func(t *testing.T) {
							for _, streaming := range m.Streaming {
								streaming := streaming
								modeSuffix := "nonstream"
								if streaming {
									modeSuffix = "stream"
								}
								t.Run(modeSuffix, func(t *testing.T) {
									t.Parallel()

									pairKey := fmt.Sprintf("%s|%s", source, target)
									if reason, skip := skipPairs[pairKey]; skip {
										t.Skipf("skipped: %s", reason)
										return
									}
									srcScenarioKey := fmt.Sprintf("%s|%s", source, scenario.Name)
									if reason, skip := skipSourceScenarios[srcScenarioKey]; skip {
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

									env := NewTestEnv(t)
									defer env.Close()

									env.SetupRoute(source, target, scenario)

									result := env.SendAs(t, source, target, scenario, streaming)

									for _, a := range scenario.Assertions {
										if err := a.Check(result); err != nil {
											t.Errorf("assertion %q failed: %v\n  body: %s",
												a.Name, err, truncate(string(result.RawBody), 300))
										}
									}
								})
							}
						})
					}
				})
			}
		})
	}
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
// Server reuse strategies based on ServerMode:
// - auto (default): Reuses TestEnv per scenario
// - all: Uses a single TestEnv for all tests (fastest, potential for interference)
// - pair: Reuses TestEnv per source-target pair (balanced approach)
func (m *Matrix) ExecuteAll() []TestResult {
	var results []TestResult

	switch m.ServerMode {
	case ServerModeAll:
		results = m.executeAllWithSingleServer()
	case ServerModePair:
		results = m.executeAllWithPairServer()
	default: // ServerModeAuto
		results = m.executeAllWithScenarioServer()
	}

	return results
}

// executeAllWithScenarioServer executes tests with per-scenario server reuse (default).
func (m *Matrix) executeAllWithScenarioServer() []TestResult {
	var results []TestResult

	// For each scenario, create one TestEnv and reuse it for all combinations
	for _, scenario := range m.Scenarios {
		scenario := scenario

		// Create TestEnv for this scenario
		env, err := NewTestEnvForCLI(NewTestEnvOptionWithRecordDir(m.RecordDir))
		if err != nil {
			// All tests for this scenario fail with setup error
			for _, source := range m.Sources {
				for _, target := range m.Targets {
					for _, streaming := range m.Streaming {
						results = append(results, TestResult{
							Name:      m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:  scenario.Name,
							Source:    source,
							Target:    target,
							Streaming: streaming,
							Passed:    false,
							Errors: []AssertionError{{
								Assertion: "setup",
								Error:     fmt.Sprintf("failed to create test env: %v", err),
							}},
						})
					}
				}
			}
			continue
		}
		defer env.Close()

		// Run all combinations for this scenario
		for _, source := range m.Sources {
			for _, target := range m.Targets {
				for _, streaming := range m.Streaming {
					if result := m.executeTest(env, scenario, source, target, streaming); result != nil {
						results = append(results, *result)
					}
				}
			}
		}

		// Close env explicitly after processing all combinations for this scenario
		env.Close()
	}

	return results
}

// executeAllWithSingleServer executes all tests with a single server.
func (m *Matrix) executeAllWithSingleServer() []TestResult {
	var results []TestResult

	// Create a single TestEnv for all tests
	env, err := NewTestEnvForCLI(NewTestEnvOptionWithRecordDir(m.RecordDir))
	if err != nil {
		// All tests fail with setup error
		return m.allSetupError(err)
	}
	defer env.Close()

	// Run all combinations
	for _, scenario := range m.Scenarios {
		for _, source := range m.Sources {
			for _, target := range m.Targets {
				for _, streaming := range m.Streaming {
					if result := m.executeTest(env, scenario, source, target, streaming); result != nil {
						results = append(results, *result)
					}
				}
			}
		}
	}

	return results
}

// executeAllWithPairServer executes tests with per source-target pair server reuse.
func (m *Matrix) executeAllWithPairServer() []TestResult {
	var results []TestResult

	// For each source-target pair, create one TestEnv
	for _, source := range m.Sources {
		for _, target := range m.Targets {
			pairKey := fmt.Sprintf("%s|%s", source, target)

			// Check if this pair is skipped
			if reason, skip := skipPairs[pairKey]; skip {
				// All scenarios for this pair are skipped
				for _, scenario := range m.Scenarios {
					for _, streaming := range m.Streaming {
						results = append(results, TestResult{
							Name:       m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:   scenario.Name,
							Source:     source,
							Target:     target,
							Streaming:  streaming,
							Skipped:    true,
							SkipReason: reason,
						})
					}
				}
				continue
			}

			// Create TestEnv for this pair
			env, err := NewTestEnvForCLI(NewTestEnvOptionWithRecordDir(m.RecordDir))
			if err != nil {
				// All tests for this pair fail with setup error
				for _, scenario := range m.Scenarios {
					for _, streaming := range m.Streaming {
						results = append(results, TestResult{
							Name:      m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:  scenario.Name,
							Source:    source,
							Target:    target,
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

			// Run all scenarios for this pair
			for _, scenario := range m.Scenarios {
				for _, streaming := range m.Streaming {
					if result := m.executeTest(env, scenario, source, target, streaming); result != nil {
						results = append(results, *result)
					}
				}
			}

			env.Close()
		}
	}

	return results
}

// executeTest executes a single test with the given environment.
// Returns nil if the test should be skipped.
func (m *Matrix) executeTest(env *TestEnv, scenario Scenario, source, target protocol.APIType, streaming bool) *TestResult {
	// Check skip conditions first
	pairKey := fmt.Sprintf("%s|%s", source, target)
	if reason, skip := skipPairs[pairKey]; skip {
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

	// Check streaming compatibility
	if streaming && !scenarioSupportsStreaming(scenario) {
		return &TestResult{
			Name:       m.buildTestName(scenario.Name, source, target, streaming),
			Scenario:   scenario.Name,
			Source:     source,
			Target:     target,
			Streaming:  streaming,
			Skipped:    true,
			SkipReason: "scenario does not support streaming",
		}
	}

	if !streaming && scenarioRequiresStreaming(scenario) {
		return &TestResult{
			Name:       m.buildTestName(scenario.Name, source, target, streaming),
			Scenario:   scenario.Name,
			Source:     source,
			Target:     target,
			Streaming:  streaming,
			Skipped:    true,
			SkipReason: "scenario requires streaming mode",
		}
	}

	// Execute test (with batch if requested)
	if m.BatchCount > 1 {
		return m.executeBatch(env, scenario, source, target, streaming)
	}

	result := m.executeOneWithEnv(env, scenario, source, target, streaming)
	return &result
}

// allSetupError returns results all marked as setup errors.
func (m *Matrix) allSetupError(err error) []TestResult {
	var results []TestResult
	for _, scenario := range m.Scenarios {
		for _, source := range m.Sources {
			for _, target := range m.Targets {
				for _, streaming := range m.Streaming {
					results = append(results, TestResult{
						Name:      m.buildTestName(scenario.Name, source, target, streaming),
						Scenario:  scenario.Name,
						Source:    source,
						Target:    target,
						Streaming: streaming,
						Passed:    false,
						Errors: []AssertionError{{
							Assertion: "setup",
							Error:     fmt.Sprintf("failed to create test env: %v", err),
						}},
					})
				}
			}
		}
	}
	return results
}

// executeOne runs a single test combination and returns the result.
// Creates a new TestEnv for this test only.
func (m *Matrix) executeOne(s Scenario, source, target protocol.APIType, streaming bool) TestResult {
	start := time.Now()

	// Create test environment
	env, err := NewTestEnvForCLI()
	if err != nil {
		return TestResult{
			Name:      m.buildTestName(s.Name, source, target, streaming),
			Scenario:  s.Name,
			Source:    source,
			Target:    target,
			Streaming: streaming,
			Passed:    false,
			Errors: []AssertionError{{
				Assertion: "setup",
				Error:     fmt.Sprintf("failed to create test env: %v", err),
			}},
		}
	}
	defer env.Close()

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
	mode := "nonstream"
	if streaming {
		mode = "stream"
	}
	return fmt.Sprintf("%s/%s/%s/%s", scenario, source, target, mode)
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
