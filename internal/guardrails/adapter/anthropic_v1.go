package adapter

import (
	"encoding/json"

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
