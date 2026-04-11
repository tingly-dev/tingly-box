package adapter

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
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

// RefreshInputFromAnthropicBetaResponse rebuilds the normalized response-side
// guardrails input from a fully assembled Anthropic beta response.
func RefreshInputFromAnthropicBetaResponse(input guardrailscore.Input, resp *anthropic.BetaMessage) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionResponse
	input.Content = guardrailscore.Content{
		Messages: input.Content.Messages,
	}
	if resp != nil {
		input.Content.Text = responseTextFromAnthropicV1BetaBlocks(resp.Content)
		input.Content.Command = commandFromAnthropicV1BetaBlocks(resp.Content)
	}
	return input
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
