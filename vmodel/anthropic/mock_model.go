package anthropic

import (
	"encoding/json"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockModelConfig holds configuration for an Anthropic mock virtual model
// (static or tool).
type MockModelConfig struct {
	ID           string
	Name         string
	Description  string
	Content      string        // static: fixed response text
	StopReason   string        // default: "stop" (or "tool_use" if ToolCall is set)
	Delay        time.Duration // simulated latency
	StreamChunks []string      // optional custom stream chunks

	// tool-type: if set, the response includes a tool_use block.
	ToolCall *vmodel.ToolCallConfig
}

// MockModel is an Anthropic-only mock virtual model. It returns a fixed
// content block (or tool_use block) and supports streaming via per-chunk
// token splitting.
type MockModel struct {
	vmodel.BaseMockModel
	cfg *MockModelConfig
}

// Compile-time interface check.
var _ VirtualModel = (*MockModel)(nil)

// NewMockModel creates a MockModel. StopReason defaults to "stop"
// (or "tool_use" if ToolCall is set).
func NewMockModel(cfg *MockModelConfig) *MockModel {
	if cfg.StopReason == "" {
		if cfg.ToolCall != nil {
			cfg.StopReason = "tool_use"
		} else {
			cfg.StopReason = "stop"
		}
	}
	description := cfg.Description
	if description == "" {
		description = vmodel.DefaultMockDescription
	}
	typ := vmodel.VirtualModelTypeStatic
	if cfg.ToolCall != nil {
		typ = vmodel.VirtualModelTypeTool
	}
	return &MockModel{
		BaseMockModel: vmodel.BaseMockModel{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Description: description,
			Type:        typ,
			Delay:       cfg.Delay,
		},
		cfg: cfg,
	}
}

func (m *MockModel) streamChunks() []string {
	if len(m.cfg.StreamChunks) > 0 {
		return m.cfg.StreamChunks
	}
	return token.SplitIntoChunks(m.cfg.Content)
}

// HandleAnthropic returns fixed content from config in Anthropic format.
func (m *MockModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if m.cfg.ToolCall != nil {
		return m.toolResponse(), nil
	}
	return m.staticResponse(), nil
}

func (m *MockModel) staticResponse() VModelResponse {
	return VModelResponse{
		Content: []sdk.BetaContentBlockParamUnion{
			{OfText: &sdk.BetaTextBlockParam{Text: m.cfg.Content}},
		},
		StopReason: sdk.BetaStopReason(m.cfg.StopReason),
	}
}

func (m *MockModel) toolResponse() VModelResponse {
	tc := m.cfg.ToolCall
	displayText := vmodel.ToolCallDisplayContent(tc.Arguments)
	inputJSON, _ := json.Marshal(tc.Arguments)

	return VModelResponse{
		Content: []sdk.BetaContentBlockParamUnion{
			{OfText: &sdk.BetaTextBlockParam{Text: displayText}},
			{OfToolUse: &sdk.BetaToolUseBlockParam{
				ID:    "toolu_virtual",
				Name:  tc.Name,
				Input: json.RawMessage(inputJSON),
			}},
		},
		StopReason: "tool_use",
	}
}

// HandleAnthropicStream streams fixed content using configured chunks with simulated delay.
func (m *MockModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := m.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(StreamStartEvent{MsgID: "msg_virtual", Model: m.cfg.ID})
	chunks := m.streamChunks()
	perChunk := vmodel.ResolveChunkDelay(m.cfg.Delay, len(chunks))
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			vmodel.EmitChunks(chunks, perChunk, func(_ int, chunk string) {
				emit(TextDeltaEvent{Index: i, Text: chunk})
			})
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			emit(ToolUseEvent{
				Index: i,
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	emit(DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}
