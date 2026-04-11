package virtualmodel

import (
	"fmt"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// helpers

func makeReq(texts ...string) *protocol.AnthropicBetaMessagesRequest {
	msgs := make([]anthropic.BetaMessageParam, 0, len(texts))
	for i, t := range texts {
		role := anthropic.BetaMessageParamRoleUser
		if i%2 == 1 {
			role = anthropic.BetaMessageParamRoleAssistant
		}
		msgs = append(msgs, anthropic.BetaMessageParam{
			Role:    role,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(t)},
		})
	}
	return &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: anthropic.BetaMessageNewParams{Messages: msgs},
	}
}

// ── compile-time sub-interface checks ─────────────────────────────────────────

var _ AnthropicVirtualModel = (*MockModel)(nil)
var _ OpenAIChatVirtualModel = (*MockModel)(nil)
var _ AnthropicVirtualModel = (*TransformModel)(nil)

// ── MockModel (static) ────────────────────────────────────────────────────────

func TestMockModel_Static_Handle_DoesNotModifyReq(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "static-test", Content: "hello"})
	req := makeReq("user message")
	before := req.Messages[0].Content[0].OfText.Text

	_, _ = vm.HandleAnthropic(req)

	if req.Messages[0].Content[0].OfText.Text != before {
		t.Error("HandleAnthropic() must not modify req for MockModel")
	}
}

func TestMockModel_Static_Handle_ReturnsTextContent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "fixed reply"})
	resp, err := vm.HandleAnthropic(makeReq("hi"))
	if err != nil {
		t.Fatalf("HandleAnthropic() should return nil error, got %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].OfText == nil || resp.Content[0].OfText.Text != "fixed reply" {
		t.Errorf("unexpected content: %+v", resp.Content[0])
	}
}

func TestMockModel_Static_Response_StopReason(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "x", FinishReason: "stop"})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "stop" {
		t.Errorf("expected StopReason 'stop', got %q", resp.StopReason)
	}
}

func TestMockModel_Static_Response_DefaultFinishReason(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "x"})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason == "" {
		t.Error("StopReason must not be empty")
	}
}

func TestMockModel_Static_SimulatedDelay(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "x", Delay: 123 * time.Millisecond})
	if vm.SimulatedDelay() != 123*time.Millisecond {
		t.Errorf("unexpected delay: %v", vm.SimulatedDelay())
	}
}

// ── MockModel (tool) ──────────────────────────────────────────────────────────

func TestMockModel_Tool_Handle_DoesNotModifyReq(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-test",
		ToolCall: &ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{"question": "proceed?"}},
	})
	req := makeReq("hi")
	_, _ = vm.HandleAnthropic(req)
	if len(req.Messages) != 1 {
		t.Error("HandleAnthropic() must not modify req for tool MockModel")
	}
}

func TestMockModel_Tool_Response_StopReasonIsToolUse(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-test",
		ToolCall: &ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "tool_use" {
		t.Errorf("expected StopReason 'tool_use', got %q", resp.StopReason)
	}
}

func TestMockModel_Tool_Response_ContentHasToolUseBlock(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-test",
		ToolCall: &ToolCallConfig{Name: "ask_user", Arguments: map[string]interface{}{"question": "sure?"}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))

	hasToolUse := false
	for _, blk := range resp.Content {
		if blk.OfToolUse != nil {
			hasToolUse = true
			if blk.OfToolUse.Name != "ask_user" {
				t.Errorf("expected tool name 'ask_user', got %q", blk.OfToolUse.Name)
			}
		}
	}
	if !hasToolUse {
		t.Error("expected a tool_use content block")
	}
}

func TestMockModel_Tool_Response_DisplayContent_Question(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-test",
		ToolCall: &ToolCallConfig{Name: "ask_user", Arguments: map[string]interface{}{"question": "are you sure?"}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))

	for _, blk := range resp.Content {
		if blk.OfText != nil && blk.OfText.Text == "are you sure?" {
			return
		}
	}
	t.Error("expected text block with question content")
}

// ── MockModel OpenAI Chat ──────────────────────────────────────────────────────

func TestMockModel_OpenAIChat_Static_Response(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hello openai"})
	resp, err := vm.HandleOpenAIChat(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello openai" {
		t.Errorf("expected 'hello openai', got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected FinishReason 'stop', got %q", resp.FinishReason)
	}
}

func TestMockModel_OpenAIChat_Tool_Response(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-openai",
		ToolCall: &ToolCallConfig{Name: "web_search", Arguments: map[string]interface{}{"query": "test"}},
	})
	resp, err := vm.HandleOpenAIChat(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Errorf("expected tool name 'web_search', got %q", resp.ToolCalls[0].Name)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected FinishReason 'tool_calls', got %q", resp.FinishReason)
	}
}

// ── TransformModel ────────────────────────────────────────────────────────────

func TestTransformModel_Handle_NoOpWhenNothingConfigured(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{ID: "t"})
	_, err := vm.HandleAnthropic(makeReq("hello"))
	if err != nil {
		t.Errorf("HandleAnthropic() with no chain/transformer should return nil error, got %v", err)
	}
}

