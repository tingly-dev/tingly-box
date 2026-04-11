package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pt "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

func TestAllScenarios_Registered(t *testing.T) {
	all := pt.AllScenarios()
	names := make(map[string]bool, len(all))
	for _, s := range all {
		names[s.Name] = true
	}
	required := []string{
		"text", "tool_use", "tool_result", "thinking",
		"multi_turn", "streaming_text", "streaming_tool_use", "error",
	}
	for _, name := range required {
		assert.True(t, names[name], "scenario %q must be registered", name)
	}
}

func TestScenario_Text(t *testing.T) {
	s := pt.TextScenario()
	assert.Equal(t, "text", s.Name)
	assert.NotEmpty(t, s.Tags)
	assert.Contains(t, s.Tags, "text")
	assert.NotNil(t, s.MockResponses)
	assert.Greater(t, len(s.Assertions), 0)
}

func TestScenario_ToolUse(t *testing.T) {
	s := pt.ToolUseScenario()
	assert.Equal(t, "tool_use", s.Name)
	assert.Contains(t, s.Tags, "tool_use")
	assert.NotNil(t, s.MockResponses["openai"])
	assert.NotNil(t, s.MockResponses["anthropic"])
	assert.NotNil(t, s.MockResponses["google"])
}

func TestScenario_Thinking(t *testing.T) {
	s := pt.ThinkingScenario()
	assert.Equal(t, "thinking", s.Name)
	assert.Contains(t, s.Tags, "thinking")
	assert.NotNil(t, s.MockResponses["anthropic"])
}

func TestScenario_Error(t *testing.T) {
	s := pt.ErrorScenario()
	assert.Equal(t, "error", s.Name)
	status, _ := s.MockResponses["openai"].NonStream()
	assert.NotEqual(t, 200, status)
}

func TestScenario_Streaming_Text(t *testing.T) {
	s := pt.StreamingTextScenario()
	assert.Equal(t, "streaming_text", s.Name)
	assert.Contains(t, s.Tags, "streaming")
	events := s.MockResponses["openai"].Stream()
	assert.Greater(t, len(events), 0)
	last := events[len(events)-1]
	assert.Contains(t, last, "[DONE]")
}

func TestScenario_Streaming_ToolUse(t *testing.T) {
	s := pt.StreamingToolUseScenario()
	assert.Equal(t, "streaming_tool_use", s.Name)
	assert.Contains(t, s.Tags, "streaming")
	assert.Contains(t, s.Tags, "tool_use")
	events := s.MockResponses["openai"].Stream()
	assert.Greater(t, len(events), 2)
}
