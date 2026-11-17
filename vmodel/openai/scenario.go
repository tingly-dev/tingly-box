package openai

import (
	"encoding/json"
	"github.com/tingly-dev/tingly-box/vmodel"
	"time"
)

// MockScenario is the compatibility shape for existing OpenAI Go callers.
// New definitions use SharedMockSpec and tool_calls with structured arguments.
type MockScenario struct {
	ID           string
	Name         string
	Delay        time.Duration
	StreamChunks []string
	Content      string
	ToolCalls    []VToolCall
	FinishReason string
	Error        *vmodel.ErrorInjection
}

func NewMockFromScenario(s *MockScenario) VirtualModel {
	cfg := vmodel.SharedMockSpec{ID: s.ID, Name: s.Name, Delay: s.Delay, StreamChunks: s.StreamChunks, Content: s.Content, FinishReason: s.FinishReason, Error: s.Error}
	cfg.Name = cfg.ID
	cfg.Description = "A scenario-based virtual model"
	for _, tc := range s.ToolCalls {
		cfg.ToolCalls = append(cfg.ToolCalls, vmodel.ToolCallConfig{ID: tc.ID, Name: tc.Name, RawArguments: json.RawMessage(tc.Arguments)})
	}
	return NewMockModel(&cfg)
}
