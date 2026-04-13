package adapter

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// BlockPrefix marks tool_result content already replaced by guardrails.
const BlockPrefix = "[Tingly-box] Blocked by guardrails."

func ExtractToolResultTextV1(messages []anthropic.MessageParam) (string, int, int) {
	for i := len(messages) - 1; i >= 0; i-- {
		text, blocks, parts := extractToolResultTextV1FromMessage(messages[i])
		if blocks > 0 {
			return text, blocks, parts
		}
	}
	return "", 0, 0
}

func extractToolResultTextV1FromMessage(msg anthropic.MessageParam) (string, int, int) {
	var b strings.Builder
	var blocks int
	var parts int
	for _, block := range msg.Content {
		if block.OfToolResult == nil {
			continue
		}
		blocks++
		for _, content := range block.OfToolResult.Content {
			parts++
			if content.OfText != nil {
				b.WriteString(content.OfText.Text)
				continue
			}
			if raw, err := json.Marshal(content); err == nil {
				b.WriteString(string(raw))
			}
		}
	}
	return b.String(), blocks, parts
}

func ExtractToolResultTextV1Beta(messages []anthropic.BetaMessageParam) (string, int, int) {
	for i := len(messages) - 1; i >= 0; i-- {
		text, blocks, parts := extractToolResultTextV1BetaFromMessage(messages[i])
		if blocks > 0 {
			return text, blocks, parts
		}
	}
	return "", 0, 0
}

func extractToolResultTextV1BetaFromMessage(msg anthropic.BetaMessageParam) (string, int, int) {
	var b strings.Builder
	var blocks int
	var parts int
	for _, block := range msg.Content {
		if block.OfToolResult == nil {
			continue
		}
		blocks++
		for _, content := range block.OfToolResult.Content {
			parts++
			if content.OfText != nil {
				b.WriteString(content.OfText.Text)
				continue
			}
			if raw, err := json.Marshal(content); err == nil {
				b.WriteString(string(raw))
			}
		}
	}
	return b.String(), blocks, parts
}

func ReplaceToolResultContentV1(messages []anthropic.MessageParam, message string) {
	for i := range messages {
		msg := &messages[i]
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolResult == nil {
				continue
			}
			block.OfToolResult.IsError = anthropic.Bool(true)
			block.OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{{
				OfText: &anthropic.TextBlockParam{Text: message},
			}}
		}
	}
}

func ReplaceToolResultContentV1Beta(messages []anthropic.BetaMessageParam, message string) {
	for i := range messages {
		msg := &messages[i]
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolResult == nil {
				continue
			}
			block.OfToolResult.IsError = anthropic.Bool(true)
			block.OfToolResult.Content = []anthropic.BetaToolResultBlockParamContentUnion{{
				OfText: &anthropic.BetaTextBlockParam{Text: message},
			}}
		}
	}
}

// parseToolArguments decodes a raw JSON argument string into a structured map.
// When decoding fails, the raw payload is preserved under `_raw`.
func parseToolArguments(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return map[string]interface{}{"_raw": raw}
}

// BuildCommand constructs a normalized command payload from already-parsed
// tool arguments.
func BuildCommand(name string, args map[string]interface{}) *guardrailscore.Command {
	if name == "" && len(args) == 0 {
		return nil
	}
	cmd := &guardrailscore.Command{
		Name:      name,
		Arguments: args,
	}
	cmd.AttachDerivedFields()
	return cmd
}

// BuildCommandFromRawArguments parses raw JSON-ish tool arguments and returns
// a derived command ready for evaluation/mutation use.
func BuildCommandFromRawArguments(name, raw string) *guardrailscore.Command {
	return BuildCommand(name, parseToolArguments(raw))
}
