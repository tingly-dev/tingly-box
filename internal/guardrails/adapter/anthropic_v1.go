package adapter

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
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

// ResponseViewFromAnthropicV1Response adapts a non-stream Anthropic v1 response
// into the shared response view.
func ResponseViewFromAnthropicV1Response(messageHistory []guardrailscore.Message, resp *anthropic.Message) guardrailsevaluate.ResponseView {
	if resp == nil {
		return guardrailsevaluate.ResponseView{MessageHistory: messageHistory}
	}
	return guardrailsevaluate.ResponseView{
		Text:           responseTextFromAnthropicV1Blocks(resp.Content),
		Command:        commandFromAnthropicV1Blocks(resp.Content),
		MessageHistory: messageHistory,
	}
}

// AdaptToolResultRequestFromAnthropicV1 extracts the latest tool_result payload
// from an Anthropic v1 request and normalizes it into the shared request view
// used by Guardrails request-side evaluation.
func AdaptToolResultRequestFromAnthropicV1(req *anthropic.MessageNewParams) guardrailsevaluate.ToolResultRequestView {
	if req == nil {
		return guardrailsevaluate.ToolResultRequestView{}
	}

	text, blockCount, partCount := ExtractToolResultTextV1(req.Messages)
	history := AdaptMessagesFromAnthropicV1(req.System, req.Messages)

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
