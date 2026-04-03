package adapter

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// AdaptMessagesFromAnthropicV1Beta converts Anthropic beta request history into
// the shared guardrails message format used as evaluation context.
func AdaptMessagesFromAnthropicV1Beta(system []anthropic.BetaTextBlockParam, messages []anthropic.BetaMessageParam) []guardrailscore.Message {
	out := make([]guardrailscore.Message, 0, len(messages)+1)

	if len(system) > 0 {
		out = append(out, guardrailscore.Message{
			Role:    "system",
			Content: request.ConvertBetaTextBlocksToString(system),
		})
	}

	for _, msg := range messages {
		out = append(out, guardrailscore.Message{
			Role:    string(msg.Role),
			Content: request.ConvertBetaContentBlocksToString(msg.Content),
		})
	}

	return out
}

// ResponseViewFromAnthropicV1BetaResponse adapts a non-stream Anthropic beta
// response into the shared response view.
func ResponseViewFromAnthropicV1BetaResponse(messageHistory []guardrailscore.Message, resp *anthropic.BetaMessage) guardrailsevaluate.ResponseView {
	if resp == nil {
		return guardrailsevaluate.ResponseView{MessageHistory: messageHistory}
	}
	return guardrailsevaluate.ResponseView{
		Text:           responseTextFromAnthropicV1BetaBlocks(resp.Content),
		Command:        commandFromAnthropicV1BetaBlocks(resp.Content),
		MessageHistory: messageHistory,
	}
}

// AdaptToolResultRequestFromAnthropicBeta extracts the latest tool_result
// payload from an Anthropic beta request and normalizes it into the shared
// request view used by Guardrails request-side evaluation.
func AdaptToolResultRequestFromAnthropicBeta(req *anthropic.BetaMessageNewParams) guardrailsevaluate.ToolResultRequestView {
	if req == nil {
		return guardrailsevaluate.ToolResultRequestView{}
	}

	text, blockCount, partCount := ExtractToolResultTextV1Beta(req.Messages)
	history := AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)

	return guardrailsevaluate.ToolResultRequestView{
		View: guardrailsevaluate.RequestView{
			Text:           text,
			MessageHistory: history,
		},
		HasToolResult: blockCount > 0,
		BlockCount:    blockCount,
		PartCount:     partCount,
	}
}

func responseTextFromAnthropicV1BetaBlocks(blocks []anthropic.BetaContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				parts = append(parts, block.Thinking)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func commandFromAnthropicV1BetaBlocks(blocks []anthropic.BetaContentBlockUnion) *guardrailscore.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrailscore.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}
