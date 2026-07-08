package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
)

// HandleGoogleToOpenAIStreamResponse processes Google streaming events and converts them to OpenAI format
func HandleGoogleToOpenAIStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) error {
	logrus.WithContext(c.Request.Context()).Info("Starting Google to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Google to OpenAI streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Google to OpenAI streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("Streaming not supported by this connection")
	}

	// Track streaming state
	var (
		chatID     = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		created    = time.Now().Unix()
		toolCalls  []map[string]interface{}
		hasStarted bool
	)

	// Process the stream
	for googleResp, err := range stream {
		// Check context cancellation
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Google to OpenAI stream")
			return nil
		default:
		}

		if err != nil {
			// Check if it was a client cancellation
			if errors.Is(err, context.Canceled) {
				logrus.WithContext(c.Request.Context()).Debug("Google stream canceled by client")
				return nil
			}
			logrus.WithContext(c.Request.Context()).Errorf("Google stream error: %v", err)
			return nil
		}

		// Send initial chunk if not already sent
		if !hasStarted {
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{"role": "assistant"},
						"finish_reason": nil,
					},
				},
			}
			sendOpenAIStreamChunkForce(c, chunk)
			hasStarted = true
		}

		// Process candidates
		if len(googleResp.Candidates) > 0 {
			candidate := googleResp.Candidates[0]

			// Extract content
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					// Handle text parts
					if part.Text != "" {
						// Send text delta
						chunk := map[string]interface{}{
							"id":      chatID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   responseModel,
							"choices": []map[string]interface{}{
								{
									"index": 0,
									"delta": map[string]interface{}{
										"content": part.Text,
									},
									"finish_reason": nil,
								},
							},
						}
						sendOpenAIStreamChunkForce(c, chunk)
					}

					// Handle function calls
					if part.FunctionCall != nil {
						toolCall := map[string]interface{}{
							"id":   part.FunctionCall.ID,
							"type": "function",
							"function": map[string]interface{}{
								"name": part.FunctionCall.Name,
							},
						}
						// Marshal args to JSON string
						if argsBytes, err := json.Marshal(part.FunctionCall.Args); err == nil {
							toolCall["function"].(map[string]interface{})["arguments"] = string(argsBytes)
						}
						toolCalls = append(toolCalls, toolCall)

						// Send tool_calls delta
						chunk := map[string]interface{}{
							"id":      chatID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   responseModel,
							"choices": []map[string]interface{}{
								{
									"index": 0,
									"delta": map[string]interface{}{
										"tool_calls": []map[string]interface{}{toolCall},
									},
									"finish_reason": nil,
								},
							},
						}
						sendOpenAIStreamChunkForce(c, chunk)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				finishReason := nonstream.MapGoogleFinishReasonToOpenAI(candidate.FinishReason)

				// If there were tool calls, set finish reason accordingly
				if len(toolCalls) > 0 && finishReason == "stop" {
					finishReason = "tool_calls"
				}

				// Send final chunk with finish reason and usage
				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{},
							"finish_reason": finishReason,
						},
					},
				}

				// Add usage if available
				if googleResp.UsageMetadata != nil {
					chunk["usage"] = map[string]interface{}{
						"prompt_tokens":     googleResp.UsageMetadata.PromptTokenCount,
						"completion_tokens": googleResp.UsageMetadata.CandidatesTokenCount,
						"total_tokens":      googleResp.UsageMetadata.TotalTokenCount,
					}
				}

				sendOpenAIStreamChunkForce(c, chunk)
				// Send final [DONE] message
				c.Writer.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				return nil
			}
		}
	}

	return nil
}

