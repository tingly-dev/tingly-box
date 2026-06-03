package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

type AnthropicToOpenAIToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type AnthropicToOpenAIMCPHooks struct {
	ShouldSuppressTool func(name string) bool
	OnToolCallsFinal   func(calls []AnthropicToOpenAIToolCall) error
}

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

// AnthropicToOpenAIStream processes Anthropic streaming events and converts them to OpenAI format
// Returns inputTokens, outputTokens, and error for usage tracking
func AnthropicToOpenAIStream(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool) (int, int, error) {
	return AnthropicToOpenAIStreamWithMCPHooks(hc, req, stream, responseModel, disableStreamUsage, nil)
}

func AnthropicToOpenAIStreamWithMCPHooks(hc *protocol.HandleContext, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool, hooks *AnthropicToOpenAIMCPHooks) (int, int, error) {
	c := hc.GinContext
	logrus.WithContext(c.Request.Context()).Info("Starting Anthropic to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				sendOpenAIStreamError(c, "Internal streaming error", "internal_error")
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Anthropic to OpenAI streaming response handler")
	}()

	conv := newAnthropicToOpenAIConverter(stream, responseModel, disableStreamUsage, hooks)
	usage, err := RunConverter(hc, conv, openaiChatSSEWriter(c))
	in, out := usage.InputTokens, usage.OutputTokens

	// MCP continuation: hook requested the stream to be retried
	if hookErr := conv.HookErr(); errors.Is(hookErr, ErrMCPStreamContinue) {
		return in, out, hookErr
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return in, out, nil
		}
		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			return in, out, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return in, out, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return in, out, streamErr
	}

	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to OpenAI stream canceled by client")
			return in, out, nil
		}
		if errors.Is(err, io.EOF) {
			return in, out, nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		streamErr := fmt.Errorf("anthropic stream error: %w", err)
		hc.DispatchStreamError(streamErr)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return in, out, streamErr
		}
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return in, out, streamErr
	}

	OpenAISSEDone(c)
	hc.CallOnStreamComplete()
	return in, out, nil
}

// sendOpenAIStreamChunk sends a ChatCompletionChunk as SSE
func sendOpenAIStreamChunk(c *gin.Context, chunk openai.ChatCompletionChunk, disableStreamUsage bool) {
	chunkMap, err := chunkToMap(chunk)
	if err != nil {
		logrus.WithContext(c.Request.Context()).Errorf("Failed to convert chunk to map: %v", err)
		return
	}

	// Cursor compatibility path must not expose usage in stream chunks.
	if disableStreamUsage {
		delete(chunkMap, "usage")
	}

	OpenAISSE(c, chunkMap)
}

func chunkToMap(chunk openai.ChatCompletionChunk) (map[string]interface{}, error) {
	bytes, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	var chunkMap map[string]interface{}
	if err := json.Unmarshal(bytes, &chunkMap); err != nil {
		return nil, err
	}
	return chunkMap, nil
}

// sendOpenAIStreamChunkForce helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunkForce(c *gin.Context, chunk map[string]interface{}) {
	OpenAISSE(c, chunk)
}

// sendOpenAIStreamError sends an error chunk in OpenAI format
func sendOpenAIStreamError(c *gin.Context, message, errorType string) {
	errorMap := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
		},
	}
	OpenAISSE(c, errorMap)
}

// createReasoningContentChunk creates a chunk with reasoning_content field
// This is a workaround for OpenAI's extended thinking format which is not natively supported in the SDK
func createReasoningContentChunk(chatID string, created int64, model, reasoning string) openai.ChatCompletionChunk {
	chunk := openai.ChatCompletionChunk{
		ID:      chatID,
		Created: created,
		Model:   model,
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Content: "",
				},
			},
		},
	}

	if reasoning == "" {
		return chunk
	}

	chunkJSON, _ := json.Marshal(chunk)
	var chunkMap map[string]interface{}
	json.Unmarshal(chunkJSON, &chunkMap)

	if choices, ok := chunkMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				delta["reasoning_content"] = reasoning
			}
		}
	}

	updatedJSON, _ := json.Marshal(chunkMap)
	json.Unmarshal(updatedJSON, &chunk)

	return chunk
}
