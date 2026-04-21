// Package protocoltest provides a framework for end-to-end validation of the model
// gateway's protocol transformation layer.
//
// # Architecture
//
//  1. server_validate.VirtualServer — a mock HTTP provider that speaks OpenAI, Anthropic,
//     and Google response formats. Conceptually a "virtual model" for testing.
//
//  2. TestEnv — wires a real gateway Server (with transform pipeline) to a
//     VirtualServer, configures routing rules, and provides SendAs() for round-trip testing.
//
//  3. Matrix — executes the full cross-product of sources × targets × scenarios × streaming modes.
//
// Note: The existing internal/virtualmodel package is a production Gin server.
// This package (protocoltest) is the test-only framework. Future integration with
// virtualmodel is planned once both stabilize.
//
// # Usage
//
//	env := protocoltest.NewTestEnv(t)
//	defer env.Close()
//	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, protocoltest.TextScenario())
//	result := env.SendAs(t, protocol.TypeAnthropicV1, protocoltest.TextScenario(), false)
//	assert.Equal(t, "assistant", result.Role)
package protocol_validate

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TokenUsage holds token counts extracted from a provider response.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// ToolCallResult holds a single tool/function call extracted from a response.
type ToolCallResult struct {
	ID        string
	Name      string
	Arguments string // raw JSON string
}

// RoundTripResult captures the full result of a round-trip request through the gateway.
type RoundTripResult struct {
	// Source and target context
	SourceProtocol protocol.APIType
	TargetProtocol protocol.APIType
	ScenarioName   string
	IsStreaming    bool

	// HTTP layer
	HTTPStatus   int
	RawBody      []byte
	StreamEvents []string // raw SSE event lines (streaming only)

	// Extracted semantics (populated by the framework after parsing)
	Content         string
	Role            string
	Model           string
	FinishReason    string
	ToolCalls       []ToolCallResult
	ThinkingContent string
	Usage           *TokenUsage
}

// Assertion is a named check applied to a RoundTripResult.
type Assertion struct {
	Name  string
	Check func(r *RoundTripResult) error
}

// TestResult represents the outcome of a single matrix test combination.
// This is returned by Matrix.ExecuteAll() for CLI and other non-testing contexts.
type TestResult struct {
	// Test identification
	Name      string // Full test name: "scenario/source/target/mode"
	Scenario  string // Scenario name: "text", "tool_use", etc.
	Source    protocol.APIType
	Target    protocol.APIType
	Streaming bool

	// Test outcome
	Passed     bool   // true if all assertions passed
	Skipped    bool   // true if test was skipped
	SkipReason string // reason for skipping

	// Error details
	Errors   []AssertionError // list of assertion failures
	Duration time.Duration    // test execution time

	// Batch statistics (populated when BatchCount > 1)
	BatchCount  int           // number of times the test was executed
	BatchPassed int           // number of executions that passed
	BatchMinDur time.Duration // minimum duration across executions
	BatchAvgDur time.Duration // average duration across executions
	BatchMaxDur time.Duration // maximum duration across executions
	BatchErrors []string      // unique error messages from failed executions

	// Response details (for debugging/verbose output)
	HTTPStatus int              // HTTP status code
	Response   *RoundTripResult // full round-trip result (from first or last execution)
}

// AssertionError represents a single assertion failure.
type AssertionError struct {
	Assertion string // assertion name
	Error     string // error message
	Context   string // additional context (truncated body, etc.)
}
