package token

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tiktoken-go/tokenizer"
)

// EstimateInputTokens estimates input tokens from OpenAI request using tiktoken
func EstimateInputTokens(req *openai.ChatCompletionNewParams) (int, error) {
	// Get the encoding for the model (default to O200kBase which is used by GPT-4o and above)
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Helper function to count tokens with fallback to character/4 estimate
	countOrEstimate := func(text string) int {
		c, err := enc.Count(text)
		if err != nil {
			return len(text) / 4
		}
		return c
	}

	totalTokens := 0

	// Count tokens in messages
	for _, msg := range req.Messages {
		// Count role
		if role := msg.GetRole(); role != nil {
			totalTokens += countOrEstimate(*role)
		}

		// Count content
		content := msg.GetContent()
		switch content.AsAny().(type) {
		case *string:
			if s := content.AsAny().(*string); s != nil {
				totalTokens += countOrEstimate(*s)
			}
		case *[]openai.ChatCompletionContentPartTextParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartTextParam); parts != nil {
				for _, part := range *parts {
					totalTokens += countOrEstimate(part.Text)
				}
			}
		case *[]openai.ChatCompletionContentPartUnionParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartUnionParam); parts != nil {
				for _, part := range *parts {
					if part.OfText != nil {
						totalTokens += countOrEstimate(part.OfText.Text)
					}
				}
			}
		}
	}

	// Add some overhead for the request format
	totalTokens += 3

	return totalTokens, nil
}

// EstimateOutputTokens estimates output tokens from accumulated content
func EstimateOutputTokens(content string) int {
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return len(content) / 4
	}
	count, err := enc.Count(content)
	if err != nil {
		return len(content) / 4
	}
	return count
}

// CountTokensViaTiktoken approximates token count for OpenAI-style providers using tiktoken
func CountTokensViaTiktoken(model string, messages []anthropic.MessageParam, system anthropic.MessageCountTokensParamsSystemUnion) (int, error) {
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Helper function to count tokens with fallback to character/4 estimate
	countOrEstimate := func(text string) int {
		c, err := enc.Count(text)
		if err != nil {
			return len(text) / 4
		}
		return c
	}

	totalTokens := 0

	// Count tokens in system messages
	for _, sys := range system.OfTextBlockArray {
		totalTokens += countOrEstimate(sys.Text)
	}
	if system.OfString.Valid() {
		totalTokens += countOrEstimate(system.OfString.String())
	}

	// Count tokens in regular messages
	for _, msg := range messages {
		totalTokens += countOrEstimate(string(msg.Role))

		// Count content blocks
		for _, block := range msg.Content {
			if block.OfText != nil {
				totalTokens += countOrEstimate(block.OfText.Text)
			}
			if block.OfThinking != nil {
				totalTokens += countOrEstimate(block.OfThinking.Thinking)
			}
			// Note: We're not counting image tokens here as they require special handling
			// This is an approximation for text-only requests
		}
	}

	// Add some overhead for the request format (approximately)
	totalTokens += 3

	return totalTokens, nil
}

// CountBetaTokensViaTiktoken approximates token count for OpenAI-style providers using tiktoken
func CountBetaTokensViaTiktoken(model string, messages []anthropic.BetaMessageParam, system anthropic.BetaMessageCountTokensParamsSystemUnion) (int, error) {
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Helper function to count tokens with fallback to character/4 estimate
	countOrEstimate := func(text string) int {
		c, err := enc.Count(text)
		if err != nil {
			return len(text) / 4
		}
		return c
	}

	totalTokens := 0

	// Count tokens in system messages
	if ok := system.OfString.Valid(); ok {
		totalTokens += countOrEstimate(system.OfString.String())
	}
	for _, sys := range system.OfBetaTextBlockArray {
		totalTokens += countOrEstimate(sys.Text)
	}

	// Count tokens in regular messages
	for _, msg := range messages {
		totalTokens += countOrEstimate(string(msg.Role))

		// Count content blocks
		for _, block := range msg.Content {
			if block.OfText != nil {
				totalTokens += countOrEstimate(block.OfText.Text)
			}
			if block.OfThinking != nil {
				totalTokens += countOrEstimate(block.OfThinking.Thinking)
			}
			// Note: We're not counting image tokens here as they require special handling
			// This is an approximation for text-only requests
		}
	}

	// Add some overhead for the request format (approximately)
	totalTokens += 3

	return totalTokens, nil
}
