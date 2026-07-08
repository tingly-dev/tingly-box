package token

import (
	"fmt"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/tiktoken-go/tokenizer"
)

// StreamTokenCounter maintains token count state for streaming responses.
//
// Delta text is buffered and only tokenized when counts are actually read,
// and only if the upstream never reported authoritative usage — so streams
// where the provider sends a usage chunk (the common case) pay no BPE cost
// at all.
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
	mu           sync.Mutex
	encoder      tokenizer.Codec
	inputTokens  int
	outputTokens int
	// pendingOutput buffers streamed delta text that has not been tokenized
	// yet. It is flushed (tokenized in one pass) the first time counts are
	// read, and discarded outright when upstream usage arrives.
	pendingOutput        strings.Builder
	upstreamInputTokens  int64
	upstreamOutputTokens int64
	upstreamCacheTokens  int64 // prompt_tokens_details.cached_tokens
	upstreamReasoning    int64 // completion_tokens_details.reasoning_tokens
}

// NewStreamTokenCounter creates a new streaming token counter.
func NewStreamTokenCounter() (*StreamTokenCounter, error) {
	enc, err := getCodec(tokenizer.O200kBase)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer: %w", err)
	}
	return &StreamTokenCounter{
		encoder: enc,
	}, nil
}

// NewStreamTokenCounterWithEncoding creates a new streaming token counter with a specific encoding.
func NewStreamTokenCounterWithEncoding(encoding string) (*StreamTokenCounter, error) {
	enc, err := getCodec(tokenizer.Encoding(encoding))
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer %s: %w", encoding, err)
	}
	return &StreamTokenCounter{
		encoder: enc,
	}, nil
}

// countTokens counts tokens for a text string.
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

// flushPendingLocked tokenizes buffered delta text into outputTokens.
// When upstream usage has been seen the local estimate is dead weight, so the
// buffer is discarded without tokenizing. Callers must hold c.mu.
func (c *StreamTokenCounter) flushPendingLocked() {
	if c.pendingOutput.Len() == 0 {
		return
	}
	if c.upstreamOutputTokens == 0 {
		c.outputTokens += c.countTokens(c.pendingOutput.String())
	}
	c.pendingOutput.Reset()
}

// ConsumeOpenAIChunk processes an OpenAI streaming chunk and updates token counts.
// Delta text is buffered, not tokenized inline; read GetCounts for totals.
//
// The function handles:
//   - Content deltas (text responses)
//   - Refusal deltas (refusal messages)
//   - Tool call deltas (function names and arguments)
//   - Usage information (typically in the final chunk when stream_options.include_usage is set)
func (c *StreamTokenCounter) ConsumeOpenAIChunk(chunk *openai.ChatCompletionChunk) error {
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
		// Upstream counts supersede any buffered local estimate.
		if c.upstreamOutputTokens > 0 {
			c.pendingOutput.Reset()
		}
		return nil
	}

	// Upstream already reported authoritative output usage: buffering further
	// delta text would be wasted work.
	if c.upstreamOutputTokens > 0 {
		return nil
	}

	// Buffer each delta's text for lazy one-pass tokenization.
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			c.pendingOutput.WriteString(choice.Delta.Content)
		}
		if choice.Delta.Refusal != "" {
			c.pendingOutput.WriteString(choice.Delta.Refusal)
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			if toolCall.ID != "" {
				c.pendingOutput.WriteString(toolCall.ID)
			}
			if toolCall.Function.Name != "" {
				c.pendingOutput.WriteString(toolCall.Function.Name)
			}
			if toolCall.Function.Arguments != "" {
				c.pendingOutput.WriteString(toolCall.Function.Arguments)
			}
		}
		if choice.Delta.FunctionCall.Name != "" {
			c.pendingOutput.WriteString(choice.Delta.FunctionCall.Name)
		}
		if choice.Delta.FunctionCall.Arguments != "" {
			c.pendingOutput.WriteString(choice.Delta.FunctionCall.Arguments)
		}
	}

	return nil
}

// GetCounts returns the current token counts (inputTokens, outputTokens).
func (c *StreamTokenCounter) GetCounts() (inputTokens, outputTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushPendingLocked()

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

// SetOutputTokens sets the output token count, discarding any buffered
// not-yet-counted delta text.
func (c *StreamTokenCounter) SetOutputTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingOutput.Reset()
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
	c.pendingOutput.Reset()
}

// TotalTokens returns the sum of input and output tokens.
func (c *StreamTokenCounter) TotalTokens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushPendingLocked()
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
	c.flushPendingLocked()
	return c.outputTokens
}
