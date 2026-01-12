package adaptor

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"
)

// HandleGoogleToOpenAIStreamResponse processes Google streaming events and converts them to OpenAI format
func HandleGoogleToOpenAIStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) error {
	logrus.Info("Starting Google to OpenAI streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Google to OpenAI streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.Info("Finished Google to OpenAI streaming response handler")
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
		if err != nil {
			logrus.Errorf("Google stream error: %v", err)
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
			sendOpenAIStreamChunk(c, chunk)
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
						sendOpenAIStreamChunk(c, chunk)
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
						sendOpenAIStreamChunk(c, chunk)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				finishReason := mapGoogleFinishReasonToOpenAI(candidate.FinishReason)

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

				sendOpenAIStreamChunk(c, chunk)
				// Send final [DONE] message
				c.Writer.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				return nil
			}
		}
	}

	return nil
}

// HandleGoogleToAnthropicStreamResponse processes Google streaming events and converts them to Anthropic format
func HandleGoogleToAnthropicStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) error {
	logrus.Info("Starting Google to Anthropic streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Google to Anthropic streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.Info("Finished Google to Anthropic streaming response handler")
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

	// Generate message ID for Anthropic format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Track streaming state
	var (
		textBlockIndex = -1
		toolBlockIndex = -1
		outputTokens   int64
	)

	// Send message_start event first
	messageStartEvent := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         responseModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicStreamEventFromG(c, "message_start", messageStartEvent, flusher)

	// Process the stream
	for googleResp, err := range stream {
		if err != nil {
			logrus.Errorf("Google stream error: %v", err)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": err.Error(),
					"type":    "stream_error",
					"code":    "stream_failed",
				},
			}
			sendAnthropicStreamEventFromG(c, "error", errorEvent, flusher)
			return nil
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
							contentBlockStartEvent := map[string]interface{}{
								"type":  "content_block_start",
								"index": textBlockIndex,
								"content_block": map[string]interface{}{
									"type": "text",
									"text": "",
								},
							}
							sendAnthropicStreamEventFromG(c, "content_block_start", contentBlockStartEvent, flusher)
						}

						// Send content_block_delta with text
						deltaEvent := map[string]interface{}{
							"type":  "content_block_delta",
							"index": textBlockIndex,
							"delta": map[string]interface{}{
								"type": "text_delta",
								"text": part.Text,
							},
						}
						sendAnthropicStreamEventFromG(c, "content_block_delta", deltaEvent, flusher)
					}

					// Handle function calls
					if part.FunctionCall != nil {
						toolBlockIndex++
						// Send content_block_start for tool_use
						contentBlockStartEvent := map[string]interface{}{
							"type":  "content_block_start",
							"index": toolBlockIndex,
							"content_block": map[string]interface{}{
								"type":  "tool_use",
								"id":    part.FunctionCall.ID,
								"name":  part.FunctionCall.Name,
								"input": part.FunctionCall.Args,
							},
						}
						sendAnthropicStreamEventFromG(c, "content_block_start", contentBlockStartEvent, flusher)

						// Send content_block_stop for this tool block
						contentBlockStopEvent := map[string]interface{}{
							"type":  "content_block_stop",
							"index": toolBlockIndex,
						}
						sendAnthropicStreamEventFromG(c, "content_block_stop", contentBlockStopEvent, flusher)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				stopReason := mapGoogleFinishReasonToAnthropic(candidate.FinishReason)

				// Send content_block_stop for text if applicable
				if textBlockIndex != -1 {
					contentBlockStopEvent := map[string]interface{}{
						"type":  "content_block_stop",
						"index": textBlockIndex,
					}
					sendAnthropicStreamEventFromG(c, "content_block_stop", contentBlockStopEvent, flusher)
				}

				// Collect usage info
				if googleResp.UsageMetadata != nil {
					outputTokens = int64(googleResp.UsageMetadata.CandidatesTokenCount)
				}

				// Send message_delta with stop reason and usage
				messageDeltaEvent := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason":   stopReason,
						"stop_sequence": nil,
					},
					"usage": map[string]interface{}{
						"output_tokens": outputTokens,
					},
				}
				sendAnthropicStreamEventFromG(c, "message_delta", messageDeltaEvent, flusher)

				// Send message_stop
				messageStopEvent := map[string]interface{}{
					"type": "message_stop",
				}
				sendAnthropicStreamEventFromG(c, "message_stop", messageStopEvent, flusher)
				return nil
			}
		}

		// Track usage
		if googleResp.UsageMetadata != nil {
			outputTokens = int64(googleResp.UsageMetadata.CandidatesTokenCount)
		}
	}

	return nil
}

// sendAnthropicStreamEventFromG helper function (rename to avoid duplicate)
func sendAnthropicStreamEventFromG(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic stream event: %v", err)
		return
	}

	// Anthropic SSE format: event: <type>\ndata: <json>\n\n
	c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(eventJSON))))
	flusher.Flush()
}

