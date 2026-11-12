package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// RemoveThinkingBudgetTransform removes the budget while leaving the thinking
// mode and all other fields untouched.
type RemoveThinkingBudgetTransform struct{}

func NewRemoveThinkingBudgetTransform() *RemoveThinkingBudgetTransform {
	return &RemoveThinkingBudgetTransform{}
}

func (*RemoveThinkingBudgetTransform) Name() string { return "remove_thinking_budget" }

func (*RemoveThinkingBudgetTransform) Apply(ctx *TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		if req.Thinking.OfEnabled != nil {
			extra := req.Thinking.OfEnabled.ExtraFields()
			if extra == nil {
				extra = make(map[string]any)
			}
			extra["budget_tokens"] = anthropicparam.Omit
			req.Thinking.OfEnabled.SetExtraFields(extra)
		}
	case *anthropic.BetaMessageNewParams:
		if req.Thinking.OfEnabled != nil {
			extra := req.Thinking.OfEnabled.ExtraFields()
			if extra == nil {
				extra = make(map[string]any)
			}
			extra["budget_tokens"] = anthropicparam.Omit
			req.Thinking.OfEnabled.SetExtraFields(extra)
		}
	case *openai.ChatCompletionNewParams:
		extra := req.ExtraFields()
		if _, ok := extra["thinking_budget"]; ok {
			delete(extra, "thinking_budget")
			req.SetExtraFields(extra)
		}
	case *responses.ResponseNewParams:
		extra := req.ExtraFields()
		if _, ok := extra["thinking_budget"]; ok {
			delete(extra, "thinking_budget")
			req.SetExtraFields(extra)
		}
	}
	return nil
}
