package server

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tiktoken-go/tokenizer"
)

// countTokensWithTiktoken approximates token count for OpenAI-style providers using tiktoken
func countTokensWithTiktoken(model string, messages []anthropic.MessageParam, system []anthropic.TextBlockParam) (int, error) {
	// Get the encoding for the model (default to O200kBase which is used by GPT-4o and above)
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	totalTokens := 0

	// Count tokens in system messages
	for _, sys := range system {
		count, err := enc.Count(sys.Text)
		if err != nil {
			// If counting fails, estimate with character count / 4
			count = len(sys.Text) / 4
		}
		totalTokens += count
	}

	// Count tokens in regular messages
	for _, msg := range messages {
		// Count role
		count, err := enc.Count(string(msg.Role))
		if err != nil {
			count = len(string(msg.Role)) / 4
		}
		totalTokens += count

		// Count content blocks
		for _, block := range msg.Content {
			if block.OfText != nil {
				count, err := enc.Count(block.OfText.Text)
				if err != nil {
					// If counting fails, estimate with character count / 4
					count = len(block.OfText.Text) / 4
				}
				totalTokens += count
			}
			// Note: We're not counting image tokens here as they require special handling
			// This is an approximation for text-only requests
		}
	}

	// Add some overhead for the request format (approximately)
	// This accounts for the JSON structure and special tokens
	totalTokens += 3 // Add a small buffer for format overhead

	return totalTokens, nil
}
