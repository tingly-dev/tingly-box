package server

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type stubGuardrails struct {
	input guardrails.Input
}

func (s *stubGuardrails) Evaluate(_ context.Context, input guardrails.Input) (guardrails.Result, error) {
	s.input = input
	return guardrails.Result{Verdict: guardrails.VerdictAllow}, nil
}

func TestGuardrailsHookCollectsText(t *testing.T) {
	engine := &stubGuardrails{}
	base := guardrails.Input{Scenario: "openai", Model: "gpt-4.1-mini"}

	onEvent, onComplete, _ := NewGuardrailsHooks(engine, base)
	if onEvent == nil || onComplete == nil {
		t.Fatalf("expected hooks")
	}

	err := onEvent(map[string]interface{}{
		"type": "content_block_delta",
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": "hello",
		},
	})
	if err != nil {
		t.Fatalf("onEvent: %v", err)
	}

	onComplete()

	if engine.input.Content.Text != "hello" {
		t.Fatalf("expected text to be collected, got %q", engine.input.Content.Text)
	}
	if engine.input.Direction != guardrails.DirectionResponse {
		t.Fatalf("expected response direction, got %q", engine.input.Direction)
	}
}