// HandleGoogleToAnthropicBetaStreamResponse processes Google streaming events and converts them to Anthropic beta format
func HandleGoogleToAnthropicBetaStreamResponse(c *gin.Context, stream iter.Seq2[*genai.GenerateContentResponse, error], responseModel string) error {
	logrus.Info("Starting Google to Anthropic beta streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Google to Anthropic beta streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.Info("Finished Google to Anthropic beta streaming response handler")
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

	// Generate message ID for Anthropic beta format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Track streaming state
	var (
		textBlockIndex = -1
		toolBlockIndex = -1
		outputTokens   int64
	)

	// Send message_start event first
	messageStartEvent := map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         responseModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicBetaStreamEventFromG(c, eventTypeMessageStart, messageStartEvent, flusher)

	// Process the stream
	for googleResp, err := range stream {
		if err != nil {
			logrus.Errorf("Google stream error: %v", err)
			errorEvent := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"message": err.Error(),
					"type":    "stream_error",
					"code":    "stream_failed",
				},
			}
			sendAnthropicBetaStreamEventFromG(c, "error", errorEvent, flusher)
			return nil
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
							contentBlockStartEvent := map[string]interface{}{
								"type":  eventTypeContentBlockStart,
								"index": textBlockIndex,
								"content_block": map[string]interface{}{
									"type": "text",
									"text": "",
								},
							}
							sendAnthropicBetaStreamEventFromG(c, eventTypeContentBlockStart, contentBlockStartEvent, flusher)
						}

						// Send content_block_delta with text
						deltaEvent := map[string]interface{}{
							"type":  eventTypeContentBlockDelta,
							"index": textBlockIndex,
							"delta": map[string]interface{}{
								"type": "text_delta",
								"text": part.Text,
							},
						}
						sendAnthropicBetaStreamEventFromG(c, eventTypeContentBlockDelta, deltaEvent, flusher)
					}

					// Handle function calls
					if part.FunctionCall != nil {
						toolBlockIndex++
						// Send content_block_start for tool_use
						contentBlockStartEvent := map[string]interface{}{
							"type":  eventTypeContentBlockStart,
							"index": toolBlockIndex,
							"content_block": map[string]interface{}{
								"type":  "tool_use",
								"id":    part.FunctionCall.ID,
								"name":  part.FunctionCall.Name,
								"input": part.FunctionCall.Args,
							},
						}
						sendAnthropicBetaStreamEventFromG(c, eventTypeContentBlockStart, contentBlockStartEvent, flusher)

						// Send content_block_stop for this tool block
						contentBlockStopEvent := map[string]interface{}{
							"type":  eventTypeContentBlockStop,
							"index": toolBlockIndex,
						}
						sendAnthropicBetaStreamEventFromG(c, eventTypeContentBlockStop, contentBlockStopEvent, flusher)
					}
				}
			}

			// Check for finish reason
			if candidate.FinishReason != "" {
				stopReason := mapGoogleFinishReasonToAnthropicBeta(candidate.FinishReason)

				// Send content_block_stop for text if applicable
				if textBlockIndex != -1 {
					contentBlockStopEvent := map[string]interface{}{
						"type":  eventTypeContentBlockStop,
						"index": textBlockIndex,
					}
					sendAnthropicBetaStreamEventFromG(c, eventTypeContentBlockStop, contentBlockStopEvent, flusher)
				}

				// Collect usage info
				if googleResp.UsageMetadata != nil {
					outputTokens = int64(googleResp.UsageMetadata.CandidatesTokenCount)
				}

				// Send message_delta with stop reason and usage
				messageDeltaEvent := map[string]interface{}{
					"type": eventTypeMessageDelta,
					"delta": map[string]interface{}{
						"stop_reason":   string(stopReason),
						"stop_sequence": "",
					},
					"usage": map[string]interface{}{
						"output_tokens": outputTokens,
					},
				}
				sendAnthropicBetaStreamEventFromG(c, eventTypeMessageDelta, messageDeltaEvent, flusher)

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
						"usage": map[string]interface{}{
							"output_tokens": outputTokens,
						},
					},
				}
				sendAnthropicBetaStreamEventFromG(c, eventTypeMessageStop, messageStopEvent, flusher)

				// Send final simple data with type (without event, aka empty)
				c.SSEvent("", map[string]interface{}{"type": eventTypeMessageStop})
				flusher.Flush()
				return nil
			}
		}

		// Track usage
		if googleResp.UsageMetadata != nil {
			outputTokens = int64(googleResp.UsageMetadata.CandidatesTokenCount)
		}
	}

	return nil
}

// sendAnthropicBetaStreamEventFromG helper function for beta streaming
func sendAnthropicBetaStreamEventFromG(c *gin.Context, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic beta stream event: %v", err)
		return
	}

	// Anthropic beta SSE format: event: <type>\ndata: <json>\n\n
	c.SSEvent(eventType, string(eventJSON))
	flusher.Flush()
}
