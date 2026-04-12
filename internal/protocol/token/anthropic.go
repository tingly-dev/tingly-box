package token

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tiktoken-go/tokenizer"
)

func EstimateAnthropicTokens(messages []anthropic.MessageParam) int64 {
	var total int64
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil {
				total += EstimateTokensString(block.OfText.Text)
			}
		}
		total += 5
	}
	return total
}

func EstimateBetaAnthropicTokens(messages []anthropic.BetaMessageParam) int64 {
	var total int64
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil {
				total += EstimateTokensString(block.OfText.Text)
			}
		}
		total += 5
	}
	return total
}

// EstimateAnthropicBetaMessagesTokens estimates tokens for Anthropic messages
func EstimateAnthropicBetaMessagesTokens(messages []anthropic.BetaMessageParam) int64 {
	var total int64
	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.OfText != nil {
				total += EstimateTokensString(b.OfText.Text)
				total += 5
			}
		}
	}
	return total
}

// CountTokensViaTiktoken approximates token count for OpenAI-style providers using tiktoken
func CountTokensViaTiktoken(req *anthropic.MessageCountTokensParams) (int, error) {
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	messages := req.Messages
	system := req.System
	tools := req.Tools

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

	// Count tokens in tools
	for _, tool := range tools {
		toolJSON, err := json.Marshal(tool)
		if err != nil {
			// If serialization fails, estimate based on tool components
			if tool.OfTool != nil {
				totalTokens += countOrEstimate(tool.OfTool.Name)
				if tool.OfTool.Description.Valid() {
					totalTokens += countOrEstimate(tool.OfTool.Description.Value)
				}
				// InputSchema is complex, count its JSON representation
				if schemaJSON, err := json.Marshal(tool.OfTool.InputSchema); err == nil {
					totalTokens += countOrEstimate(string(schemaJSON))
				}
			}
		} else {
			totalTokens += countOrEstimate(string(toolJSON))
		}
	}

	// Add some overhead for the request format (approximately)
	totalTokens += 3

	return totalTokens, nil
}

// CountBetaTokensViaTiktoken approximates token count for OpenAI-style providers using tiktoken
func CountBetaTokensViaTiktoken(req *anthropic.BetaMessageCountTokensParams) (int, error) {

	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	messages := req.Messages
	system := req.System
	tools := req.Tools

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

	// Count tokens in tools
	for _, tool := range tools {
		toolJSON, err := json.Marshal(tool)
		if err != nil {
			// If serialization fails, estimate based on tool components
			if tool.OfTool != nil {
				totalTokens += countOrEstimate(tool.OfTool.Name)
				if tool.OfTool.Description.Valid() {
					totalTokens += countOrEstimate(tool.OfTool.Description.Value)
				}
				// InputSchema is complex, count its JSON representation
				if schemaJSON, err := json.Marshal(tool.OfTool.InputSchema); err == nil {
					totalTokens += countOrEstimate(string(schemaJSON))
				}
			}
		} else {
			totalTokens += countOrEstimate(string(toolJSON))
		}
	}

	// Add some overhead for the request format (approximately)
	totalTokens += 3

	return totalTokens, nil
}
