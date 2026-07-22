package protocoltest

import (
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// This file re-exports the reusable check-logic and scenario-fixture foundation
// (vmodel/benchmark/check and vmodel/benchmark/scenario) under the protocoltest
// names so existing call sites — here and in cli/harness — keep compiling
// unchanged. The canonical definitions now live in the foundation; protocoltest
// is a thin consumer. See .design/vmodel-benchmark.md.

// ─── check: result + assertion types ───────────────────────────────────────────

type (
	// RoundTripResult is the protocol-neutral view of one gateway round trip.
	RoundTripResult = check.RoundTripResult
	// ToolCallResult holds a single tool/function call extracted from a response.
	ToolCallResult = check.ToolCallResult
	// TokenUsage holds token counts extracted from a provider response.
	TokenUsage = check.TokenUsage
	// Assertion is a named check applied to a RoundTripResult.
	Assertion = check.Assertion
)

// ─── check: assertion library ──────────────────────────────────────────────────

var (
	AssertContentEquals        = check.AssertContentEquals
	AssertContentContains      = check.AssertContentContains
	AssertContentNonEmpty      = check.AssertContentNonEmpty
	AssertRoleEquals           = check.AssertRoleEquals
	AssertFinishReason         = check.AssertFinishReason
	AssertFinishReasonOneOf    = check.AssertFinishReasonOneOf
	AssertHasToolCalls         = check.AssertHasToolCalls
	AssertToolCallName         = check.AssertToolCallName
	AssertToolCallArgs         = check.AssertToolCallArgs
	AssertHasThinking          = check.AssertHasThinking
	AssertNoThinking           = check.AssertNoThinking
	AssertUsageNonZero         = check.AssertUsageNonZero
	AssertHTTPStatus           = check.AssertHTTPStatus
	AssertStreamEventCount     = check.AssertStreamEventCount
	AssertHTTPStatusAtLeast    = check.AssertHTTPStatusAtLeast
	AssertErrorMessageContains = check.AssertErrorMessageContains
	AssertModelContains        = check.AssertModelContains
	AssertStreamEventsContain  = check.AssertStreamEventsContain
	AssertFinishReasonNonEmpty = check.AssertFinishReasonNonEmpty
	AssertUsagePropagated      = check.AssertUsagePropagated
)

// ─── scenario: types + format constants ────────────────────────────────────────

type (
	// ResponseFormat selects which provider format a MockResponseBuilder serves.
	ResponseFormat = scenario.ResponseFormat
	// MockResponseBuilder defines how a virtual server responds for one format.
	MockResponseBuilder = scenario.MockResponseBuilder
	// Scenario is a named mock-provider fixture; implements vmodel.VirtualModel.
	Scenario = scenario.Scenario
)

const (
	FormatOpenAIChat      = scenario.FormatOpenAIChat
	FormatOpenAIResponses = scenario.FormatOpenAIResponses
	FormatAnthropic       = scenario.FormatAnthropic
	FormatGoogle          = scenario.FormatGoogle
)

// ─── scenario: built-in fixtures + helpers ─────────────────────────────────────

var (
	AllScenarios       = scenario.AllScenarios
	AllErrorScenarios  = scenario.AllErrorScenarios
	GetErrorSpec       = scenario.GetErrorSpec
	BuildErrorFromSpec = scenario.BuildErrorFromSpec

	TextScenario                = scenario.TextScenario
	ToolUseScenario             = scenario.ToolUseScenario
	ToolResultScenario          = scenario.ToolResultScenario
	ThinkingScenario            = scenario.ThinkingScenario
	MultiTurnScenario           = scenario.MultiTurnScenario
	StreamingTextScenario       = scenario.StreamingTextScenario
	StreamingToolUseScenario    = scenario.StreamingToolUseScenario
	IncompleteScenario          = scenario.IncompleteScenario
	ErrorScenario               = scenario.ErrorScenario
	Error500Scenario            = scenario.Error500Scenario
	ErrorAuth401Scenario        = scenario.ErrorAuth401Scenario
	ErrorMidStreamCloseScenario = scenario.ErrorMidStreamCloseScenario
)

// mustMarshal is retained as a protocoltest-local helper because it is used by
// testenv.go and flags.go to build request bodies. The scenario package has its
// own unexported copy.
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}
	return b
}
