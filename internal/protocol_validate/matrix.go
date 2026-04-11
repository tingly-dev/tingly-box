package protocol_validate

import (
	"fmt"
	"strings"
	"testing"

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
