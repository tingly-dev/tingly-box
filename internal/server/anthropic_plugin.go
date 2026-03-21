package server

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// cleanSystemMessages removes billing header messages from system blocks
// This is used for Claude Code scenario to filter out injected billing headers
func cleanSystemMessages(blocks []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}

// cleanBetaSystemMessages removes billing header messages from beta system blocks
func cleanBetaSystemMessages(blocks []anthropic.BetaTextBlockParam) []anthropic.BetaTextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.BetaTextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}
