package stream

import (
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// anthropicToOpenAIConverter is a stateful iterator that reads Anthropic Beta stream
// events and emits OpenAI Chat Completion wire chunks.
//
// Chunks are built with wire DTOs (omitempty outbound shapes) rather than the
// openai-go SDK response structs: the SDK types marshal unset fields as zero
// values ("role":"", "finish_reason":"", zero usage on every chunk), which
// strict clients reject.
type anthropicToOpenAIConverter struct {
	stream             *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	responseModel      string
	disableStreamUsage bool
	hooks              *AnthropicToOpenAIMCPHooks

	// state
	chatID           string
	created          int64
	acc              *usagepkg.AnthropicAccumulator
	toolCallID       string
	toolCallName     string
	toolCallArgs     strings.Builder
	hasToolCalls     bool
	suppressToolCall bool
	pendingToolCalls []AnthropicToOpenAIToolCall
	thinkingText     strings.Builder

	// pending event queue
	pending []interface{}
	hookErr error
	started bool
	done    bool
}

func newAnthropicToOpenAIConverter(
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	responseModel string,
	disableStreamUsage bool,
	hooks *AnthropicToOpenAIMCPHooks,
) *anthropicToOpenAIConverter {
	return &anthropicToOpenAIConverter{
		stream:             stream,
		responseModel:      responseModel,
		disableStreamUsage: disableStreamUsage,
		hooks:              hooks,
		chatID:             fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		created:            time.Now().Unix(),
		acc:                usagepkg.NewAnthropicAccumulator(),
	}
}

func (c *anthropicToOpenAIConverter) Next() (interface{}, bool, error) {
	// Drain buffered events first
	if len(c.pending) > 0 {
		evt := c.pending[0]
		c.pending = c.pending[1:]
		return evt, false, nil
	}

	if c.done {
		return nil, true, nil
	}

	for {
		if !c.stream.Next() {
			if err := c.stream.Err(); err != nil {
				return nil, false, err
			}
			if c.started && !c.done {
				return nil, false, fmt.Errorf("anthropic stream ended without message_stop")
			}
			return nil, true, nil
		}
		event := c.stream.Current()
		c.processEvent(&event)

		if len(c.pending) > 0 {
			evt := c.pending[0]
			c.pending = c.pending[1:]
			return evt, false, nil
		}
		if c.done {
			return nil, true, nil
		}
	}
}

func (c *anthropicToOpenAIConverter) Usage() *protocol.TokenUsage {
	return c.acc.Result()
}

// HookErr returns the MCP hook error, if any (e.g. ErrMCPStreamContinue).
func (c *anthropicToOpenAIConverter) HookErr() error {
	return c.hookErr
}

func (c *anthropicToOpenAIConverter) processEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	switch event.Type {
	case "message_start":
		c.started = true
		c.emitChunk(wire.ChatStreamDelta{Role: "assistant"}, nil)
		c.acc.ConsumeBeta(event)

	case "content_block_start":
		if event.ContentBlock.Type == "text" {
			// no chunk to emit; text deltas come via content_block_delta
		} else if event.ContentBlock.Type == "tool_use" {
			c.toolCallID = event.ContentBlock.ID
			c.toolCallName = event.ContentBlock.Name
			c.toolCallArgs.Reset()
			c.suppressToolCall = c.hooks != nil && c.hooks.ShouldSuppressTool != nil && c.hooks.ShouldSuppressTool(c.toolCallName)
			if !c.suppressToolCall {
				c.hasToolCalls = true
				emptyArgs := ""
				c.emitChunk(wire.ChatStreamDelta{
					ToolCalls: []wire.ChatStreamToolCall{
						{
							Index: 0,
							ID:    c.toolCallID,
							Type:  "function",
							Function: wire.ChatStreamToolFunction{
								Name:      c.toolCallName,
								Arguments: &emptyArgs,
							},
						},
					},
				}, nil)
			}
		} else if event.ContentBlock.Type == "thinking" {
			c.thinkingText.Reset()
		} else if event.ContentBlock.Type == "redacted_thinking" {
			c.thinkingText.Reset()
			c.thinkingText.WriteString("[REDACTED THINKING]")
		}

	case "content_block_delta":
		if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			c.emitChunk(wire.ChatStreamDelta{Content: event.Delta.Text}, nil)
		} else if event.Delta.Type == "input_json_delta" && event.Delta.PartialJSON != "" {
			args := event.Delta.PartialJSON
			c.toolCallArgs.WriteString(args)
			if !c.suppressToolCall {
				c.emitChunk(wire.ChatStreamDelta{
					ToolCalls: []wire.ChatStreamToolCall{
						{
							Index:    0,
							Function: wire.ChatStreamToolFunction{Arguments: &args},
						},
					},
				}, nil)
			}
		} else if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
			thinking := event.Delta.Thinking
			c.thinkingText.WriteString(thinking)
			c.emitChunk(wire.ChatStreamDelta{ReasoningContent: thinking}, nil)
		}
		// signature_delta is intentionally ignored

	case "content_block_stop":
		if c.toolCallID != "" {
			c.pendingToolCalls = append(c.pendingToolCalls, AnthropicToOpenAIToolCall{
				ID:        c.toolCallID,
				Name:      c.toolCallName,
				Arguments: c.toolCallArgs.String(),
			})
			c.toolCallID = ""
			c.toolCallName = ""
			c.toolCallArgs.Reset()
			c.suppressToolCall = false
		}

	case "message_delta":
		c.acc.ConsumeBeta(event)

	case "message_stop":
		if c.hooks != nil && c.hooks.OnToolCallsFinal != nil && len(c.pendingToolCalls) > 0 {
			if err := c.hooks.OnToolCallsFinal(c.pendingToolCalls); err != nil {
				c.hookErr = err
				c.done = true
				return
			}
		}
		finishReason := "stop"
		if c.hasToolCalls {
			finishReason = openaiFinishReasonToolCalls
		}
		chunk := c.newChunk(wire.ChatStreamDelta{}, &finishReason)
		if !c.disableStreamUsage && c.acc.HasUsage() {
			chunk.Usage = chatStreamUsageWire(c.acc.Result())
		}
		c.pending = append(c.pending, chunk)
		c.done = true
	}
}

