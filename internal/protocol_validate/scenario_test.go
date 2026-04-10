package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pv "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestAllScenarios_Registered verifies every expected scenario is registered in AllScenarios().
func TestAllScenarios_Registered(t *testing.T) {
	all := pv.AllScenarios()
	names := make(map[string]bool, len(all))
	for _, s := range all {
		names[s.Name] = true
	}

	required := []string{
		"text",
		"tool_use",
		"tool_result",
		"thinking",
		"multi_turn",
		"streaming_text",
		"streaming_tool_use",
		"error",
	}
	for _, name := range required {
		assert.True(t, names[name], "scenario %q must be registered", name)
	}
}

// TestScenario_Text verifies the text scenario structure.
func TestScenario_Text(t *testing.T) {
	s := pv.TextScenario()
	assert.Equal(t, "text", s.Name)
	assert.NotEmpty(t, s.Tags)
	assert.Contains(t, s.Tags, "text")
	assert.NotNil(t, s.MockResponses)
	assert.NotNil(t, s.Assertions)
	assert.Greater(t, len(s.Assertions), 0)
}

// TestScenario_ToolUse verifies the tool_use scenario includes tool call definitions.
func TestScenario_ToolUse(t *testing.T) {
	s := pv.ToolUseScenario()
	assert.Equal(t, "tool_use", s.Name)
	assert.Contains(t, s.Tags, "tool_use")
	// Must define mock responses for all 3 provider styles
	assert.NotNil(t, s.MockResponses["openai"])
	assert.NotNil(t, s.MockResponses["anthropic"])
	assert.NotNil(t, s.MockResponses["google"])
}

// TestScenario_Thinking verifies the thinking scenario includes thinking block responses.
func TestScenario_Thinking(t *testing.T) {
	s := pv.ThinkingScenario()
	assert.Equal(t, "thinking", s.Name)
	assert.Contains(t, s.Tags, "thinking")
	assert.NotNil(t, s.MockResponses["anthropic"])
}

// TestScenario_MultiTurn verifies the multi_turn scenario sets up conversation history.
func TestScenario_MultiTurn(t *testing.T) {
	s := pv.MultiTurnScenario()
	assert.Equal(t, "multi_turn", s.Name)
	assert.Contains(t, s.Tags, "multi_turn")
}

// TestScenario_Error verifies the error scenario configures a non-2xx response.
func TestScenario_Error(t *testing.T) {
	s := pv.ErrorScenario()
	assert.Equal(t, "error", s.Name)
	// Error scenario mock response must return non-200
	status, _ := s.MockResponses["openai"].NonStream()
	assert.NotEqual(t, 200, status)
}

// TestScenario_Streaming_Text verifies streaming scenario provides SSE events.
func TestScenario_Streaming_Text(t *testing.T) {
	s := pv.StreamingTextScenario()
	assert.Equal(t, "streaming_text", s.Name)
	assert.Contains(t, s.Tags, "streaming")

	events := s.MockResponses["openai"].Stream()
	assert.Greater(t, len(events), 0)
	// Must end with [DONE]
	last := events[len(events)-1]
	assert.Contains(t, last, "[DONE]")
}

// TestScenario_Streaming_ToolUse verifies streaming tool call scenario.
func TestScenario_Streaming_ToolUse(t *testing.T) {
	s := pv.StreamingToolUseScenario()
	assert.Equal(t, "streaming_tool_use", s.Name)
	assert.Contains(t, s.Tags, "streaming")
	assert.Contains(t, s.Tags, "tool_use")

	events := s.MockResponses["openai"].Stream()
	assert.Greater(t, len(events), 2)
}
