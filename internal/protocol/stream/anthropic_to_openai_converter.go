package stream

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// anthropicToOpenAIConverter is a stateful iterator that reads Anthropic Beta stream
// events and emits OpenAI Chat Completion chunk maps.
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
		chunk := openai.ChatCompletionChunk{
			ID:      c.chatID,
			Created: c.created,
			Model:   c.responseModel,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Role: "assistant",
					},
				},
			},
		}
		c.emitChunk(chunk)
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
				chunk := openai.ChatCompletionChunk{
					ID:      c.chatID,
					Created: c.created,
					Model:   c.responseModel,
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionChunkChoiceDelta{
								Role: "assistant",
								ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
									{
										Index: 0,
										ID:    c.toolCallID,
										Type:  "function",
										Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
											Name:      c.toolCallName,
											Arguments: "",
										},
									},
								},
							},
						},
					},
				}
				c.emitChunk(chunk)
			}
		} else if event.ContentBlock.Type == "thinking" {
			c.thinkingText.Reset()
		} else if event.ContentBlock.Type == "redacted_thinking" {
			c.thinkingText.Reset()
			c.thinkingText.WriteString("[REDACTED THINKING]")
		}

	case "content_block_delta":
		if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			text := event.Delta.Text
			chunk := openai.ChatCompletionChunk{
				ID:      c.chatID,
				Created: c.created,
				Model:   c.responseModel,
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionChunkChoiceDelta{
							Role:    "assistant",
							Content: text,
						},
					},
				},
			}
			c.emitChunk(chunk)
		} else if event.Delta.Type == "input_json_delta" && event.Delta.PartialJSON != "" {
			args := event.Delta.PartialJSON
			c.toolCallArgs.WriteString(args)
			if !c.suppressToolCall {
				chunk := openai.ChatCompletionChunk{
					ID:      c.chatID,
					Created: c.created,
					Model:   c.responseModel,
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionChunkChoiceDelta{
								Role: "assistant",
								ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
									{
										Index: 0,
										Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
											Arguments: args,
										},
									},
								},
							},
						},
					},
				}
				c.emitChunk(chunk)
			}
		} else if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
			thinking := event.Delta.Thinking
			c.thinkingText.WriteString(thinking)
			chunk := createReasoningContentChunk(c.chatID, c.created, c.responseModel, thinking)
			c.emitChunk(chunk)
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
		if errors.Is(c.hookErr, ErrMCPStreamContinue) {
			c.done = true
			return
		}

		finishReason := "stop"
		if c.hasToolCalls {
			finishReason = openaiFinishReasonToolCalls
		}
		delta := openai.ChatCompletionChunkChoiceDelta{}
		if c.hasToolCalls {
			delta.Content = ""
		}
		chunk := openai.ChatCompletionChunk{
			ID:      c.chatID,
			Created: c.created,
			Model:   c.responseModel,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index:        0,
					Delta:        delta,
					FinishReason: finishReason,
				},
			},
		}
		if !c.disableStreamUsage && c.acc.HasUsage() {
			chunk.Usage = usagepkg.ChatUsage(c.acc.Result())
		}
		c.emitChunk(chunk)
		c.done = true
	}
}

// emitChunk converts a ChatCompletionChunk to a map and appends to pending.
func (c *anthropicToOpenAIConverter) emitChunk(chunk openai.ChatCompletionChunk) {
	m, err := chunkToMap(chunk)
	if err != nil {
		return
	}
	if c.disableStreamUsage {
		delete(m, "usage")
	}
	c.pending = append(c.pending, m)
}
