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
)

// AnthropicToOpenAIStream processes Anthropic streaming events and converts them to OpenAI format
// Returns inputTokens, outputTokens, and error for usage tracking
func AnthropicToOpenAIStream(c *gin.Context, req *anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], responseModel string, disableStreamUsage bool) (int, int, error) {
	logrus.Info("Starting Anthropic to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				sendOpenAIStreamError(c, "Internal streaming error", "internal_error")
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.Info("Finished Anthropic to OpenAI streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Track streaming state
	var (
		chatID       = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		created      = time.Now().Unix()
		contentText  = strings.Builder{}
		usage        *anthropic.BetaMessageDeltaUsage
		inputTokens  int
		outputTokens int
		finished     bool
		// Track tool call state for proper streaming
		toolCallID   string
		toolCallName string
		toolCallArgs strings.Builder
		hasToolCalls bool
		// Track thinking state for extended thinking support
		thinkingText strings.Builder
	)

	// Process the stream with context cancellation checking
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Anthropic to OpenAI stream")
			return false
		default:
		}

		// Try to get next event
		if !stream.Next() {
			return false
		}

		event := stream.Current()

		// Handle different event types
		switch event.Type {
		case "message_start":
			// Send initial chat completion chunk with role
			chunk := openai.ChatCompletionChunk{
				ID:      chatID,
				Created: created,
				Model:   responseModel,
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionChunkChoiceDelta{
							Role: "assistant",
						},
					},
				},
			}
			sendOpenAIStreamChunk(c, chunk, disableStreamUsage)

		case "content_block_start":
			// Content block starting
			if event.ContentBlock.Type == "text" {
				// Reset content builder for new text block
				contentText.Reset()
			} else if event.ContentBlock.Type == "tool_use" {
				// Tool use block starting - send first tool_call chunk
				toolCallID = event.ContentBlock.ID
				toolCallName = event.ContentBlock.Name
				toolCallArgs.Reset()
				hasToolCalls = true

				// Send initial tool_call chunk with id, type, and name
				chunk := openai.ChatCompletionChunk{
					ID:      chatID,
					Created: created,
					Model:   responseModel,
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionChunkChoiceDelta{
								ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
									{
										Index: 0,
										ID:    toolCallID,
										Type:  "function",
										Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
											Name:      toolCallName,
											Arguments: "",
										},
									},
								},
							},
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk, disableStreamUsage)
			} else if event.ContentBlock.Type == "thinking" {
				// Thinking block starting - reset thinking builder
				thinkingText.Reset()
			} else if event.ContentBlock.Type == "redacted_thinking" {
				// Redacted thinking - should be included as reasoning_content with placeholder
				thinkingText.Reset()
				thinkingText.WriteString("[REDACTED THINKING]")
			}

		case "content_block_delta":
			// Text, tool arguments, or thinking delta - send as OpenAI chunk
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				text := event.Delta.Text
				contentText.WriteString(text)

				chunk := openai.ChatCompletionChunk{
					ID:      chatID,
					Created: created,
					Model:   responseModel,
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionChunkChoiceDelta{
								Content: text,
							},
						},
					},
				}
				sendOpenAIStreamChunk(c, chunk, disableStreamUsage)
			} else if event.Delta.Type == "input_json_delta" && event.Delta.PartialJSON != "" {
				// Tool call arguments delta
				args := event.Delta.PartialJSON
				toolCallArgs.WriteString(args)

				// Send subsequent tool_call chunks with only arguments (no id, no name, no type)
				chunk := openai.ChatCompletionChunk{
					ID:      chatID,
					Created: created,
					Model:   responseModel,
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionChunkChoiceDelta{
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
				sendOpenAIStreamChunk(c, chunk, disableStreamUsage)
			} else if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
				// Thinking content delta - convert to OpenAI's reasoning_content format
				thinking := event.Delta.Thinking
				thinkingText.WriteString(thinking)

				// Send as reasoning_content - use custom chunk creation since reasoning_content is not a standard field
				chunk := createReasoningContentChunk(chatID, created, responseModel, thinking)
				sendOpenAIStreamChunk(c, chunk, disableStreamUsage)
			}
			// Note: signature_delta is intentionally ignored as OpenAI doesn't have an equivalent

		case "content_block_stop":
			// Content block finished - no specific action needed

		case "message_delta":
			// Message delta (includes usage info)
			if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
				usage = &event.Usage
				inputTokens = int(event.Usage.InputTokens)
				outputTokens = int(event.Usage.OutputTokens)
			}

		case "message_stop":
			// Determine the correct finish_reason
			// "tool_calls" if we had tool use, "stop" otherwise
			// Any of "stop", "length", "tool_calls", "content_filter", "function_call".
			finishReason := "stop"
			if hasToolCalls {
				finishReason = "tool_calls"
			}

			// Build delta for final chunk
			delta := openai.ChatCompletionChunkChoiceDelta{}
			if hasToolCalls {
				// For tool_calls, content should be empty string (matching DeepSeek format)
				delta.Content = ""
			}

			chunk := openai.ChatCompletionChunk{
				ID:      chatID,
				Created: created,
				Model:   responseModel,
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        delta,
						FinishReason: finishReason,
					},
				},
			}

			// Add usage if available and not disabled
			if !disableStreamUsage && usage != nil {
				chunk.Usage = openai.CompletionUsage{
					PromptTokens:     usage.InputTokens,
					CompletionTokens: usage.OutputTokens,
					TotalTokens:      usage.InputTokens + usage.OutputTokens,
				}
			}

			sendOpenAIStreamChunk(c, chunk, disableStreamUsage)
			// Send final [DONE] message
			// MENTION: must keep extra space (matching openai_chat.go:462)
			c.SSEvent("", " [DONE]")
			finished = true
			return false
		}

		return true
	})

	if finished {
		return inputTokens, outputTokens, nil
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Anthropic to OpenAI stream canceled by client")
			return inputTokens, outputTokens, nil
		}
		// EOF is expected when stream ends normally
		if errors.Is(err, io.EOF) {
			logrus.Info("Anthropic stream ended normally (EOF)")
			return inputTokens, outputTokens, nil
		}
		logrus.Errorf("Anthropic stream error: %v", err)
		sendOpenAIStreamError(c, err.Error(), "stream_error")
		return inputTokens, outputTokens, nil
	}

	return inputTokens, outputTokens, nil
}