// HandleGoogleToAnthropicStreamResponse processes Google streaming events and converts them to Anthropic format.
// Returns UsageStat containing token usage information for tracking.
func HandleGoogleToAnthropicStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) (*protocol.TokenUsage, error) {
	logrus.WithContext(c.Request.Context()).Info("Starting Google to Anthropic streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Google to Anthropic streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Google to Anthropic streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Track streaming state
	var (
		textBlockIndex = -1
		toolBlockIndex = -1
		usage          = protocol.ZeroTokenUsage()
	)

	// Send message_start event first.
	// First SSE byte for this stream — open the failover gate so buffered
	// output flushes and subsequent writes pass straight through.
	protocol.CommitFirstChunk(c)
	sendAnthropicStreamEvent(c, eventTypeMessageStart, newAnthropicMessageStartEvent(messageID, responseModel, 0), flusher)

	// Process the stream
	for googleResp, err := range stream {
		// Check context cancellation
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Google to Anthropic stream")
			return usage, nil
		default:
		}

		if err != nil {
			// Check if it was a client cancellation
			if errors.Is(err, context.Canceled) {
				logrus.WithContext(c.Request.Context()).Debug("Google stream canceled by client")
				return usage, nil
			}
			logrus.WithContext(c.Request.Context()).Errorf("Google stream error: %v", err)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": err.Error(),
					"type":    "stream_error",
					"code":    "stream_failed",
				},
			}
			sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
			return usage, err
		}

		// Process candidates
		if len(googleResp.Candidates) > 0 {
			candidate := googleResp.Candidates[0]

			// Extract content
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					// Handle text parts
					if part.Text != "" {
						// Send content_block_start for text on first occurrence
						if textBlockIndex == -1 {
							textBlockIndex = 0
							sendAnthropicStreamEvent(c, eventTypeContentBlockStart, anthropicContentBlockStartEvent{
								Type:         eventTypeContentBlockStart,
								Index:        textBlockIndex,
								ContentBlock: anthropicTextBlockStart(),
							}, flusher)
						}

						// Send content_block_delta with text
						sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, anthropicContentBlockDeltaEvent{
							Type:  eventTypeContentBlockDelta,
							Index: textBlockIndex,
							Delta: anthropicTextDelta(part.Text),
						}, flusher)
					}

					// Handle function calls
					if part.FunctionCall != nil {
						toolBlockIndex++
						// Send content_block_start for tool_use
						sendAnthropicStreamEvent(c, eventTypeContentBlockStart, anthropicContentBlockStartEvent{
							Type:         eventTypeContentBlockStart,
							Index:        toolBlockIndex,
							ContentBlock: anthropicToolUseBlockStartWithInput(part.FunctionCall.ID, part.FunctionCall.Name, part.FunctionCall.Args),
						}, flusher)

						// Send content_block_stop for this tool block
						sendAnthropicStreamEvent(c, eventTypeContentBlockStop, anthropicContentBlockStopEvent{
							Type:  eventTypeContentBlockStop,
							Index: toolBlockIndex,
						}, flusher)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				stopReason := nonstream.MapGoogleFinishReasonToAnthropic(candidate.FinishReason)

				// Send content_block_stop for text if applicable
				if textBlockIndex != -1 {
					sendAnthropicStreamEvent(c, eventTypeContentBlockStop, anthropicContentBlockStopEvent{
						Type:  eventTypeContentBlockStop,
						Index: textBlockIndex,
					}, flusher)
				}

				// Collect usage info
				if googleResp.UsageMetadata != nil {
					usage = protocol.NewTokenUsageWithCache(
						int(googleResp.UsageMetadata.PromptTokenCount),
						int(googleResp.UsageMetadata.CandidatesTokenCount),
						int(googleResp.UsageMetadata.CachedContentTokenCount),
					)
				}

				// Send message_delta with stop reason and usage
				messageDeltaEvent := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason":   stopReason,
						"stop_sequence": nil,
					},
					"usage": usage.ToAnthropicMessageDeltaUsageMap(),
				}
				sendAnthropicStreamEvent(c, "message_delta", messageDeltaEvent, flusher)

				// Send message_stop
				sendAnthropicStreamEvent(c, eventTypeMessageStop, anthropicMessageStopEvent{Type: eventTypeMessageStop}, flusher)
				return usage, nil
			}
		}

		// Track usage
		if googleResp.UsageMetadata != nil {
			usage = protocol.NewTokenUsageWithCache(
				int(googleResp.UsageMetadata.PromptTokenCount),
				int(googleResp.UsageMetadata.CandidatesTokenCount),
				int(googleResp.UsageMetadata.CachedContentTokenCount),
			)
		}
	}

	return usage, nil
}

