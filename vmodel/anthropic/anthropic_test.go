package anthropic

import (
	"fmt"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// helpers

func makeReq(texts ...string) *protocol.AnthropicBetaMessagesRequest {
	msgs := make([]sdk.BetaMessageParam, 0, len(texts))
	for i, t := range texts {
		role := sdk.BetaMessageParamRoleUser
		if i%2 == 1 {
			role = sdk.BetaMessageParamRoleAssistant
		}
		msgs = append(msgs, sdk.BetaMessageParam{
			Role:    role,
			Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock(t)},
		})
	}
	return &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: sdk.BetaMessageNewParams{Messages: msgs},
	}
}

// ── compile-time interface checks ────────────────────────────────────────────

var _ VirtualModel = (*MockModel)(nil)
var _ VirtualModel = (*TransformModel)(nil)

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
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "x", StopReason: "stop"})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "stop" {
		t.Errorf("expected StopReason 'stop', got %q", resp.StopReason)
	}
}

func TestMockModel_Static_Response_DefaultStopReason(t *testing.T) {
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
		ToolCall: &vmodel.ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{"question": "proceed?"}},
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
		ToolCall: &vmodel.ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "tool_use" {
		t.Errorf("expected StopReason 'tool_use', got %q", resp.StopReason)
	}
}

func TestMockModel_Tool_Response_ContentHasToolUseBlock(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-test",
		ToolCall: &vmodel.ToolCallConfig{Name: "ask_user", Arguments: map[string]interface{}{"question": "sure?"}},
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
		ToolCall: &vmodel.ToolCallConfig{Name: "ask_user", Arguments: map[string]interface{}{"question": "are you sure?"}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))

	for _, blk := range resp.Content {
		if blk.OfText != nil && blk.OfText.Text == "are you sure?" {
			return
		}
	}
	t.Error("expected text block with question content")
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

// ── MockScenario ──────────────────────────────────────────────────────────────

func TestMockScenario_Anthropic(t *testing.T) {
	vm := NewMockFromScenario(&MockScenario{
		ID:      "sc-anthropic",
		Content: "anthropic response",
	})
	resp, err := vm.HandleAnthropic(makeReq("hi"))
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

func TestMockScenario_DefaultStopReasons(t *testing.T) {
	vm := NewMockFromScenario(&MockScenario{
		ID:       "sc-defaults",
		ToolCall: &vmodel.ToolCallConfig{Name: "fn", Arguments: map[string]interface{}{}},
	})
	resp, _ := vm.HandleAnthropic(makeReq("hi"))
	if resp.StopReason != "tool_use" {
		t.Errorf("expected StopReason 'tool_use', got %q", resp.StopReason)
	}
}

// ── Stream ────────────────────────────────────────────────────────────────────

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
	if _, ok := events[0].(StreamStartEvent); !ok {
		t.Errorf("expected first event StreamStartEvent, got %T", events[0])
	}
	last := events[len(events)-1]
	if done, ok := last.(DoneEvent); !ok {
		t.Errorf("expected last event DoneEvent, got %T", last)
	} else if done.StopReason == "" {
		t.Error("DoneEvent.StopReason must not be empty")
	}
	hasDelta := false
	for _, ev := range events {
		if _, ok := ev.(TextDeltaEvent); ok {
			hasDelta = true
		}
	}
	if !hasDelta {
		t.Error("expected at least one TextDeltaEvent")
	}
}

func TestMockModel_HandleAnthropicStream_ToolModel_EmitsToolUseEvent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool",
		ToolCall: &vmodel.ToolCallConfig{Name: "my_tool", Arguments: map[string]interface{}{"k": "v"}},
	})
	var events []any
	_ = vm.HandleAnthropicStream(makeReq("hi"), func(ev any) { events = append(events, ev) })

	hasToolUse := false
	for _, ev := range events {
		if tu, ok := ev.(ToolUseEvent); ok {
			hasToolUse = true
			if tu.Name != "my_tool" {
				t.Errorf("expected tool name 'my_tool', got %q", tu.Name)
			}
		}
	}
	if !hasToolUse {
		t.Error("expected ToolUseEvent")
	}
	last := events[len(events)-1]
	if done, ok := last.(DoneEvent); !ok || done.StopReason != "tool_use" {
		t.Errorf("expected DoneEvent with StopReason 'tool_use', got %+v", last)
	}
}

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
	if _, ok := events[0].(StreamStartEvent); !ok {
		t.Errorf("expected StreamStartEvent first, got %T", events[0])
	}
}

func TestDefaultStream_ReconstructsContent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "reconstruct me"})
	var texts []string
	_ = DefaultStream(vm, makeReq("hi"), func(ev any) {
		if delta, ok := ev.(TextDeltaEvent); ok {
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

// ── Registry ──────────────────────────────────────────────────────────────────

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	vm := NewMockModel(&MockModelConfig{ID: "reg-test", Content: "x"})
	if err := r.Register(vm); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if r.Get("reg-test") == nil {
		t.Error("Get should return registered model")
	}
	if err := r.Register(vm); err == nil {
		t.Error("duplicate Register should fail")
	}
	r.Unregister("reg-test")
	if r.Get("reg-test") != nil {
		t.Error("Unregister should remove model")
	}
}

func TestRegistry_RegisterDefaults(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	expected := []string{
		"virtual-claude-3", "echo-model",
		"ask-user-question", "ask-confirmation", "web-search-example",
		"compact-thinking", "compact-round-only", "compact-round-files",
		"claude-code-compact", "claude-code-strategy",
	}
	for _, id := range expected {
		if r.Get(id) == nil {
			t.Errorf("expected default model %q to be registered", id)
		}
	}
	if got := r.Get("virtual-gpt-4"); got != nil {
		t.Error("virtual-gpt-4 must NOT be in the Anthropic registry")
	}
}

// ── test doubles ─────────────────────────────────────────────────────────────

type recordingTransformer struct{ called *bool }

func (r *recordingTransformer) Name() string { return "recording" }
func (r *recordingTransformer) Apply(_ *transform.TransformContext) error {
	*r.called = true
	return nil
}

type failingTransformer struct{}

func (f *failingTransformer) Name() string { return "failing" }
func (f *failingTransformer) Apply(_ *transform.TransformContext) error {
	return errTransformFailed
}

var errTransformFailed = fmt.Errorf("transformer exploded")
