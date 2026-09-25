package protocol

import (
	"encoding/json"
	"strings"
)

// ToolUseInput converts function-call arguments — the JSON string carried by
// OpenAI Chat, Responses and MCP tool calls — into the input object of an
// Anthropic tool_use block.
//
// Anthropic requires input to be a JSON object: a block without it is rejected
// with "tool_use.input: Field required", and "input": null is rejected as well.
// Blank or "null" arguments (a call to a tool without parameters) therefore
// become {}, as do malformed or non-object arguments; ok is false only in the
// latter case, so callers can log the lossy fallback.
func ToolUseInput(arguments string) (input map[string]any, ok bool) {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, true
	}
	if err := json.Unmarshal([]byte(trimmed), &input); err != nil || input == nil {
		return map[string]any{}, false
	}
	return input, true
}
