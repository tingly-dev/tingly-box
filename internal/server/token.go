package server

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tiktoken-go/tokenizer"
)

// estimateInputTokens estimates input tokens from OpenAI request using tiktoken
func estimateInputTokens(req *openai.ChatCompletionNewParams) (int, error) {
	// Get the encoding for the model (default to O200kBase which is used by GPT-4o and above)
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	totalTokens := 0

	// Count tokens in messages
	for _, msg := range req.Messages {
		// Count role
		role := msg.GetRole()
		if role != nil {
			count, err := enc.Count(*role)
			if err != nil {
				count = len(*role) / 4
			}
			totalTokens += count
		}

		// Count content
		content := msg.GetContent()
		switch content.AsAny().(type) {
		case *string:
			if s := content.AsAny().(*string); s != nil {
				count, err := enc.Count(*s)
				if err != nil {
					count = len(*s) / 4
				}
				totalTokens += count
			}
		case *[]openai.ChatCompletionContentPartTextParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartTextParam); parts != nil {
				for _, part := range *parts {
					count, err := enc.Count(part.Text)
					if err != nil {
						count = len(part.Text) / 4
					}
					totalTokens += count
				}
			}
		case *[]openai.ChatCompletionContentPartUnionParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartUnionParam); parts != nil {
				for _, part := range *parts {
					if part.OfText != nil {
						count, err := enc.Count(part.OfText.Text)
						if err != nil {
							count = len(part.OfText.Text) / 4
						}
						totalTokens += count
					}
				}
			}
		}
	}

	// Add some overhead for the request format
	totalTokens += 3

	return totalTokens, nil
}

// estimateOutputTokens estimates output tokens from accumulated content
func estimateOutputTokens(content string) int {
	// Get the encoding
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		// Fallback to character count / 4
		return len(content) / 4
	}

	count, err := enc.Count(content)
	if err != nil {
		// Fallback to character count / 4
		return len(content) / 4
	}

	return count
}

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

			if block.OfThinking != nil {
				count, err := enc.Count(block.OfThinking.Thinking)
				if err != nil {
					// If counting fails, estimate with character count / 4
					count = len(block.OfThinking.Thinking) / 4
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

// countBetaTokensWithTiktoken approximates token count for OpenAI-style providers using tiktoken
func countBetaTokensWithTiktoken(model string, messages []anthropic.BetaMessageParam, system anthropic.BetaMessageCountTokensParamsSystemUnion) (int, error) {
	// Get the encoding for the model (default to O200kBase which is used by GPT-4o and above)
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	totalTokens := 0

	// Count tokens in system messages
	if ok := system.OfString.Valid(); ok {
		s := system.OfString.String()
		count, err := enc.Count(s)
		if err != nil {
			// If counting fails, estimate with character count / 4
			count = len(s) / 4
		}
		totalTokens += count
	}

	for _, sys := range system.OfBetaTextBlockArray {
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

			if block.OfThinking != nil {
				count, err := enc.Count(block.OfThinking.Thinking)
				if err != nil {
					// If counting fails, estimate with character count / 4
					count = len(block.OfThinking.Thinking) / 4
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
