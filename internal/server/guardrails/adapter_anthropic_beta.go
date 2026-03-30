package serverguardrails

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// AdaptMessagesFromAnthropicV1Beta converts Anthropic beta request history into
// the shared guardrails message format used as evaluation context.
func AdaptMessagesFromAnthropicV1Beta(system []anthropic.BetaTextBlockParam, messages []anthropic.BetaMessageParam) []guardrails.Message {
	out := make([]guardrails.Message, 0, len(messages)+1)

	if len(system) > 0 {
		out = append(out, guardrails.Message{
			Role:    "system",
			Content: request.ConvertBetaTextBlocksToString(system),
		})
	}

	for _, msg := range messages {
		out = append(out, guardrails.Message{
			Role:    string(msg.Role),
			Content: request.ConvertBetaContentBlocksToString(msg.Content),
		})
	}

	return out
}

// ResponseViewFromAnthropicV1BetaResponse adapts a non-stream Anthropic beta
// response into the shared response view.
func ResponseViewFromAnthropicV1BetaResponse(messageHistory []guardrails.Message, resp *anthropic.BetaMessage) ResponseView {
	if resp == nil {
		return ResponseView{MessageHistory: messageHistory}
	}
	return ResponseView{
		Text:           responseTextFromAnthropicV1BetaBlocks(resp.Content),
		Command:        commandFromAnthropicV1BetaBlocks(resp.Content),
		MessageHistory: messageHistory,
	}
}

// AdaptToolResultRequestFromAnthropicBeta extracts the latest tool_result
// payload from an Anthropic beta request and normalizes it into the shared
// request view used by Guardrails request-side evaluation.
func AdaptToolResultRequestFromAnthropicBeta(req *anthropic.BetaMessageNewParams) ToolResultRequestView {
	if req == nil {
		return ToolResultRequestView{}
	}

	text, blockCount, partCount := ExtractToolResultTextV1Beta(req.Messages)
	history := AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)

	return ToolResultRequestView{
		View: RequestView{
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

func commandFromAnthropicV1BetaBlocks(blocks []anthropic.BetaContentBlockUnion) *guardrails.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrails.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}