// sendOpenAIStreamChunk sends a ChatCompletionChunk as SSE
func sendOpenAIStreamChunk(c *gin.Context, chunk openai.ChatCompletionChunk, disableStreamUsage bool) {
	chunkMap, err := chunkToMap(chunk)
	if err != nil {
		logrus.Errorf("Failed to convert chunk to map: %v", err)
		return
	}

	// Cursor compatibility path must not expose usage in stream chunks.
	if disableStreamUsage {
		delete(chunkMap, "usage")
	}

	chunkJSON, err := json.Marshal(chunkMap)
	if err != nil {
		logrus.Errorf("Failed to marshal chunk: %v", err)
		return
	}
	// MENTION: Must keep extra space (matching openai_chat.go:365)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
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

// sendOpenAIStreamChunk helper function to send a chunk in OpenAI format
func sendOpenAIStreamChunkForce(c *gin.Context, chunk map[string]interface{}) {
	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		logrus.Errorf("Failed to marshal chunk: %v", err)
		return
	}
	// MENTION: Must keep extra space (matching openai_chat.go:365)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

// sendOpenAIStreamError sends an error chunk in OpenAI format
func sendOpenAIStreamError(c *gin.Context, message, errorType string) {
	errorMap := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
		},
	}
	errorJSON, _ := json.Marshal(errorMap)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", errorJSON))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

// createReasoningContentChunk creates a chunk with reasoning_content field
// This is a workaround for OpenAI's extended thinking format which is not natively supported in the SDK
func createReasoningContentChunk(chatID string, created int64, model, reasoning string) openai.ChatCompletionChunk {
	// Create the chunk structure manually with reasoning_content
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

	// Marshal to JSON, add reasoning_content, and unmarshal back
	chunkJSON, _ := json.Marshal(chunk)
	var chunkMap map[string]interface{}
	json.Unmarshal(chunkJSON, &chunkMap)

	// Add reasoning_content to the delta
	if choices, ok := chunkMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				delta["reasoning_content"] = reasoning
			}
		}
	}

	// Marshal back and unmarshal into the struct
	updatedJSON, _ := json.Marshal(chunkMap)
	json.Unmarshal(updatedJSON, &chunk)

	return chunk
}
