package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRuleThinkingTransform_AnthropicBudget(t *testing.T) {
	req := &anthropic.MessageNewParams{}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortHigh).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Thinking.OfEnabled == nil {
		t.Fatalf("expected thinking enabled, got %#v", req.Thinking)
	}
	if got := req.Thinking.OfEnabled.BudgetTokens; got != typ.ThinkingBudgetMapping[typ.ThinkingEffortHigh] {
		t.Errorf("budget = %d, want %d", got, typ.ThinkingBudgetMapping[typ.ThinkingEffortHigh])
	}
}

func TestRuleThinkingTransform_AnthropicOffDisables(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Thinking: anthropic.ThinkingConfigParamOfEnabled(20480),
	}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Thinking.OfDisabled == nil {
		t.Fatalf("expected OfDisabled, got %#v", req.Thinking)
	}
	if req.Thinking.OfEnabled != nil {
		t.Errorf("expected OfEnabled cleared, got %#v", req.Thinking.OfEnabled)
	}
}

func TestRuleThinkingTransform_OpenAIChatEffort(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortLow).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != shared.ReasoningEffortLow {
		t.Errorf("reasoning_effort = %q, want %q", req.ReasoningEffort, shared.ReasoningEffortLow)
	}
}

func TestRuleThinkingTransform_OpenAIMaxCollapsesToHigh(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortMax).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != shared.ReasoningEffortHigh {
		t.Errorf("max should collapse to high, got %q", req.ReasoningEffort)
	}
}

func TestRuleThinkingTransform_OpenAIOffStripsThinkingExtra(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		ReasoningEffort: shared.ReasoningEffortMedium,
	}
	// Simulate an upstream-bound request that picked up a `thinking` blob
	// from a prior Anthropic-style client or vendor transform.
	req.SetExtraFields(map[string]interface{}{
		"thinking": map[string]interface{}{"type": "enabled"},
	})

	ctx := &TransformContext{Request: req}
	if err := NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("reasoning_effort = %q, want empty", req.ReasoningEffort)
	}
	if _, has := req.ExtraFields()["thinking"]; has {
		t.Errorf("expected `thinking` extra field to be stripped, still present: %#v", req.ExtraFields())
	}
}

func TestRuleThinkingTransform_OpenAILevelStripsThinkingExtra(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	req.SetExtraFields(map[string]interface{}{
		"thinking": map[string]interface{}{"type": "disabled"},
		"other":    "keep-me",
	})

	ctx := &TransformContext{Request: req}
	if err := NewRuleThinkingTransform(typ.ThinkingEffortHigh).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != shared.ReasoningEffortHigh {
		t.Errorf("reasoning_effort = %q, want high", req.ReasoningEffort)
	}
	extras := req.ExtraFields()
	if _, has := extras["thinking"]; has {
		t.Errorf("expected `thinking` extra stripped, got %#v", extras)
	}
	if extras["other"] != "keep-me" {
		t.Errorf("unrelated extras should be preserved, got %#v", extras)
	}
}

func TestRuleThinkingTransform_ResponsesEffort(t *testing.T) {
	req := &responses.ResponseNewParams{}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortMedium).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Reasoning.Effort != shared.ReasoningEffortMedium {
		t.Errorf("reasoning.effort = %q, want %q", req.Reasoning.Effort, shared.ReasoningEffortMedium)
	}
}

func TestRuleThinkingTransform_DefaultIsNoop(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		ReasoningEffort: shared.ReasoningEffortMedium,
	}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortDefault).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != shared.ReasoningEffortMedium {
		t.Errorf("default effort must not touch reasoning_effort, got %q", req.ReasoningEffort)
	}
}

func TestRuleThinkingTransform_UnknownEffortIsNoop(t *testing.T) {
	req := &openai.ChatCompletionNewParams{}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform("bogus").Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("expected no-op for unknown effort, got %q", req.ReasoningEffort)
	}
}
