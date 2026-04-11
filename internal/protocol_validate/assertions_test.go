package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pv "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestAssertContentEquals passes when content matches exactly.
func TestAssertContentEquals(t *testing.T) {
	result := &pv.RoundTripResult{Content: "Hello, world!"}
	err := pv.AssertContentEquals("Hello, world!").Check(result)
	assert.NoError(t, err)
}

// TestAssertContentEquals_Fails when content does not match.
func TestAssertContentEquals_Fails(t *testing.T) {
	result := &pv.RoundTripResult{Content: "Hello, world!"}
	err := pv.AssertContentEquals("Goodbye").Check(result)
	assert.Error(t, err)
}

// TestAssertContentContains passes when content contains substring.
func TestAssertContentContains(t *testing.T) {
	result := &pv.RoundTripResult{Content: "The weather in Paris is sunny."}
	err := pv.AssertContentContains("Paris").Check(result)
	assert.NoError(t, err)
}

// TestAssertContentContains_Fails when content does not contain substring.
func TestAssertContentContains_Fails(t *testing.T) {
	result := &pv.RoundTripResult{Content: "The weather in Paris is sunny."}
	err := pv.AssertContentContains("Tokyo").Check(result)
	assert.Error(t, err)
}

// TestAssertRoleEquals verifies role assertion.
func TestAssertRoleEquals(t *testing.T) {
	result := &pv.RoundTripResult{Role: "assistant"}
	assert.NoError(t, pv.AssertRoleEquals("assistant").Check(result))
	assert.Error(t, pv.AssertRoleEquals("user").Check(result))
}

// TestAssertFinishReason verifies finish_reason assertion.
func TestAssertFinishReason(t *testing.T) {
	result := &pv.RoundTripResult{FinishReason: "stop"}
	assert.NoError(t, pv.AssertFinishReason("stop").Check(result))
	assert.Error(t, pv.AssertFinishReason("tool_calls").Check(result))
}

// TestAssertHasToolCalls_Count verifies the right number of tool calls.
func TestAssertHasToolCalls_Count(t *testing.T) {
	result := &pv.RoundTripResult{
		ToolCalls: []pv.ToolCallResult{
			{ID: "call_1", Name: "get_weather", Arguments: `{"location":"Paris"}`},
		},
	}
	assert.NoError(t, pv.AssertHasToolCalls(1).Check(result))
	assert.Error(t, pv.AssertHasToolCalls(2).Check(result))
}

// TestAssertToolCallName verifies tool call name at index.
func TestAssertToolCallName(t *testing.T) {
	result := &pv.RoundTripResult{
		ToolCalls: []pv.ToolCallResult{
			{ID: "call_1", Name: "get_weather", Arguments: `{"location":"Paris"}`},
		},
	}
	assert.NoError(t, pv.AssertToolCallName(0, "get_weather").Check(result))
	assert.Error(t, pv.AssertToolCallName(0, "send_email").Check(result))
	// Out of bounds
	assert.Error(t, pv.AssertToolCallName(1, "get_weather").Check(result))
}

// TestAssertToolCallArgs verifies a key-value pair in tool call arguments JSON.
func TestAssertToolCallArgs(t *testing.T) {
	result := &pv.RoundTripResult{
		ToolCalls: []pv.ToolCallResult{
			{ID: "call_1", Name: "get_weather", Arguments: `{"location":"Paris","unit":"celsius"}`},
		},
	}
	assert.NoError(t, pv.AssertToolCallArgs(0, "location", "Paris").Check(result))
	assert.NoError(t, pv.AssertToolCallArgs(0, "unit", "celsius").Check(result))
	assert.Error(t, pv.AssertToolCallArgs(0, "location", "Tokyo").Check(result))
	assert.Error(t, pv.AssertToolCallArgs(0, "missing_key", "value").Check(result))
}

// TestAssertHasThinking passes when thinking content is non-empty.
func TestAssertHasThinking(t *testing.T) {
	result := &pv.RoundTripResult{ThinkingContent: "Let me reason through this..."}
	assert.NoError(t, pv.AssertHasThinking().Check(result))

	empty := &pv.RoundTripResult{ThinkingContent: ""}
	assert.Error(t, pv.AssertHasThinking().Check(empty))
}

// TestAssertNoThinking passes when thinking content is empty.
func TestAssertNoThinking(t *testing.T) {
	result := &pv.RoundTripResult{ThinkingContent: ""}
	assert.NoError(t, pv.AssertNoThinking().Check(result))

	withThinking := &pv.RoundTripResult{ThinkingContent: "some thinking"}
	assert.Error(t, pv.AssertNoThinking().Check(withThinking))
}

// TestAssertUsageNonZero passes when at least one token count is > 0.
func TestAssertUsageNonZero(t *testing.T) {
	result := &pv.RoundTripResult{
		Usage: &pv.TokenUsage{InputTokens: 10, OutputTokens: 5},
	}
	assert.NoError(t, pv.AssertUsageNonZero().Check(result))

	empty := &pv.RoundTripResult{Usage: &pv.TokenUsage{}}
	assert.Error(t, pv.AssertUsageNonZero().Check(empty))

	nilUsage := &pv.RoundTripResult{Usage: nil}
	assert.Error(t, pv.AssertUsageNonZero().Check(nilUsage))
}

// TestAssertHTTPStatus verifies HTTP status assertion.
func TestAssertHTTPStatus(t *testing.T) {
	result := &pv.RoundTripResult{HTTPStatus: 200}
	assert.NoError(t, pv.AssertHTTPStatus(200).Check(result))
	assert.Error(t, pv.AssertHTTPStatus(429).Check(result))
}

// TestAssertStreamEventCount verifies minimum stream event count.
func TestAssertStreamEventCount(t *testing.T) {
	result := &pv.RoundTripResult{
		StreamEvents: []string{"event1", "event2", "event3"},
	}
	assert.NoError(t, pv.AssertStreamEventCount(3).Check(result))
	assert.NoError(t, pv.AssertStreamEventCount(1).Check(result))
	assert.Error(t, pv.AssertStreamEventCount(5).Check(result))
}

// TestAssertModelContains verifies model name substring check.
func TestAssertModelContains(t *testing.T) {
	result := &pv.RoundTripResult{Model: "gpt-4o-mini-2024-07-18"}
	assert.NoError(t, pv.AssertModelContains("gpt-4o").Check(result))
	assert.Error(t, pv.AssertModelContains("claude").Check(result))
}

// TestAssertAll_ComposedChecks verifies multiple assertions can be composed
// and all must pass.
func TestAssertAll_ComposedChecks(t *testing.T) {
	result := &pv.RoundTripResult{
		HTTPStatus:   200,
		Role:         "assistant",
		Content:      "The capital of France is Paris.",
		FinishReason: "stop",
		Usage:        &pv.TokenUsage{InputTokens: 10, OutputTokens: 20},
	}

	assertions := []pv.Assertion{
		pv.AssertHTTPStatus(200),
		pv.AssertRoleEquals("assistant"),
		pv.AssertContentContains("Paris"),
		pv.AssertFinishReason("stop"),
		pv.AssertUsageNonZero(),
	}

	for _, a := range assertions {
		err := a.Check(result)
		require.NoError(t, err, "assertion %q failed", a.Name)
	}
}
