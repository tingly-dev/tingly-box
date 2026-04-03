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

func FilterMessages(messages []guardrailscore.Message) []guardrailscore.Message {
	if len(messages) == 0 {
		return messages
	}
	filtered := make([]guardrailscore.Message, 0, len(messages))
	for _, msg := range messages {
		if strings.HasPrefix(msg.Content, BlockPrefix) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
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

func TextResponse(model, message string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "guardrails_blocked",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{{
			"type": "text",
			"text": message,
		}},
		"model":         model,
		"stop_reason":   "guardrails",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	}
}