func TestTransformModel_Handle_AppliesTransformer(t *testing.T) {
	applied := false
	vm := NewTransformModel(&TransformModelConfig{
		ID:          "t",
		Transformer: &recordingTransformer{called: &applied},
	})
	_, _ = vm.HandleAnthropic(makeReq("hi"))
	if !applied {
		t.Error("HandleAnthropic() should apply Transformer")
	}
}

func TestTransformModel_Handle_TransformerError_Wrapped(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{
		ID:          "t",
		Transformer: &failingTransformer{},
	})
	_, err := vm.HandleAnthropic(makeReq("hi"))
	if err == nil {
		t.Error("HandleAnthropic() should return error when transformer fails")
	}
}

func TestTransformModel_Response_ReturnsLastMessageText(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{ID: "t"})
	req := makeReq("first", "second", "last user msg")
	resp, _ := vm.HandleAnthropic(req)

	if len(resp.Content) != 1 || resp.Content[0].OfText == nil {
		t.Fatalf("expected 1 text block, got %+v", resp.Content)
	}
	if resp.Content[0].OfText.Text != "last user msg" {
		t.Errorf("expected last message text, got %q", resp.Content[0].OfText.Text)
	}
}

func TestTransformModel_Response_StopReasonIsEndTurn(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{ID: "t"})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "end_turn" {
		t.Errorf("expected StopReason 'end_turn', got %q", resp.StopReason)
	}
}

func TestTransformModel_SimulatedDelay_IsZero(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{ID: "t"})
	if vm.SimulatedDelay() != 0 {
		t.Errorf("TransformModel.SimulatedDelay() must be 0, got %v", vm.SimulatedDelay())
	}
}

func TestTransformModel_DoesNotImplementOpenAIChat(t *testing.T) {
	var base VirtualModel = NewTransformModel(&TransformModelConfig{ID: "t"})
	if _, ok := base.(OpenAIChatVirtualModel); ok {
		t.Error("TransformModel must not implement OpenAIChatVirtualModel")
	}
}

// ── MockScenario ───────────────────────────────────────────────────────────────

func TestMockScenario_AnthropicOnly(t *testing.T) {
	vm := NewMockModelFromScenario(&MockScenario{
		ID:        "sc-anthropic",
		Anthropic: &AnthropicMockResponse{Content: "anthropic response"},
	})
	avm, ok := vm.(AnthropicVirtualModel)
	if !ok {
		t.Fatal("scenario with Anthropic field must implement AnthropicVirtualModel")
	}
	resp, err := avm.HandleAnthropic(makeReq("hi"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) == 0 || resp.Content[0].OfText == nil || resp.Content[0].OfText.Text != "anthropic response" {
		t.Errorf("unexpected anthropic response: %+v", resp)
	}
	if resp.StopReason != "stop" {
		t.Errorf("expected StopReason 'stop', got %q", resp.StopReason)
	}
}

func TestMockScenario_OpenAIChatOnly(t *testing.T) {
	vm := NewMockModelFromScenario(&MockScenario{
		ID:         "sc-openai",
		OpenAIChat: &OpenAIChatMockResponse{Content: "openai response"},
	})
	ovm, ok := vm.(OpenAIChatVirtualModel)
	if !ok {
		t.Fatal("scenario with OpenAIChat field must implement OpenAIChatVirtualModel")
	}
	resp, err := ovm.HandleOpenAIChat(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "openai response" {
		t.Errorf("expected 'openai response', got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected FinishReason 'stop', got %q", resp.FinishReason)
	}
}

func TestMockScenario_DefaultStopReasons(t *testing.T) {
	sc := &MockScenario{
		ID: "sc-defaults",
		Anthropic: &AnthropicMockResponse{
			ToolCall: &ToolCallConfig{Name: "fn", Arguments: map[string]interface{}{}},
		},
		OpenAIChat: &OpenAIChatMockResponse{
			ToolCalls: []VToolCall{{ID: "t1", Name: "fn", Arguments: "{}"}},
		},
	}
	vm := NewMockModelFromScenario(sc)

	aResp, _ := vm.(AnthropicVirtualModel).HandleAnthropic(makeReq("hi"))
	if aResp.StopReason != "tool_use" {
		t.Errorf("expected anthropic StopReason 'tool_use', got %q", aResp.StopReason)
	}

	oResp, _ := vm.(OpenAIChatVirtualModel).HandleOpenAIChat(nil)
	if oResp.FinishReason != "tool_calls" {
		t.Errorf("expected openai FinishReason 'tool_calls', got %q", oResp.FinishReason)
	}
}

// ── test doubles ─────────────────────────────────────────────────────────────

type recordingTransformer struct{ called *bool }

func (r *recordingTransformer) HandleV1(_ *anthropic.MessageNewParams) error { return nil }
func (r *recordingTransformer) HandleV1Beta(_ *anthropic.BetaMessageNewParams) error {
	*r.called = true
	return nil
}

type failingTransformer struct{}

func (f *failingTransformer) HandleV1(_ *anthropic.MessageNewParams) error { return nil }
func (f *failingTransformer) HandleV1Beta(_ *anthropic.BetaMessageNewParams) error {
	return errTransformFailed
}

var errTransformFailed = fmt.Errorf("transformer exploded")

// ── Stream interface: MockModel ───────────────────────────────────────────────

func TestMockModel_HandleAnthropicStream_EmitsEvents(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hello world"})
	var events []any
	err := vm.HandleAnthropicStream(makeReq("hi"), func(ev any) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	// first event must be stream start
	if _, ok := events[0].(AnthropicStreamStartEvent); !ok {
		t.Errorf("expected first event AnthropicStreamStartEvent, got %T", events[0])
	}
	// last event must be done
	last := events[len(events)-1]
	if done, ok := last.(AnthropicDoneEvent); !ok {
		t.Errorf("expected last event AnthropicDoneEvent, got %T", last)
	} else if done.StopReason == "" {
		t.Error("AnthropicDoneEvent.StopReason must not be empty")
	}
	// must contain at least one text delta
	hasDelta := false
	for _, ev := range events {
		if _, ok := ev.(AnthropicTextDeltaEvent); ok {
			hasDelta = true
		}
	}
	if !hasDelta {
		t.Error("expected at least one AnthropicTextDeltaEvent")
	}
}

func TestMockModel_HandleAnthropicStream_ToolModel_EmitsToolUseEvent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool",
		ToolCall: &ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{"k": "v"}},
	})
	var events []any
	_ = vm.HandleAnthropicStream(makeReq("hi"), func(ev any) { events = append(events, ev) })

	hasToolUse := false
	for _, ev := range events {
		if tu, ok := ev.(AnthropicToolUseEvent); ok {
			hasToolUse = true
			if tu.Name != "my_tool" {
				t.Errorf("expected tool name 'my_tool', got %q", tu.Name)
			}
		}
	}
	if !hasToolUse {
		t.Error("expected AnthropicToolUseEvent")
	}
	// done event stop reason must be tool_use
	last := events[len(events)-1]
	if done, ok := last.(AnthropicDoneEvent); !ok || done.StopReason != "tool_use" {
		t.Errorf("expected AnthropicDoneEvent with StopReason 'tool_use', got %+v", last)
	}
}

