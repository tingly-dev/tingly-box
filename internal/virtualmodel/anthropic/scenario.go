package anthropic

import (
	"encoding/json"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// MockScenario describes a named test scenario with an Anthropic-protocol mock response.
// It mirrors server_validate.Scenario for cross-package alignment.
type MockScenario struct {
	ID           string
	Name         string
	Delay        time.Duration
	StreamChunks []string

	Content    string
	ToolCall   *virtualmodel.ToolCallConfig
	StopReason string // defaults to "stop" (or "tool_use" if ToolCall is set)
}

type scenarioModel struct {
	id         string
	delay      time.Duration
	chunks     []string
	content    string
	toolCall   *virtualmodel.ToolCallConfig
	stopReason string
}

// NewMockFromScenario creates an Anthropic VirtualModel from a MockScenario.
func NewMockFromScenario(s *MockScenario) VirtualModel {
	stop := s.StopReason
	if stop == "" {
		if s.ToolCall != nil {
			stop = "tool_use"
		} else {
			stop = "stop"
		}
	}
	return &scenarioModel{
		id:         s.ID,
		delay:      s.Delay,
		chunks:     s.StreamChunks,
		content:    s.Content,
		toolCall:   s.ToolCall,
		stopReason: stop,
	}
}

func (m *scenarioModel) GetID() string          { return m.id }
func (m *scenarioModel) GetName() string        { return m.id }
func (m *scenarioModel) GetDescription() string { return "A scenario-based virtual model" }
func (m *scenarioModel) GetType() virtualmodel.VirtualModelType {
	if m.toolCall != nil {
		return virtualmodel.VirtualModelTypeTool
	}
	return virtualmodel.VirtualModelTypeStatic
}
func (m *scenarioModel) SimulatedDelay() time.Duration { return m.delay }
func (m *scenarioModel) ToModel() virtualmodel.Model {
	return virtualmodel.Model{ID: m.id, Object: "model", OwnedBy: "tingly-box-virtual"}
}

func (m *scenarioModel) streamChunks() []string {
	if len(m.chunks) > 0 {
		return m.chunks
	}
	return token.SplitIntoChunks(m.content)
}

func (m *scenarioModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if m.toolCall != nil {
		displayText := virtualmodel.ToolCallDisplayContent(m.toolCall.Arguments)
		inputJSON, _ := json.Marshal(m.toolCall.Arguments)
		return VModelResponse{
			Content: []sdk.BetaContentBlockParamUnion{
				{OfText: &sdk.BetaTextBlockParam{Text: displayText}},
				{OfToolUse: &sdk.BetaToolUseBlockParam{
					ID:    "tool_virtual",
					Name:  m.toolCall.Name,
					Input: json.RawMessage(inputJSON),
				}},
			},
			StopReason: sdk.BetaStopReason(m.stopReason),
		}, nil
	}
	return VModelResponse{
		Content:    []sdk.BetaContentBlockParamUnion{{OfText: &sdk.BetaTextBlockParam{Text: m.content}}},
		StopReason: sdk.BetaStopReason(m.stopReason),
	}, nil
}

func (m *scenarioModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	return DefaultStream(m, req, emit)
}
