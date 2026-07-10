package openai

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// ── compile-time interface checks ────────────────────────────────────────────

var _ VirtualModel = (*MockModel)(nil)

// ── MockModel ─────────────────────────────────────────────────────────────────

func TestMockModel_Static_Response(t *testing.T) {
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

func TestMockModel_Tool_Response(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-openai",
		ToolCall: &vmodel.ToolCallConfig{Name: "web_search", Arguments: map[string]interface{}{"query": "test"}},
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

func TestMockModel_SimulatedDelay(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "x", Delay: 99 * time.Millisecond})
	if vm.SimulatedDelay() != 99*time.Millisecond {
		t.Errorf("unexpected delay: %v", vm.SimulatedDelay())
	}
}

// ── MockScenario ──────────────────────────────────────────────────────────────

func TestMockScenario_OpenAI(t *testing.T) {
	vm := NewMockFromScenario(&MockScenario{
		ID:      "sc-openai",
		Content: "openai response",
	})
	resp, err := vm.HandleOpenAIChat(nil)
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

func TestMockScenario_DefaultFinishReason_Tool(t *testing.T) {
	vm := NewMockFromScenario(&MockScenario{
		ID:        "sc-defaults",
		ToolCalls: []VToolCall{{ID: "t1", Name: "fn", Arguments: "{}"}},
	})
	resp, _ := vm.HandleOpenAIChat(nil)
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected FinishReason 'tool_calls', got %q", resp.FinishReason)
	}
}

// ── Stream ────────────────────────────────────────────────────────────────────

func TestMockModel_HandleOpenAIChatStream_EmitsEvents(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hi there"})
	var events []any
	err := vm.HandleOpenAIChatStream(context.Background(), nil, func(ev any) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := events[len(events)-1]
	if done, ok := last.(DoneEvent); !ok {
		t.Errorf("expected DoneEvent, got %T", last)
	} else if done.FinishReason == "" {
		t.Error("DoneEvent.FinishReason must not be empty")
	}
}

func TestMockModel_HandleOpenAIChatStream_ToolModel(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{
		ID:       "tool-openai",
		ToolCall: &vmodel.ToolCallConfig{Name: "search", Arguments: map[string]interface{}{"q": "test"}},
	})
	var events []any
	_ = vm.HandleOpenAIChatStream(context.Background(), nil, func(ev any) { events = append(events, ev) })

	hasToolEvent := false
	for _, ev := range events {
		if te, ok := ev.(ToolEvent); ok {
			hasToolEvent = true
			if te.ToolCall.Name != "search" {
				t.Errorf("expected tool name 'search', got %q", te.ToolCall.Name)
			}
		}
	}
	if !hasToolEvent {
		t.Error("expected ToolEvent")
	}
}

func TestDefaultStream_ReconstructsContent(t *testing.T) {
	vm := NewMockModel(&MockModelConfig{ID: "s", Content: "hello there"})
	var texts []string
	_ = DefaultStream(context.Background(), vm, nil, func(ev any) {
		if delta, ok := ev.(DeltaEvent); ok {
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
		"virtual-gpt-4", "echo-model",
		"ask-user-question", "ask-confirmation", "web-search-example",
	}
	for _, id := range expected {
		if r.Get(id) == nil {
			t.Errorf("expected default model %q to be registered", id)
		}
	}
	if got := r.Get("virtual-claude-3"); got != nil {
		t.Error("virtual-claude-3 must NOT be in the OpenAI registry")
	}
	if got := r.Get("compact-round-only"); got != nil {
		t.Error("compact-round-only must NOT be in the OpenAI registry")
	}
}
