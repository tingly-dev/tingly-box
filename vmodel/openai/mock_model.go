package openai

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockModelConfig holds configuration for an OpenAI mock virtual model
// (static or tool).
// The shared config keeps file imports and Go-defined mocks identical.
type MockModelConfig = vmodel.SharedMockSpec

// MockModel is an OpenAI-Chat-only mock virtual model.
type MockModel struct {
	vmodel.BaseMockModel
	cfg *MockModelConfig
}

// Compile-time interface check.
var _ VirtualModel = (*MockModel)(nil)

// NewMockModel creates a MockModel. FinishReason defaults to "stop"
// (or "tool_calls" if ToolCall is set).
func NewMockModel(cfg *MockModelConfig) *MockModel {
	if cfg.FinishReason == "" {
		if len(cfg.Calls()) > 0 {
			cfg.FinishReason = "tool_calls"
		} else {
			cfg.FinishReason = "stop"
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

// HandleOpenAIChat returns fixed content from config in OpenAI Chat format.
func (m *MockModel) HandleOpenAIChat(_ *protocol.OpenAIChatCompletionRequest) (VModelResponse, error) {
	if len(m.cfg.Calls()) > 0 {
		return m.toolResponse(), nil
	}
	return VModelResponse{
		Content:      m.cfg.Content,
		FinishReason: m.cfg.FinishReason,
	}, nil
}

func (m *MockModel) toolResponse() VModelResponse {
	calls := m.cfg.Calls()
	out := make([]VToolCall, 0, len(calls))
	for i, tc := range calls {
		id := tc.ID
		if id == "" {
			id = "toolu_virtual"
			if len(calls) > 1 {
				id = fmt.Sprintf("toolu_virtual_%d", i)
			}
		}
		out = append(out, VToolCall{ID: id, Name: tc.Name, Arguments: string(vmodel.ToolArguments(tc))})
	}
	content := m.cfg.Content
	if m.cfg.ToolCall != nil {
		content = ""
	} // Legacy singular tool mocks ignored Content.
	return VModelResponse{Content: content, ToolCalls: out, FinishReason: m.cfg.FinishReason}
}

// HandleOpenAIChatStream streams fixed content using configured chunks with simulated delay.
func (m *MockModel) HandleOpenAIChatStream(ctx context.Context, req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	resp, err := m.HandleOpenAIChat(req)
	if err != nil {
		return err
	}
	chunks := m.streamChunks()
	perChunk := vmodel.ResolveChunkDelay(m.cfg.Delay, len(chunks))
	if err := vmodel.EmitChunks(ctx, chunks, perChunk, func(i int, chunk string) bool {
		emit(DeltaEvent{Index: i, Content: chunk})
		return true
	}); err != nil {
		return err
	}
	for i, tc := range resp.ToolCalls {
		emit(ToolEvent{Index: i, ToolCall: tc})
	}
	if m.cfg.Usage != nil {
		emit(UsageEvent{Usage: *m.cfg.Usage})
	}
	emit(DoneEvent{FinishReason: resp.FinishReason})
	return nil
}

// Spec returns the model configuration for lossless catalog export.
func (m *MockModel) Spec() vmodel.SharedMockSpec { return *m.cfg }
