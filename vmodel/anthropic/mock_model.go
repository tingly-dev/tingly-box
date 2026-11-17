package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockModelConfig holds configuration for an Anthropic mock virtual model
// (static or tool).
// The shared config keeps file imports and Go-defined mocks identical.
type MockModelConfig = vmodel.SharedMockSpec

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
		if len(cfg.Calls()) > 0 {
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
	if len(cfg.Calls()) > 0 {
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

// ErrorInjection implements vmodel.ErrorInjectingModel.
func (m *MockModel) ErrorInjection() *vmodel.ErrorInjection { return m.cfg.Error }

func (m *MockModel) streamChunks() []string {
	if len(m.cfg.StreamChunks) > 0 {
		return m.cfg.StreamChunks
	}
	return token.SplitIntoChunks(m.cfg.Content)
}

// HandleAnthropic returns fixed content from config in Anthropic format.
func (m *MockModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if len(m.cfg.Calls()) > 0 {
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
	calls := m.cfg.Calls()
	text := m.cfg.Content
	if (text == "" || m.cfg.ToolCall != nil) && len(calls) == 1 {
		text = vmodel.ToolCallDisplayContent(calls[0].Arguments)
	}
	content := []sdk.BetaContentBlockParamUnion{{OfText: &sdk.BetaTextBlockParam{Text: text}}}
	for i, tc := range calls {
		id := tc.ID
		if id == "" {
			id = "toolu_virtual"
			if len(calls) > 1 {
				id = fmt.Sprintf("toolu_virtual_%d", i)
			}
		}
		content = append(content, sdk.BetaContentBlockParamUnion{OfToolUse: &sdk.BetaToolUseBlockParam{ID: id, Name: tc.Name, Input: vmodel.ToolArguments(tc)}})
	}
	return VModelResponse{Content: content, StopReason: sdk.BetaStopReason(m.cfg.StopReason)}
}

// HandleAnthropicStream streams fixed content using configured chunks with simulated delay.
func (m *MockModel) HandleAnthropicStream(ctx context.Context, req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := m.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(StreamStartEvent{MsgID: "msg_virtual", Model: m.cfg.ID})
	chunks := m.streamChunks()
	perChunk := vmodel.ResolveChunkDelay(m.cfg.Delay, len(chunks))
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			if err := vmodel.EmitChunks(ctx, chunks, perChunk, func(_ int, chunk string) bool {
				emit(TextDeltaEvent{Index: i, Text: chunk})
				return true
			}); err != nil {
				return err
			}
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
	if m.cfg.Usage != nil {
		emit(UsageEvent{Usage: *m.cfg.Usage})
	}
	emit(DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}

// Spec returns the model configuration for lossless catalog export.
func (m *MockModel) Spec() vmodel.SharedMockSpec { return *m.cfg }