// HandleGoogleToAnthropicBetaStreamResponse processes Google streaming events and converts them to Anthropic beta format.
// Returns UsageStat containing token usage information for tracking.
func HandleGoogleToAnthropicBetaStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) (*protocol.TokenUsage, error) {
	logrus.WithContext(c.Request.Context()).Info("Starting Google to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Google to Anthropic beta streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Google to Anthropic beta streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	// Generate message ID for Anthropic beta format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Track streaming state
	var (
		textBlockIndex = -1
		toolBlockIndex = -1
		usage          = protocol.ZeroTokenUsage()
	)

	// Send message_start event first.
	// First SSE byte for this stream — open the failover gate so buffered
	// output flushes and subsequent writes pass straight through.
	protocol.CommitFirstChunk(c)
	sendAnthropicStreamEvent(c, eventTypeMessageStart, newAnthropicMessageStartEvent(messageID, responseModel, 0), flusher)

	// Process the stream
	for googleResp, err := range stream {
		// Check context cancellation
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Google to Anthropic beta stream")
			return usage, nil
		default:
		}

		if err != nil {
			// Check if it was a client cancellation
			if errors.Is(err, context.Canceled) {
				logrus.WithContext(c.Request.Context()).Debug("Google stream canceled by client")
				return usage, nil
			}
			logrus.WithContext(c.Request.Context()).Errorf("Google stream error: %v", err)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": err.Error(),
					"type":    "stream_error",
					"code":    "stream_failed",
				},
			}
			sendAnthropicStreamEvent(c, "error", errorEvent, flusher)
			return usage, err
		}

		// Process candidates
		if len(googleResp.Candidates) > 0 {
			candidate := googleResp.Candidates[0]

			// Extract content
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					// Handle text parts
					if part.Text != "" {
						// Send content_block_start for text on first occurrence
						if textBlockIndex == -1 {
							textBlockIndex = 0
							sendAnthropicStreamEvent(c, eventTypeContentBlockStart, anthropicContentBlockStartEvent{
								Type:         eventTypeContentBlockStart,
								Index:        textBlockIndex,
								ContentBlock: anthropicTextBlockStart(),
							}, flusher)
						}

						// Send content_block_delta with text
						sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, anthropicContentBlockDeltaEvent{
							Type:  eventTypeContentBlockDelta,
							Index: textBlockIndex,
							Delta: anthropicTextDelta(part.Text),
						}, flusher)
					}

					// Handle function calls
					if part.FunctionCall != nil {
						toolBlockIndex++
						// Send content_block_start for tool_use
						sendAnthropicStreamEvent(c, eventTypeContentBlockStart, anthropicContentBlockStartEvent{
							Type:         eventTypeContentBlockStart,
							Index:        toolBlockIndex,
							ContentBlock: anthropicToolUseBlockStartWithInput(part.FunctionCall.ID, part.FunctionCall.Name, part.FunctionCall.Args),
						}, flusher)

						// Send content_block_stop for this tool block
						sendAnthropicStreamEvent(c, eventTypeContentBlockStop, anthropicContentBlockStopEvent{
							Type:  eventTypeContentBlockStop,
							Index: toolBlockIndex,
						}, flusher)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				stopReason := nonstream.MapGoogleFinishReasonToAnthropicBeta(candidate.FinishReason)

				// Send content_block_stop for text if applicable
				if textBlockIndex != -1 {
					sendAnthropicStreamEvent(c, eventTypeContentBlockStop, anthropicContentBlockStopEvent{
						Type:  eventTypeContentBlockStop,
						Index: textBlockIndex,
					}, flusher)
				}

				// Collect usage info
				if googleResp.UsageMetadata != nil {
					usage = protocol.NewTokenUsageWithCache(
						int(googleResp.UsageMetadata.PromptTokenCount),
						int(googleResp.UsageMetadata.CandidatesTokenCount),
						int(googleResp.UsageMetadata.CachedContentTokenCount),
					)
				}

				// Send message_delta with stop reason and usage
				messageDeltaEvent := map[string]interface{}{
					"type": eventTypeMessageDelta,
					"delta": map[string]interface{}{
						"stop_reason":   string(stopReason),
						"stop_sequence": "",
					},
					"usage": usage.ToAnthropicMessageDeltaUsageMap(),
				}
				sendAnthropicStreamEvent(c, eventTypeMessageDelta, messageDeltaEvent, flusher)

				// Send message_stop
				messageStopEvent := map[string]interface{}{
					"type": eventTypeMessageStop,
					"message": map[string]interface{}{
						"id":            messageID,
						"type":          "message",
						"role":          "assistant",
						"content":       []interface{}{},
						"model":         responseModel,
						"stop_reason":   string(stopReason),
						"stop_sequence": "",
						"usage":         usage.ToAnthropicMessageDeltaUsageMap(),
					},
				}
				sendAnthropicStreamEvent(c, eventTypeMessageStop, messageStopEvent, flusher)

				// Send final simple data with type (without event, aka empty)
				c.SSEvent("", map[string]interface{}{"type": eventTypeMessageStop})
				flusher.Flush()
				return usage, nil
			}
		}

		// Track usage
		if googleResp.UsageMetadata != nil {
			usage = protocol.NewTokenUsageWithCache(
				int(googleResp.UsageMetadata.PromptTokenCount),
				int(googleResp.UsageMetadata.CandidatesTokenCount),
				int(googleResp.UsageMetadata.CachedContentTokenCount),
			)
		}
	}

	return usage, nil
}
