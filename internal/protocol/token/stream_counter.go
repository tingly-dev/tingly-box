package token

import (
	"fmt"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/tiktoken-go/tokenizer"
)

// StreamTokenCounter maintains token count state for streaming responses.
// It uses incremental counting strategy, tokenizing each delta immediately.
//
// Usage example:
//
//	counter, _ := NewStreamTokenCounter()
//	counter.SetInputTokens(estimateRequestTokens(req)) // Pre-count input
//	for chunk := range stream {
//	    counter.ConsumeOpenAIChunk(chunk)
//	}
//	input, output := counter.GetCounts()
type StreamTokenCounter struct {
	mu                   sync.Mutex
	encoder              tokenizer.Codec
	inputTokens          int
	outputTokens         int
	upstreamInputTokens  int64
	upstreamOutputTokens int64
	upstreamCacheTokens  int64 // prompt_tokens_details.cached_tokens
	upstreamReasoning    int64 // completion_tokens_details.reasoning_tokens
}

// NewStreamTokenCounter creates a new streaming token counter.
func NewStreamTokenCounter() (*StreamTokenCounter, error) {
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer: %w", err)
	}
	return &StreamTokenCounter{
		encoder: enc,
	}, nil
}

// NewStreamTokenCounterWithEncoding creates a new streaming token counter with a specific encoding.
func NewStreamTokenCounterWithEncoding(encoding string) (*StreamTokenCounter, error) {
	enc, err := tokenizer.Get(tokenizer.Encoding(encoding))
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer %s: %w", encoding, err)
	}
	return &StreamTokenCounter{
		encoder: enc,
	}, nil
}

// countTokens counts tokens for a text string using incremental strategy.
func (c *StreamTokenCounter) countTokens(text string) int {
	if text == "" {
		return 0
	}
	count, err := c.encoder.Count(text)
	if err != nil {
		// Fallback to character/4 estimate
		return len(text) / 4
	}
	return count
}

// ConsumeOpenAIChunk processes an OpenAI streaming chunk and updates token counts.
// Returns the current (inputTokens, outputTokens) after processing this chunk.
//
// The function handles:
//   - Content deltas (text responses)
//   - Refusal deltas (refusal messages)
//   - Tool call deltas (function names and arguments)
//   - Usage information (typically in the final chunk when stream_options.include_usage is set)
func (c *StreamTokenCounter) ConsumeOpenAIChunk(chunk *openai.ChatCompletionChunk) (inputTokens, outputTokens int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Process usage if provided in the chunk (usually only the trailing
	// usage-only chunk when stream_options.include_usage=true). The SDK's
	// Valid() check misses some legitimate cases, so we also accept any
	// non-zero prompt/completion count as evidence that usage is present.
	if chunk.JSON.Usage.Valid() || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		usage := chunk.Usage
		if usage.PromptTokens > 0 {
			c.inputTokens = int(usage.PromptTokens)
		}
		if usage.CompletionTokens > 0 {
			c.outputTokens = int(usage.CompletionTokens)
		}
		if chunk.Usage.PromptTokens > 0 {
			c.upstreamInputTokens = chunk.Usage.PromptTokens
		}
		if chunk.Usage.CompletionTokens > 0 {
			c.upstreamOutputTokens = chunk.Usage.CompletionTokens
		}
		if chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
			c.upstreamCacheTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if chunk.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			c.upstreamReasoning = chunk.Usage.CompletionTokensDetails.ReasoningTokens
		}
		return int(c.upstreamInputTokens - c.upstreamCacheTokens), int(c.upstreamOutputTokens), nil
	}

	// Incremental counting for each delta in choices
	for _, choice := range chunk.Choices {
		// Count content delta
		if choice.Delta.Content != "" {
			c.outputTokens += c.countTokens(choice.Delta.Content)
		}

		// Count refusal delta
		if choice.Delta.Refusal != "" {
			c.outputTokens += c.countTokens(choice.Delta.Refusal)
		}

		// Count tool call deltas
		for _, toolCall := range choice.Delta.ToolCalls {
			// Count tool call ID
			if toolCall.ID != "" {
				c.outputTokens += c.countTokens(toolCall.ID)
			}
			// Count function name
			if toolCall.Function.Name != "" {
				c.outputTokens += c.countTokens(toolCall.Function.Name)
			}
			// Count function arguments (partial JSON string)
			if toolCall.Function.Arguments != "" {
				c.outputTokens += c.countTokens(toolCall.Function.Arguments)
			}
		}

		// Count deprecated function call
		if choice.Delta.FunctionCall.Name != "" {
			c.outputTokens += c.countTokens(choice.Delta.FunctionCall.Name)
		}
		if choice.Delta.FunctionCall.Arguments != "" {
			c.outputTokens += c.countTokens(choice.Delta.FunctionCall.Arguments)
		}
	}

	return c.inputTokens, c.outputTokens, nil
}

// GetCounts returns the current token counts (inputTokens, outputTokens).
func (c *StreamTokenCounter) GetCounts() (inputTokens, outputTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	i, o := c.inputTokens, c.outputTokens
	if c.upstreamInputTokens > 0 {
		// Normalize: subtract cache so inputTokens = uncached-only portion
		i = int(c.upstreamInputTokens - c.upstreamCacheTokens)
	}
	if c.upstreamOutputTokens > 0 {
		o = int(c.upstreamOutputTokens)
	}
	return i, o
}

// GetUpstreamDetails returns cache and reasoning token counts harvested
// from upstream usage chunks (zero if upstream did not advertise them).
func (c *StreamTokenCounter) GetUpstreamDetails() (cacheTokens, reasoningTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(c.upstreamCacheTokens), int(c.upstreamReasoning)
}

// SetInputTokens sets the input token count.
// Use this when you have the exact count from the request.
func (c *StreamTokenCounter) SetInputTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inputTokens = tokens
}

// SetOutputTokens sets the output token count.
func (c *StreamTokenCounter) SetOutputTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputTokens = tokens
}

// AddToOutputTokens adds to the output token count.
func (c *StreamTokenCounter) AddToOutputTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputTokens += tokens
}

// Reset resets the counter to zero.
func (c *StreamTokenCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inputTokens = 0
	c.outputTokens = 0
}

// TotalTokens returns the sum of input and output tokens.
func (c *StreamTokenCounter) TotalTokens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inputTokens + c.outputTokens
}

// EstimateInputTokens estimates input tokens from a request using the counter's encoder.
// This is useful to pre-set the input token count before streaming.
func (c *StreamTokenCounter) EstimateInputTokens(req *openai.ChatCompletionNewParams) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tokens, err := EstimateInputTokens(req)
	if err != nil {
		return 0, err
	}
	c.inputTokens = tokens
	return tokens, nil
}

// CountText counts tokens for any text string.
func (c *StreamTokenCounter) CountText(text string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.countTokens(text)
}

// AddInputTokens adds to the input token count.
func (c *StreamTokenCounter) AddInputTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inputTokens += tokens
}

// InputTokens returns the current input token count.
func (c *StreamTokenCounter) InputTokens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inputTokens
}

// OutputTokens returns the current output token count.
func (c *StreamTokenCounter) OutputTokens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputTokens
}
