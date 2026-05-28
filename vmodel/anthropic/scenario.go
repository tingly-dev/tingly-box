package anthropic

import (
	"encoding/json"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockScenario describes a named test scenario with an Anthropic-protocol mock response.
// It mirrors server_validate.Scenario for cross-package alignment.
type MockScenario struct {
	ID           string
	Name         string
	Delay        time.Duration
	StreamChunks []string

	Content    string
	ToolCall   *vmodel.ToolCallConfig
	StopReason string // defaults to "stop" (or "tool_use" if ToolCall is set)

	// Error, when non-nil, makes the scenario simulate a failure instead of
	// returning a normal response. See vmodel.ErrorInjection for the two
	// supported stages (pre-content vs mid-stream).
	Error *vmodel.ErrorInjection
}

type scenarioModel struct {
	vmodel.BaseMockModel
	chunks       []string
	content      string
	toolCall     *vmodel.ToolCallConfig
	stopReason   string
	errInjection *vmodel.ErrorInjection
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
	typ := vmodel.VirtualModelTypeStatic
	if s.ToolCall != nil {
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
		toolCall:     s.ToolCall,
		stopReason:   stop,
		errInjection: s.Error,
	}
}

// ErrorInjection implements vmodel.ErrorInjectingModel.
func (m *scenarioModel) ErrorInjection() *vmodel.ErrorInjection { return m.errInjection }

func (m *scenarioModel) streamChunks() []string {
	if len(m.chunks) > 0 {
		return m.chunks
	}
	return token.SplitIntoChunks(m.content)
}

func (m *scenarioModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if m.toolCall != nil {
		displayText := vmodel.ToolCallDisplayContent(m.toolCall.Arguments)
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
