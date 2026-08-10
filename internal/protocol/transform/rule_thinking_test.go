package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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
	if req.OutputConfig.Effort != anthropic.OutputConfigEffortHigh {
		t.Errorf("output_config.effort = %q, want high", req.OutputConfig.Effort)
	}
}

func TestRuleThinkingTransform_AnthropicAdaptivePreserved(t *testing.T) {
	// A request already using adaptive thinking (Claude 4.6+ dialect) keeps
	// adaptive; the forced level lands on output_config.effort only.
	req := &anthropic.MessageNewParams{
		Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
	}
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortMax).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Thinking.OfAdaptive == nil {
		t.Fatalf("adaptive thinking should be preserved, got %#v", req.Thinking)
	}
	if req.OutputConfig.Effort != anthropic.OutputConfigEffortMax {
		t.Errorf("output_config.effort = %q, want max", req.OutputConfig.Effort)
	}
}

func TestRuleThinkingTransform_AnthropicEffortMapping(t *testing.T) {
	// Anthropic has no minimal level, while xhigh is a distinct native level
	// on supported models. Model-specific clamping happens in the vendor stage.
	for level, want := range map[string]anthropic.OutputConfigEffort{
		typ.ThinkingEffortMinimal: anthropic.OutputConfigEffortLow,
		typ.ThinkingEffortXHigh:   anthropic.OutputConfigEffortXhigh,
	} {
		req := &anthropic.MessageNewParams{}
		ctx := &TransformContext{Request: req}
		if err := NewRuleThinkingTransform(level).Apply(ctx); err != nil {
			t.Fatalf("apply(%s): %v", level, err)
		}
		if req.OutputConfig.Effort != want {
			t.Errorf("effort %s: output_config.effort = %q, want %q", level, req.OutputConfig.Effort, want)
		}
		if req.Thinking.OfEnabled == nil {
			t.Fatalf("effort %s: expected budget thinking enabled", level)
		}
		if got := req.Thinking.OfEnabled.BudgetTokens; got != typ.ThinkingBudgetMapping[level] {
			t.Errorf("effort %s: budget = %d, want %d", level, got, typ.ThinkingBudgetMapping[level])
		}
	}
}

func TestRuleThinkingTransform_AnthropicNewLadderLevels(t *testing.T) {
	// minimal and xhigh are new to this ladder; confirm both drive budget_tokens
	// the same way the pre-existing levels do.
	for _, level := range []string{typ.ThinkingEffortMinimal, typ.ThinkingEffortXHigh} {
		req := &anthropic.MessageNewParams{}
		ctx := &TransformContext{Request: req}

		if err := NewRuleThinkingTransform(level).Apply(ctx); err != nil {
			t.Fatalf("apply(%s): %v", level, err)
		}
		if req.Thinking.OfEnabled == nil {
			t.Fatalf("effort %s: expected thinking enabled, got %#v", level, req.Thinking)
		}
		if got := req.Thinking.OfEnabled.BudgetTokens; got != typ.ThinkingBudgetMapping[level] {
			t.Errorf("effort %s: budget = %d, want %d", level, got, typ.ThinkingBudgetMapping[level])
		}
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

func TestRuleThinkingTransform_OpenAIFullLadderIsNative(t *testing.T) {
	// All six ladder levels are OpenAI-defined now — no collapsing.
	cases := map[string]shared.ReasoningEffort{
		typ.ThinkingEffortMinimal: shared.ReasoningEffortMinimal,
		typ.ThinkingEffortLow:     shared.ReasoningEffortLow,
		typ.ThinkingEffortMedium:  shared.ReasoningEffortMedium,
		typ.ThinkingEffortHigh:    shared.ReasoningEffortHigh,
		typ.ThinkingEffortXHigh:   shared.ReasoningEffortXhigh,
		typ.ThinkingEffortMax:     shared.ReasoningEffortMax,
	}
	for level, want := range cases {
		req := &openai.ChatCompletionNewParams{}
		ctx := &TransformContext{Request: req}
		if err := NewRuleThinkingTransform(level).Apply(ctx); err != nil {
			t.Fatalf("apply(%s): %v", level, err)
		}
		if req.ReasoningEffort != want {
			t.Errorf("effort %s: reasoning_effort = %q, want %q", level, req.ReasoningEffort, want)
		}
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

	ctx := &TransformContext{
		Request: req,
		Config: TransformConfig{OpenAIConfig: &protocol.OpenAIConfig{
			HasThinking:     true,
			ReasoningEffort: shared.ReasoningEffortMedium,
		}},
	}
	if err := NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("reasoning_effort = %q, want empty", req.ReasoningEffort)
	}
	if _, has := req.ExtraFields()["thinking"]; has {
		t.Errorf("expected `thinking` extra field to be stripped, still present: %#v", req.ExtraFields())
	}
	if ctx.Config.OpenAIConfig.HasThinking {
		t.Error("stale base config still reports thinking enabled")
	}
	if ctx.Config.OpenAIConfig.ReasoningEffort != "" {
		t.Errorf("config reasoning_effort = %q, want empty", ctx.Config.OpenAIConfig.ReasoningEffort)
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
	ctx := &TransformContext{
		Request: req,
		Config: TransformConfig{ResponsesConfig: &protocol.OpenAIConfig{
			HasThinking:     true,
			ReasoningEffort: shared.ReasoningEffortMedium,
		}},
	}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortOff).Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if req.Reasoning.Effort != "" {
		t.Errorf("reasoning.effort = %q, want empty", req.Reasoning.Effort)
	}
	if ctx.Config.ResponsesConfig.HasThinking {
		t.Error("stale responses config still reports thinking enabled")
	}
	if ctx.Config.ResponsesConfig.ReasoningEffort != "" {
		t.Errorf("config reasoning_effort = %q, want empty", ctx.Config.ResponsesConfig.ReasoningEffort)
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
	// budget must be strictly below max_tokens so Anthropic doesn't reject it.
	if got := req.Thinking.OfEnabled; got == nil {
		t.Fatalf("expected thinking enabled")
	} else if got.BudgetTokens != req.MaxTokens-1 {
		t.Errorf("budget_tokens = %d, want max_tokens-1 = %d", got.BudgetTokens, req.MaxTokens-1)
	}
}

func TestRuleThinkingTransform_AnthropicCapsBudgetToMaxTokensWhenBelowMinimum(t *testing.T) {
	// No budget can be both >=1024 and <512, so reject the impossible rule
	// locally instead of emitting a request guaranteed to fail upstream.
	req := &anthropic.MessageNewParams{}
	req.MaxTokens = 512
	ctx := &TransformContext{Request: req}

	if err := NewRuleThinkingTransform(typ.ThinkingEffortLow).Apply(ctx); err == nil {
		t.Fatal("expected impossible thinking budget to return an error")
	}
	if req.MaxTokens != 512 {
		t.Errorf("max_tokens changed from 512 to %d — must not be raised", req.MaxTokens)
	}
	if req.Thinking.OfEnabled != nil {
		t.Errorf("invalid thinking config was applied: %#v", req.Thinking.OfEnabled)
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
