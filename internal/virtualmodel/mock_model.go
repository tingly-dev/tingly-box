package virtualmodel

import (
	"encoding/json"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// MockModelConfig holds configuration for a mock virtual model (static or tool).
type MockModelConfig struct {
	ID           string
	Name         string
	Description  string
	Content      string        // static: fixed response text
	FinishReason string        // default: "stop"
	Delay        time.Duration // simulated latency
	StreamChunks []string      // optional custom stream chunks

	// tool-type: if set, Response returns a tool_use block
	ToolCall *ToolCallConfig
}

// MockModel implements VirtualModel, AnthropicVirtualModel, and OpenAIChatVirtualModel
// for static and tool-type virtual models. HandleAnthropic/HandleOpenAIChat are no-ops;
// ResponseAnthropic/ResponseOpenAIChat return fixed content from config.
type MockModel struct {
	cfg *MockModelConfig
}

// Compile-time interface checks.
var _ AnthropicVirtualModel = (*MockModel)(nil)
var _ OpenAIChatVirtualModel = (*MockModel)(nil)

// NewMockModel creates a MockModel. FinishReason defaults to "stop" (or "tool_use" if ToolCall is set).
func NewMockModel(cfg *MockModelConfig) *MockModel {
	if cfg.FinishReason == "" {
		if cfg.ToolCall != nil {
			cfg.FinishReason = "tool_use"
		} else {
			cfg.FinishReason = "stop"
		}
	}
	return &MockModel{cfg: cfg}
}

func (m *MockModel) GetID() string { return m.cfg.ID }

func (m *MockModel) SimulatedDelay() time.Duration { return m.cfg.Delay }

// Protocols declares that MockModel supports both Anthropic and OpenAI Chat.
func (m *MockModel) Protocols() []protocol.APIType {
	return []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
}

func (m *MockModel) streamChunks() []string {
	if len(m.cfg.StreamChunks) > 0 {
		return m.cfg.StreamChunks
	}
	return token.SplitIntoChunks(m.cfg.Content)
}

func (m *MockModel) ToModel() Model {
	return Model{
		ID:      m.cfg.ID,
		Object:  "model",
		Created: 0,
		OwnedBy: "tingly-box-virtual",
	}
}

// HandleAnthropic returns fixed content from config in Anthropic format.
func (m *MockModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if m.cfg.ToolCall != nil {
		return m.anthropicToolResponse(), nil
	}
	return m.anthropicStaticResponse(), nil
}

func (m *MockModel) anthropicStaticResponse() VModelResponse {
	return VModelResponse{
		Content: []anthropic.BetaContentBlockParamUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: m.cfg.Content}},
		},
		StopReason: anthropic.BetaStopReason(m.cfg.FinishReason),
	}
}

func (m *MockModel) anthropicToolResponse() VModelResponse {
	tc := m.cfg.ToolCall
	displayText := ToolCallDisplayContent(tc.Arguments)
	inputJSON, _ := json.Marshal(tc.Arguments)

	return VModelResponse{
		Content: []anthropic.BetaContentBlockParamUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: displayText}},
			{OfToolUse: &anthropic.BetaToolUseBlockParam{
				ID:    "toolu_virtual",
				Name:  tc.Name,
				Input: json.RawMessage(inputJSON),
			}},
		},
		StopReason: "tool_use",
	}
}

// HandleAnthropicStream streams fixed content using GetStreamChunks with simulated delay.
func (m *MockModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := m.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(AnthropicStreamStartEvent{MsgID: "msg_virtual", Model: m.cfg.ID})
	delay := m.cfg.Delay
	chunks := m.streamChunks()
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			for j, chunk := range chunks {
				if delay > 0 {
					time.Sleep(delay / time.Duration(len(chunks)))
				} else {
					time.Sleep(50 * time.Millisecond)
				}
				_ = j
				emit(AnthropicTextDeltaEvent{Index: i, Text: chunk})
			}
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			emit(AnthropicToolUseEvent{
				Index: i,
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	emit(AnthropicDoneEvent{StopReason: string(resp.StopReason)})
	return nil
}

// HandleOpenAIChatStream streams fixed content using GetStreamChunks with simulated delay.
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
		emit(OpenAIChatDeltaEvent{Index: i, Content: chunk})
	}
	for i, tc := range resp.ToolCalls {
		emit(OpenAIChatToolEvent{Index: i, ToolCall: tc})
	}
	emit(OpenAIChatDoneEvent{FinishReason: resp.FinishReason})
	return nil
}

// HandleOpenAIChat returns fixed content from config in OpenAI Chat format.
func (m *MockModel) HandleOpenAIChat(_ *protocol.OpenAIChatCompletionRequest) (OpenAIChatVModelResponse, error) {
	if m.cfg.ToolCall != nil {
		return m.openaiToolResponse(), nil
	}
	return OpenAIChatVModelResponse{
		Content:      m.cfg.Content,
		FinishReason: m.cfg.FinishReason,
	}, nil
}

func (m *MockModel) openaiToolResponse() OpenAIChatVModelResponse {
	tc := m.cfg.ToolCall
	argsJSON, _ := json.Marshal(tc.Arguments)
	return OpenAIChatVModelResponse{
		ToolCalls: []VToolCall{{
			ID:        "toolu_virtual",
			Name:      tc.Name,
			Arguments: string(argsJSON),
		}},
		FinishReason: "tool_calls",
	}
}

// ── MockScenario ──────────────────────────────────────────────────────────────
//
// MockScenario defines mock responses for one or more protocols.
// It mirrors the server_validate.Scenario pattern, enabling future alignment.
// Use NewMockModelFromScenario to create a VirtualModel from a scenario.

