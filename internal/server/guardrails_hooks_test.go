package server

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

type stubGuardrails struct {
	input guardrailscore.Input
}

func (s *stubGuardrails) Evaluate(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	s.input = input
	return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
}

func TestGuardrailsHookCollectsText(t *testing.T) {
	policy := &stubGuardrails{}
	runtime := &guardrails.Guardrails{Policy: policy}
	base := guardrailscore.Input{Scenario: "openai", Model: "gpt-4.1-mini"}

	onEvent, onComplete, _ := NewGuardrailsHooks(runtime, base)
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

	if policy.input.Content.Text != "hello" {
		t.Fatalf("expected text to be collected, got %q", policy.input.Content.Text)
	}
	if policy.input.Direction != guardrailscore.DirectionResponse {
		t.Fatalf("expected response direction, got %q", policy.input.Direction)
	}
}