func TestMockModel_HandleOpenAIChatStream_EmitsEvents(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hi there"})
	var events []any
	err := vm.HandleOpenAIChatStream(nil, func(ev any) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// last event must be done
	last := events[len(events)-1]
	if done, ok := last.(OpenAIChatDoneEvent); !ok {
		t.Errorf("expected OpenAIChatDoneEvent, got %T", last)
	} else if done.FinishReason == "" {
		t.Error("OpenAIChatDoneEvent.FinishReason must not be empty")
	}
}

func TestMockModel_HandleOpenAIChatStream_ToolModel_EmitsToolEvent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-openai",
		ToolCall: &ToolCallConfig{Name: "search", Arguments: map[string]interface{}{"q": "test"}},
	})
	var events []any
	_ = vm.HandleOpenAIChatStream(nil, func(ev any) { events = append(events, ev) })

	hasToolEvent := false
	for _, ev := range events {
		if te, ok := ev.(OpenAIChatToolEvent); ok {
			hasToolEvent = true
			if te.ToolCall.Name != "search" {
				t.Errorf("expected tool name 'search', got %q", te.ToolCall.Name)
			}
		}
	}
	if !hasToolEvent {
		t.Error("expected OpenAIChatToolEvent")
	}
}

// ── Stream interface: TransformModel ─────────────────────────────────────────

func TestTransformModel_HandleAnthropicStream_UsesDefaultAdapter(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{ID: "t"})
	var events []any
	err := vm.HandleAnthropicStream(makeReq("hello"), func(ev any) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events from default adapter")
	}
	if _, ok := events[0].(AnthropicStreamStartEvent); !ok {
		t.Errorf("expected AnthropicStreamStartEvent first, got %T", events[0])
	}
}

// ── DefaultAnthropicStream / DefaultOpenAIChatStream ─────────────────────────

func TestDefaultAnthropicStream_ReconstructsContent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "reconstruct me"})
	var texts []string
	_ = DefaultAnthropicStream(vm, makeReq("hi"), func(ev any) {
		if delta, ok := ev.(AnthropicTextDeltaEvent); ok {
			texts = append(texts, delta.Text)
		}
	})
	reconstructed := ""
	for _, s := range texts {
		reconstructed += s
	}
	if reconstructed != "reconstruct me" {
		t.Errorf("expected 'reconstruct me', got %q", reconstructed)
	}
}

func TestDefaultOpenAIChatStream_ReconstructsContent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hello there"})
	var texts []string
	_ = DefaultOpenAIChatStream(vm, nil, func(ev any) {
		if delta, ok := ev.(OpenAIChatDeltaEvent); ok {
			texts = append(texts, delta.Content)
		}
	})
	reconstructed := ""
	for _, s := range texts {
		reconstructed += s
	}
	if reconstructed != "hello there" {
		t.Errorf("expected 'hello there', got %q", reconstructed)
	}
}
