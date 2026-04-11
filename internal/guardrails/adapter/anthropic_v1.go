package adapter

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// AdaptMessagesFromAnthropicV1 converts Anthropic v1 request history into the
// shared guardrails message format used as evaluation context.
func AdaptMessagesFromAnthropicV1(system []anthropic.TextBlockParam, messages []anthropic.MessageParam) []guardrailscore.Message {
	out := make([]guardrailscore.Message, 0, len(messages)+1)

	if len(system) > 0 {
		out = append(out, guardrailscore.Message{
			Role:    "system",
			Content: request.ConvertTextBlocksToString(system),
		})
	}

	for _, msg := range messages {
		out = append(out, guardrailscore.Message{
			Role:    string(msg.Role),
			Content: request.ConvertContentBlocksToString(msg.Content),
		})
	}

	return out
}

// RefreshInputFromAnthropicV1Response rebuilds the normalized response-side
// guardrails input from a fully assembled Anthropic v1 response.
func RefreshInputFromAnthropicV1Response(input guardrailscore.Input, resp *anthropic.Message) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionResponse
	input.Content = guardrailscore.Content{
		Messages: input.Content.Messages,
	}
	if resp != nil {
		input.Content.Text = responseTextFromAnthropicV1Blocks(resp.Content)
		input.Content.Command = commandFromAnthropicV1Blocks(resp.Content)
	}
	return input
}

func responseTextFromAnthropicV1Blocks(blocks []anthropic.ContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if (block.Type == "text" || block.Type == "thinking") && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func commandFromAnthropicV1Blocks(blocks []anthropic.ContentBlockUnion) *guardrailscore.Command {
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

func parseAnthropicInput(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return parsed
	}
	return map[string]interface{}{"_raw": string(raw)}
}
