package openai

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// MockModelConfig holds configuration for an OpenAI mock virtual model
// (static or tool).
type MockModelConfig struct {
	ID           string
	Name         string
	Description  string
	Content      string        // static: fixed response text
	FinishReason string        // default: "stop" (or "tool_calls" if ToolCall is set)
	Delay        time.Duration // simulated latency
	StreamChunks []string      // optional custom stream chunks

	// tool-type: if set, the response includes tool calls.
	ToolCall *vmodel.ToolCallConfig

	// Usage, when set, is emitted as a UsageEvent immediately before
	// DoneEvent so streaming consumers can be exercised against a
	// deterministic, fully-populated usage shape.
	Usage *vmodel.MockUsage

	// Error, when non-nil, makes this mock simulate a failure. See
	// vmodel.ErrorInjection for the two supported stages.
	Error *vmodel.ErrorInjection
}

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
		if cfg.ToolCall != nil {
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
	if m.cfg.ToolCall != nil {
		return m.toolResponse(), nil
	}
	return VModelResponse{
		Content:      m.cfg.Content,
		FinishReason: m.cfg.FinishReason,
	}, nil
}

func (m *MockModel) toolResponse() VModelResponse {
	tc := m.cfg.ToolCall
	argsJSON, _ := json.Marshal(tc.Arguments)
	return VModelResponse{
		ToolCalls: []VToolCall{{
			ID:        "toolu_virtual",
			Name:      tc.Name,
			Arguments: string(argsJSON),
		}},
		FinishReason: "tool_calls",
	}
}

// HandleOpenAIChatStream streams fixed content using configured chunks with simulated delay.
func (m *MockModel) HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	resp, err := m.HandleOpenAIChat(req)
	if err != nil {
		return err
	}
	chunks := m.streamChunks()
	perChunk := vmodel.ResolveChunkDelay(m.cfg.Delay, len(chunks))
	vmodel.EmitChunks(chunks, perChunk, func(i int, chunk string) {
		emit(DeltaEvent{Index: i, Content: chunk})
	})
	for i, tc := range resp.ToolCalls {
		emit(ToolEvent{Index: i, ToolCall: tc})
	}
	if m.cfg.Usage != nil {
		emit(UsageEvent{Usage: *m.cfg.Usage})
	}
	emit(DoneEvent{FinishReason: resp.FinishReason})
	return nil
}
