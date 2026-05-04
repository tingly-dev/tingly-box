package openai

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
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
	ToolCall *virtualmodel.ToolCallConfig
}

// MockModel is an OpenAI-Chat-only mock virtual model.
type MockModel struct {
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
	return &MockModel{cfg: cfg}
}

func (m *MockModel) GetID() string { return m.cfg.ID }

func (m *MockModel) GetName() string { return m.cfg.Name }

func (m *MockModel) GetDescription() string {
	if m.cfg.Description != "" {
		return m.cfg.Description
	}
	return "A virtual model that returns fixed responses for testing"
}

func (m *MockModel) GetType() virtualmodel.VirtualModelType {
	if m.cfg.ToolCall != nil {
		return virtualmodel.VirtualModelTypeTool
	}
	return virtualmodel.VirtualModelTypeStatic
}

func (m *MockModel) SimulatedDelay() time.Duration { return m.cfg.Delay }

func (m *MockModel) ToModel() virtualmodel.Model {
	return virtualmodel.Model{
		ID:      m.cfg.ID,
		Object:  "model",
		Created: 0,
		OwnedBy: "tingly-box-virtual",
	}
}

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
	delay := m.cfg.Delay
	chunks := m.streamChunks()
	for i, chunk := range chunks {
		if delay > 0 {
			time.Sleep(delay / time.Duration(len(chunks)))
		} else {
			time.Sleep(50 * time.Millisecond)
		}
		emit(DeltaEvent{Index: i, Content: chunk})
	}
	for i, tc := range resp.ToolCalls {
		emit(ToolEvent{Index: i, ToolCall: tc})
	}
	emit(DoneEvent{FinishReason: resp.FinishReason})
	return nil
}
