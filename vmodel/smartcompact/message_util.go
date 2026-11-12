package smartcompact

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
)

// lastUserMessageContainsCompact checks if the last user message contains "compact" (case-insensitive).
func lastUserMessageContainsCompact(messages []anthropic.MessageParam) bool {
	// Find the last user message
	var lastUserMsg anthropic.MessageParam
	for i := len(messages) - 1; i >= 0; i-- {
		if msgparam.Role(messages[i]) == "user" {
			lastUserMsg = messages[i]
			break
		}
	}

	// Extract text content and check for "compact" (case-insensitive)
	var textContent strings.Builder
	for _, block := range msgparam.Content(lastUserMsg) {
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
		if msgparam.BetaRole(messages[i]) == "user" {
			lastUserMsg = messages[i]
			break
		}
	}

	// Extract text content and check for "compact" (case-insensitive)
	var textContent strings.Builder
	for _, block := range msgparam.BetaContent(lastUserMsg) {
		if block.OfText != nil {
			textContent.WriteString(block.OfText.Text)
		}
	}

	return strings.Contains(strings.ToLower(textContent.String()), "compact")
}
