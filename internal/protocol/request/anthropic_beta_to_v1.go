package request

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// ConvertAnthropicBetaToV1Request projects an Anthropic Beta MessageNewParams
// onto the v1 MessageNewParams shape. Beta is a structural superset of v1,
// so this is a best-effort field-by-field copy of the fields that v1 supports.
//
// Coverage mirrors ConvertAnthropicV1ToBetaRequest: Model, MaxTokens,
// Thinking-enabled flag, System text, Messages with text / image / tool_use /
// tool_result blocks. Extend as needed if other call sites require additional
// fields.
func ConvertAnthropicBetaToV1Request(req *anthropic.BetaMessageNewParams) *anthropic.MessageNewParams {
	if req == nil {
		return nil
	}

	v1 := &anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: req.MaxTokens,
	}

	if req.Thinking.OfEnabled != nil {
		v1.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: req.Thinking.OfEnabled.BudgetTokens,
			},
		}
	}

	if len(req.System) > 0 {
		v1.System = make([]anthropic.TextBlockParam, 0, len(req.System))
		for _, s := range req.System {
			if s.Text == "" {
				continue
			}
			v1.System = append(v1.System, anthropic.TextBlockParam{Text: s.Text})
		}
	}

	if len(req.Messages) > 0 {
		v1.Messages = make([]anthropic.MessageParam, 0, len(req.Messages))
		for _, msg := range req.Messages {
			v1.Messages = append(v1.Messages, anthropic.MessageParam{
				Role:    anthropic.MessageParamRole(msg.Role),
				Content: convertAnthropicBetaContentToV1(msg.Content),
			})
		}
	}

	return v1
}

func convertAnthropicBetaContentToV1(blocks []anthropic.BetaContentBlockParamUnion) []anthropic.ContentBlockParamUnion {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
	for _, b := range blocks {
		switch {
		case b.OfText != nil:
			out = append(out, anthropic.NewTextBlock(b.OfText.Text))
		case b.OfImage != nil:
			out = append(out, anthropic.ContentBlockParamUnion{
				OfImage: &anthropic.ImageBlockParam{},
			})
		case b.OfToolUse != nil:
			out = append(out, anthropic.NewToolUseBlock(
				b.OfToolUse.ID,
				b.OfToolUse.Input,
				b.OfToolUse.Name,
			))
		case b.OfToolResult != nil:
			out = append(out, anthropic.NewToolResultBlock(
				b.OfToolResult.ToolUseID,
				"",
				false,
			))
		}
	}
	return out
}
