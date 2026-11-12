package upstream

import (
	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
)

// omitThinkingBudget removes only the Anthropic budget from the final request.
func omitThinkingBudget(request any) {
	switch req := request.(type) {
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
	}
}
