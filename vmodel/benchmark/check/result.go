// Package check holds the protocol-neutral, reusable check logic for the vmodel
// benchmark: the RoundTripResult view of a single gateway round trip and the
// named Assertion library that operates on it.
//
// It carries no test-framework dependency (no *testing.T), so it can be imported
// in-process by any *test package or by an external Go project. Its only
// non-stdlib dependency is internal/protocol for APIType — and vmodel/* already
// imports internal/protocol, so this introduces no import cycle.
package check

import (
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