// emitChunk appends a wire chunk with the given delta to the pending queue.
func (c *anthropicToOpenAIConverter) emitChunk(delta wire.ChatStreamDelta, finishReason *string) {
	c.pending = append(c.pending, c.newChunk(delta, finishReason))
}

func (c *anthropicToOpenAIConverter) newChunk(delta wire.ChatStreamDelta, finishReason *string) wire.ChatStreamChunk {
	return wire.ChatStreamChunk{
		ID:      c.chatID,
		Object:  "chat.completion.chunk",
		Created: c.created,
		Model:   c.responseModel,
		Choices: []wire.ChatStreamChoice{
			{Index: 0, Delta: delta, FinishReason: finishReason},
		},
	}
}

// chatStreamUsageWire converts normalized TokenUsage into the Chat Completions
// stream usage wire shape. Same semantics as usagepkg.ChatUsage: prompt_tokens
// is the TOTAL (uncached + cached), cached_tokens a reported subset.
func chatStreamUsageWire(u *protocol.TokenUsage) *wire.ChatStreamUsage {
	totalInput := u.InputTokens + u.CacheInputTokens
	su := &wire.ChatStreamUsage{
		PromptTokens:     int64(totalInput),
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      int64(totalInput + u.OutputTokens),
	}
	if u.CacheInputTokens > 0 {
		su.PromptTokensDetails = &wire.ChatStreamPromptTokenDetails{CachedTokens: int64(u.CacheInputTokens)}
	}
	if u.ReasoningTokens > 0 {
		su.CompletionTokensDetails = &wire.ChatStreamOutputTokenDetails{ReasoningTokens: int64(u.ReasoningTokens)}
	}
	return su
}