// AnthropicMockResponse defines the Anthropic-protocol mock response for a scenario.
type AnthropicMockResponse struct {
	Content    string
	ToolCall   *ToolCallConfig
	StopReason string // defaults to "stop" (or "tool_use" if ToolCall is set)
}

// OpenAIChatMockResponse defines the OpenAI Chat-protocol mock response for a scenario.
type OpenAIChatMockResponse struct {
	Content      string
	ToolCalls    []VToolCall
	FinishReason string // defaults to "stop" (or "tool_calls" if ToolCalls non-empty)
}

// MockScenario describes a named test scenario with per-protocol mock responses.
// Fields Anthropic and OpenAIChat are optional; set at least one.
// The resulting model implements only the sub-interfaces for which a response is provided.
type MockScenario struct {
	ID           string
	Name         string
	Delay        time.Duration
	StreamChunks []string

	Anthropic  *AnthropicMockResponse  // if set → implements AnthropicVirtualModel
	OpenAIChat *OpenAIChatMockResponse // if set → implements OpenAIChatVirtualModel
}

// scenarioModel is an internal type created from a MockScenario.
// It conditionally implements AnthropicVirtualModel and/or OpenAIChatVirtualModel
// depending on which fields of the scenario are populated.
type scenarioModel struct {
	id         string
	delay      time.Duration
	chunks     []string
	anthropic  *AnthropicMockResponse
	openaiChat *OpenAIChatMockResponse
}

// NewMockModelFromScenario creates a VirtualModel from a MockScenario.
// The returned model implements AnthropicVirtualModel if Anthropic is set,
// and OpenAIChatVirtualModel if OpenAIChat is set.
func NewMockModelFromScenario(s *MockScenario) VirtualModel {
	m := &scenarioModel{
		id:         s.ID,
		delay:      s.Delay,
		chunks:     s.StreamChunks,
		anthropic:  s.Anthropic,
		openaiChat: s.OpenAIChat,
	}
	if m.anthropic != nil && m.anthropic.StopReason == "" {
		if m.anthropic.ToolCall != nil {
			m.anthropic.StopReason = "tool_use"
		} else {
			m.anthropic.StopReason = "stop"
		}
	}
	if m.openaiChat != nil && m.openaiChat.FinishReason == "" {
		if len(m.openaiChat.ToolCalls) > 0 {
			m.openaiChat.FinishReason = "tool_calls"
		} else {
			m.openaiChat.FinishReason = "stop"
		}
	}
	return m
}

func (m *scenarioModel) GetID() string                 { return m.id }
func (m *scenarioModel) SimulatedDelay() time.Duration { return m.delay }

// Protocols returns the protocols for which this scenario has mock responses defined.
func (m *scenarioModel) Protocols() []protocol.APIType {
	var types []protocol.APIType
	if m.anthropic != nil {
		types = append(types, protocol.TypeAnthropicBeta)
	}
	if m.openaiChat != nil {
		types = append(types, protocol.TypeOpenAIChat)
	}
	return types
}
func (m *scenarioModel) ToModel() Model {
	return Model{ID: m.id, Object: "model", OwnedBy: "tingly-box-virtual"}
}
func (m *scenarioModel) streamChunks() []string {
	if len(m.chunks) > 0 {
		return m.chunks
	}
	if m.anthropic != nil {
		return token.SplitIntoChunks(m.anthropic.Content)
	}
	if m.openaiChat != nil {
		return token.SplitIntoChunks(m.openaiChat.Content)
	}
	return nil
}

// HandleAnthropic — available only when scenario.Anthropic is set.
func (m *scenarioModel) HandleAnthropic(_ *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	a := m.anthropic
	if a.ToolCall != nil {
		displayText := ToolCallDisplayContent(a.ToolCall.Arguments)
		inputJSON, _ := json.Marshal(a.ToolCall.Arguments)
		return VModelResponse{
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: displayText}},
				{OfToolUse: &anthropic.BetaToolUseBlockParam{
					ID:    "tool_virtual",
					Name:  a.ToolCall.Name,
					Input: json.RawMessage(inputJSON),
				}},
			},
			StopReason: anthropic.BetaStopReason(a.StopReason),
		}, nil
	}
	return VModelResponse{
		Content:    []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: a.Content}}},
		StopReason: anthropic.BetaStopReason(a.StopReason),
	}, nil
}

// HandleOpenAIChat — available only when scenario.OpenAIChat is set.
func (m *scenarioModel) HandleOpenAIChat(_ *protocol.OpenAIChatCompletionRequest) (OpenAIChatVModelResponse, error) {
	oc := m.openaiChat
	return OpenAIChatVModelResponse{
		Content:      oc.Content,
		ToolCalls:    oc.ToolCalls,
		FinishReason: oc.FinishReason,
	}, nil
}

func (m *scenarioModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	return DefaultAnthropicStream(m, req, emit)
}

func (m *scenarioModel) HandleOpenAIChatStream(req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	return DefaultOpenAIChatStream(m, req, emit)
}

// scenarioModel always satisfies AnthropicVirtualModel and OpenAIChatVirtualModel at the
// type level; callers use type assertion to check whether the scenario populated the
// corresponding field. The handler pattern is:
//
//	if avm, ok := vm.(AnthropicVirtualModel); ok && scenario.Anthropic != nil { ... }
//
// However, since scenarioModel always implements both method sets, the handler's
// type assertion is always true. To give accurate capability signals, callers should
// use the scenario directly. For handler-level capability routing, prefer MockModel
// (whose capabilities are always both protocols) or TransformModel (Anthropic-only).
