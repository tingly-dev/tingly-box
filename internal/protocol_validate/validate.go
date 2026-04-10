// Package protocol_validate provides a framework for end-to-end validation of
// the model gateway's protocol transformation layer.
//
// # Architecture
//
// Two main components:
//
//  1. VirtualServer — an httptest-based mock provider that speaks OpenAI, Anthropic,
//     and Google response formats. Conceptually equivalent to a "virtual model" —
//     deterministic, scenario-driven, assertion-friendly. In the future this may
//     merge with existing virtual model server infrastructure.
//
//  2. TestEnv — wires a real gateway Server (with transform pipeline) to a
//     VirtualServer, sets up routing rules, and provides SendAs() for round-trip testing.
//
// # Usage
//
//	env := protocol_validate.NewTestEnv(t)
//	defer env.Close()
//	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, protocol_validate.TextScenario())
//	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol_validate.TextScenario(), false)
//	assert.Equal(t, "assistant", result.Role)
package protocol_validate

import "github.com/tingly-dev/tingly-box/internal/protocol"

// APIStyle mirrors protocol.APIStyle for convenience in MockResponseBuilder keys.
type APIStyle = protocol.APIStyle

const (
	StyleOpenAI    = protocol.APIStyleOpenAI
	StyleAnthropic = protocol.APIStyleAnthropic
	StyleGoogle    = protocol.APIStyleGoogle
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
	HTTPStatus  int
	RawBody     []byte
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

// MockResponseBuilder defines how a virtual server should respond for one provider style.
type MockResponseBuilder struct {
	// NonStream returns the HTTP status code and response body bytes.
	NonStream func() (statusCode int, body []byte)
	// Stream returns the SSE event lines (each line is "data: ..." or "data: [DONE]").
	Stream func() []string
}

// Scenario is a named test scenario describing:
//   - What the mock provider should return (MockResponses per APIStyle)
//   - What assertions to run on the round-trip result
type Scenario struct {
	Name        string
	Description string
	Tags        []string

	// MockResponses keyed by provider APIStyle ("openai", "anthropic", "google").
	// Determines what the virtual server returns.
	MockResponses map[APIStyle]MockResponseBuilder

	// Assertions run after every round-trip for this scenario.
	Assertions []Assertion
}
