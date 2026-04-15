package smart_compact

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// lastUserMessageContainsCompact checks if the last user message contains "compact" (case-insensitive).
func lastUserMessageContainsCompact(messages []anthropic.MessageParam) bool {
	// Find the last user message
	var lastUserMsg anthropic.MessageParam
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			lastUserMsg = messages[i]
			break
		}
	}

	// Extract text content and check for "compact" (case-insensitive)
	var textContent strings.Builder
	for _, block := range lastUserMsg.Content {
		if block.OfText != nil {
			textContent.WriteString(block.OfText.Text)
		}
	}

	return strings.Contains(strings.ToLower(textContent.String()), "compact")
}

// lastUserMessageContainsCompactBeta checks if the last user message contains "compact" (case-insensitive) for beta API.
func lastUserMessageContainsCompactBeta(messages []anthropic.BetaMessageParam) bool {
	// Find the last user message
	var lastUserMsg anthropic.BetaMessageParam
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			lastUserMsg = messages[i]
			break
		}
	}

	// Extract text content and check for "compact" (case-insensitive)
	var textContent strings.Builder
	for _, block := range lastUserMsg.Content {
		if block.OfText != nil {
			textContent.WriteString(block.OfText.Text)
		}
	}

	return strings.Contains(strings.ToLower(textContent.String()), "compact")
}
