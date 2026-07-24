package request

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ConvertAnthropicV1ToBetaRequest projects an Anthropic v1 MessageNewParams onto
// the Beta MessageNewParams shape. Beta is a structural superset of v1 in the SDK,
// so this is a field-by-field copy.
//
// Coverage is limited to the fields downstream consumers (currently smart-routing
// context extraction) read: Model, MaxTokens, Thinking-enabled flag, System text,
// Messages with text / image / tool_use / tool_result blocks. Extend as needed if
// other call sites require additional fields.
func ConvertAnthropicV1ToBetaRequest(req *anthropic.MessageNewParams) *anthropic.BetaMessageNewParams {
	if req == nil {
		return nil
	}

	beta := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: req.MaxTokens,
	}

	if req.Thinking.OfEnabled != nil {
		beta.Thinking = anthropic.BetaThinkingConfigParamUnion{
			OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{
				BudgetTokens: req.Thinking.OfEnabled.BudgetTokens,
			},
		}
	}

	if len(req.System) > 0 {
		beta.System = make([]anthropic.BetaTextBlockParam, 0, len(req.System))
		for _, s := range req.System {
			if s.Text == "" {
				continue
			}
			beta.System = append(beta.System, anthropic.BetaTextBlockParam{Text: s.Text})
		}
	}

	if len(req.Messages) > 0 {
		beta.Messages = make([]anthropic.BetaMessageParam, 0, len(req.Messages))
		for _, msg := range req.Messages {
			beta.Messages = append(beta.Messages, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRole(msg.Role),
				Content: convertAnthropicV1ContentToBeta(msg.Content),
			})
		}
	}

	return beta
}

func convertAnthropicV1ContentToBeta(blocks []anthropic.ContentBlockParamUnion) []anthropic.BetaContentBlockParamUnion {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]anthropic.BetaContentBlockParamUnion, 0, len(blocks))
	for _, b := range blocks {
		switch {
		case b.OfText != nil:
			out = append(out, anthropic.NewBetaTextBlock(b.OfText.Text))
		case b.OfImage != nil:
			out = append(out, anthropic.BetaContentBlockParamUnion{
				OfImage: &anthropic.BetaImageBlockParam{},
			})
		case b.OfToolUse != nil:
			out = append(out, anthropic.NewBetaToolUseBlock(
				b.OfToolUse.ID,
				b.OfToolUse.Input,
				b.OfToolUse.Name,
			))
		case b.OfToolResult != nil:
			out = append(out, anthropic.NewBetaToolResultBlock(
				b.OfToolResult.ToolUseID,
				joinV1ToolResultText(b.OfToolResult.Content),
				false,
			))
		}
	}
	return out
}

// joinV1ToolResultText concatenates the text blocks of a v1 tool_result's
// content into a single string, mirroring convertToolResultContent in
// anthropic_v1_to_openai.go.
func joinV1ToolResultText(content []anthropic.ToolResultBlockParamContentUnion) string {
	var texts []string
	for _, c := range content {
		if c.OfText != nil && c.OfText.Text != "" {
			texts = append(texts, c.OfText.Text)
		}
	}
	return strings.Join(texts, "\n")
}
