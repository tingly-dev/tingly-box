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
	Sources   []protocol.APIType
	Targets   []protocol.APIType
	Scenarios []Scenario
	Streaming []bool
}

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
		Sources:   m.Sources,
		Targets:   m.Targets,
		Scenarios: filtered,
		Streaming: m.Streaming,
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

									result := env.SendAs(t, source, scenario, streaming)

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
// Optimization: Reuses TestEnv per scenario to reduce server startup overhead.
// Each scenario creates one TestEnv that is reused for all its combinations.
func (m *Matrix) ExecuteAll() []TestResult {
	var results []TestResult

	// For each scenario, create one TestEnv and reuse it for all combinations
	for _, scenario := range m.Scenarios {
		scenario := scenario

		// Create TestEnv for this scenario
		env, err := NewTestEnvForCLI()
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

		// Run all combinations for this scenario
		for _, source := range m.Sources {
			source := source
			for _, target := range m.Targets {
				target := target
				for _, streaming := range m.Streaming {
					streaming := streaming

					// Check skip conditions first
					pairKey := fmt.Sprintf("%s|%s", source, target)
					if reason, skip := skipPairs[pairKey]; skip {
						results = append(results, TestResult{
							Name:       m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:   scenario.Name,
							Source:     source,
							Target:     target,
							Streaming:  streaming,
							Skipped:    true,
							SkipReason: reason,
						})
						continue
					}

					srcScenarioKey := fmt.Sprintf("%s|%s", source, scenario.Name)
					if reason, skip := skipSourceScenarios[srcScenarioKey]; skip {
						results = append(results, TestResult{
							Name:       m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:   scenario.Name,
							Source:     source,
							Target:     target,
							Streaming:  streaming,
							Skipped:    true,
							SkipReason: reason,
						})
						continue
					}

					// Check streaming compatibility
					if streaming && !scenarioSupportsStreaming(scenario) {
						results = append(results, TestResult{
							Name:       m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:   scenario.Name,
							Source:     source,
							Target:     target,
							Streaming:  streaming,
							Skipped:    true,
							SkipReason: "scenario does not support streaming",
						})
						continue
					}

					if !streaming && scenarioRequiresStreaming(scenario) {
						results = append(results, TestResult{
							Name:       m.buildTestName(scenario.Name, source, target, streaming),
							Scenario:   scenario.Name,
							Source:     source,
							Target:     target,
							Streaming:  streaming,
							Skipped:    true,
							SkipReason: "scenario requires streaming mode",
						})
						continue
					}

					// Execute test with shared env
					result := m.executeOneWithEnv(env, scenario, source, target, streaming)
					results = append(results, result)
				}
			}
		}

		// Close env explicitly after processing all combinations for this scenario
		env.Close()
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
	result, err := env.SendAsCLI(source, s, streaming)
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
	result, err := env.SendAsCLI(source, s, streaming)
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
