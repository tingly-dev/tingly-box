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

func TestRuleThinkingTransform_AnthropicBetaBudget(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{}
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

func TestRuleThinkingTransform_AnthropicBetaOffDisables(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Thinking: anthropic.BetaThinkingConfigParamOfEnabled(20480),
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

func TestRuleThinkingTransform_ResponsesOff(t *testing.T) {
	req := &responses.ResponseNewParams{}
	req.Reasoning.Effort = shared.ReasoningEffortMedium
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Reasoning.Effort != "" {
		t.Errorf("reasoning.effort = %q, want empty", req.Reasoning.Effort)
	}
}

func TestRuleThinkingTransform_AnthropicCapsBudgetToMaxTokens(t *testing.T) {
	budget := typ.ThinkingBudgetMapping[typ.ThinkingEffortHigh]
	req := &anthropic.MessageNewParams{}
	req.MaxTokens = budget / 2 // smaller than the budget
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortHigh).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// max_tokens must not be raised (hard operator limit)
	if req.MaxTokens != budget/2 {
		t.Errorf("max_tokens changed from %d to %d — must not be raised", budget/2, req.MaxTokens)
	}
	// budget must be capped at max_tokens so Anthropic doesn't reject
	if got := req.Thinking.OfEnabled; got == nil {
		t.Fatalf("expected thinking enabled")
	} else if got.BudgetTokens > req.MaxTokens {
		t.Errorf("budget_tokens %d exceeds max_tokens %d", got.BudgetTokens, req.MaxTokens)
	}
}

func TestRuleThinkingTransform_AnthropicCapsBudgetToMaxTokensWhenBelowMinimum(t *testing.T) {
	// max_tokens=512 is below Anthropic's 1024 thinking minimum. The old code used
	// max(1024, MaxTokens) which would set budget=1024 > MaxTokens=512 and cause a
	// 400 from Anthropic. The correct behavior is to cap at MaxTokens and let the
	// API surface the conflict rather than silently exceeding the operator limit.
	req := &anthropic.MessageNewParams{}
	req.MaxTokens = 512
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortLow).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.MaxTokens != 512 {
		t.Errorf("max_tokens changed from 512 to %d — must not be raised", req.MaxTokens)
	}
	if got := req.Thinking.OfEnabled; got == nil {
		t.Fatalf("expected thinking enabled")
	} else if got.BudgetTokens > req.MaxTokens {
		t.Errorf("budget_tokens %d exceeds max_tokens %d — would cause Anthropic 400", got.BudgetTokens, req.MaxTokens)
	}
}

func TestRuleThinkingTransform_AnthropicDoesNotReduceBudgetWhenMaxTokensSufficient(t *testing.T) {
	budget := typ.ThinkingBudgetMapping[typ.ThinkingEffortLow]
	req := &anthropic.MessageNewParams{}
	req.MaxTokens = budget * 4 // already larger than budget
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortLow).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.MaxTokens != budget*4 {
		t.Errorf("max_tokens = %d, should not have changed from %d", req.MaxTokens, budget*4)
	}
	if got := req.Thinking.OfEnabled; got == nil {
		t.Fatalf("expected thinking enabled")
	} else if got.BudgetTokens != budget {
		t.Errorf("budget = %d, want %d", got.BudgetTokens, budget)
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
