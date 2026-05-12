package openai

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockScenario describes a named test scenario with an OpenAI Chat-protocol mock response.
type MockScenario struct {
	ID           string
	Name         string
	Delay        time.Duration
	StreamChunks []string

	Content      string
	ToolCalls    []VToolCall
	FinishReason string // defaults to "stop" (or "tool_calls" if ToolCalls non-empty)
}

type scenarioModel struct {
	vmodel.BaseMockModel
	chunks       []string
	content      string
	toolCalls    []VToolCall
	finishReason string
}

// NewMockFromScenario creates an OpenAI Chat VirtualModel from a MockScenario.
func NewMockFromScenario(s *MockScenario) VirtualModel {
	finish := s.FinishReason
	if finish == "" {
		if len(s.ToolCalls) > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	typ := vmodel.VirtualModelTypeStatic
	if len(s.ToolCalls) > 0 {
		typ = vmodel.VirtualModelTypeTool
	}
	return &scenarioModel{
		BaseMockModel: vmodel.BaseMockModel{
			ID:          s.ID,
			Name:        s.ID,
			Description: "A scenario-based virtual model",
			Type:        typ,
			Delay:       s.Delay,
		},
		chunks:       s.StreamChunks,
		content:      s.Content,
		toolCalls:    s.ToolCalls,
		finishReason: finish,
	}
}

func (m *scenarioModel) streamChunks() []string {
	if len(m.chunks) > 0 {
		return m.chunks
	}
	return token.SplitIntoChunks(m.content)
}

func (m *scenarioModel) HandleOpenAIChat(_ *protocol.OpenAIChatCompletionRequest) (VModelResponse, error) {
	return VModelResponse{
		Content:      m.content,
		ToolCalls:    m.toolCalls,
		FinishReason: m.finishReason,
	}, nil
}

func (m *scenarioModel) HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	return DefaultStream(m, req, emit)
}
