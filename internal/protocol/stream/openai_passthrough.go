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

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// ===================================================================
// OpenAI Handle Functions
// ===================================================================

// HandleOpenAIChatStream handles OpenAI chat streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatStream(hc *protocol.HandleContext, streamResp *openaistream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	// Set SSE headers (mimicking OpenAI response headers)
	c := hc.GinContext
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool
	var contentBuilder strings.Builder
	var firstChunkID string

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
			Error: protocol.ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return protocol.ZeroTokenUsage(), fmt.Errorf("streaming not supported")
	}

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if streamResp.Err() != nil {
				return false, streamResp.Err(), nil
			}
			if !streamResp.Next() {
				return false, nil, nil
			}
			chunk := streamResp.Current()
			return true, nil, &chunk
		},
		func(event interface{}) error {
			chunk := event.(*openai.ChatCompletionChunk)

			// Store the first chunk ID for usage estimation
			if firstChunkID == "" && chunk.ID != "" {
				firstChunkID = chunk.ID
			}

			// Accumulate usage from chunks (if present)
			if chunk.Usage.PromptTokens != 0 {
				inputTokens = int(chunk.Usage.PromptTokens)
				hasUsage = true
			}
			if chunk.Usage.CompletionTokens != 0 {
				outputTokens = int(chunk.Usage.CompletionTokens)
				hasUsage = true
			}
			// Track cache tokens from prompt tokens details if available
			if chunk.Usage.PromptTokensDetails.CachedTokens != 0 {
				cacheTokens = int(chunk.Usage.PromptTokensDetails.CachedTokens)
				hasUsage = true
			}

			// Check if we have choices and they're not empty
			if len(chunk.Choices) == 0 {
				return nil
			}

			choice := chunk.Choices[0]

			// Accumulate content for estimation
			if choice.Delta.Content != "" {
				contentBuilder.WriteString(choice.Delta.Content)
			}

			// Build delta map - only include non-empty fields to avoid validation errors
			delta := map[string]interface{}{}
			if choice.Delta.Role != "" {
				delta["role"] = choice.Delta.Role
			}
			// Handle content field: null when tool_calls are present, otherwise use actual content or empty string
			hasToolCalls := len(choice.Delta.ToolCalls) > 0
			if hasToolCalls {
				// When tool_calls are present, content should be null (matching OpenAI spec)
				delta["content"] = nil
			} else if choice.Delta.Content != "" {
				delta["content"] = choice.Delta.Content
			} else {
				delta["content"] = ""
			}
			if choice.Delta.Refusal != "" {
				delta["refusal"] = choice.Delta.Refusal
			} else {
				delta["refusal"] = nil
			}
			if choice.Delta.JSON.FunctionCall.Valid() {
				delta["function_call"] = choice.Delta.FunctionCall
			}
			if hasToolCalls {
				// Clean up tool_calls to remove empty name/id fields in subsequent chunks
				// This matches OpenAI's actual streaming format where only the first chunk has name
				cleanedToolCalls := make([]map[string]interface{}, 0, len(choice.Delta.ToolCalls))
				for _, tc := range choice.Delta.ToolCalls {
					// Parse the raw JSON to get the actual fields
					var rawTc map[string]interface{}
					if err := json.Unmarshal([]byte(tc.RawJSON()), &rawTc); err == nil {
						cleanedTc := make(map[string]interface{})
						// Always include index
						if idx, ok := rawTc["index"]; ok {
							cleanedTc["index"] = idx
						}
						// Include id only if non-empty (first chunk has id, subsequent don't)
						if id, ok := rawTc["id"]; ok && id != "" {
							cleanedTc["id"] = id
						}
						// Include type only if id is present (first chunk only)
						// DeepSeek format: type only in first chunk, not in subsequent chunks
						if _, hasID := rawTc["id"]; hasID {
							if typ, ok := rawTc["type"]; ok {
								cleanedTc["type"] = typ
							}
						}
						// Handle function field - clean up empty name
						if fn, ok := rawTc["function"].(map[string]interface{}); ok {
							cleanedFn := make(map[string]interface{})
							// Only include name if non-empty (first chunk has name, subsequent don't)
							if name, ok := fn["name"]; ok && name != "" {
								cleanedFn["name"] = name
							}
							// Always include arguments if present
							if args, ok := fn["arguments"]; ok && args != "" {
								cleanedFn["arguments"] = args
							}
							if len(cleanedFn) > 0 {
								cleanedTc["function"] = cleanedFn
							}
						}
						// Only add if we have meaningful content
						if len(cleanedTc) > 1 { // At least index + one other field
							cleanedToolCalls = append(cleanedToolCalls, cleanedTc)
						}
					} else {
						// Fallback to raw value if parsing fails
						cleanedToolCalls = append(cleanedToolCalls, rawTc)
					}
				}
				if len(cleanedToolCalls) > 0 {
					delta["tool_calls"] = cleanedToolCalls
				}
			}

			finishReason := &choice.FinishReason
			if finishReason != nil && *finishReason == "" {
				finishReason = nil
			}

			// Prepare the chunk in OpenAI format
			chunkMap := map[string]interface{}{
				"id":      chunk.ID,
				"object":  "chat.completion.chunk",
				"created": chunk.Created,
				"model":   hc.ResponseModel,
				"choices": []map[string]interface{}{
					{
						"index":         choice.Index,
						"delta":         delta,
						"finish_reason": finishReason,
						"logprobs":      choice.Logprobs,
					},
				},
			}

			// Add usage if present (usually only in the last chunk) and not disabled
			if !hc.DisableStreamUsage && (chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0) {
				chunkMap["usage"] = chunk.Usage
			}

			// Add system fingerprint if present
			if chunk.SystemFingerprint != "" {
				chunkMap["system_fingerprint"] = chunk.SystemFingerprint
			}

			// Add service_tier if present
			if chunk.ServiceTier != "" {
				chunkMap["service_tier"] = chunk.ServiceTier
			} else {
				chunkMap["service_tier"] = "default"
			}

			// Add obfuscation if present in extra fields, otherwise use generated value
			obfuscationValue := GenerateObfuscationString() // Generate obfuscation value once per stream
			if obfuscationField, ok := chunk.JSON.ExtraFields["obfuscation"]; ok && obfuscationField.Valid() {
				var upstreamObfuscation string
				if err := json.Unmarshal([]byte(obfuscationField.Raw()), &upstreamObfuscation); err == nil {
					chunkMap["obfuscation"] = upstreamObfuscation
				} else {
					chunkMap["obfuscation"] = obfuscationValue
				}
			} else {
				chunkMap["obfuscation"] = obfuscationValue
			}

			// Convert to JSON and send as SSE
			chunkJSON, err := json.Marshal(chunkMap)
			if err != nil {
				return err
			}

			// Send the chunk
			// MENTION: Must keep extra space
			c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
			flusher.Flush()
			return nil
		},
	)

	if err != nil && !errors.Is(err, context.Canceled) {
		if !hasUsage {
			inputTokens, _ = token.EstimateInputTokens(req)
			outputTokens = token.EstimateOutputTokens(contentBuilder.String())
		}

		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, _ := json.Marshal(errorChunk)
		c.SSEvent("", string(errorJSON))
		flusher.Flush()
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	if !hasUsage {
		inputTokens, _ = token.EstimateInputTokens(req)
		outputTokens = token.EstimateOutputTokens(contentBuilder.String())

		// Use the first chunk ID, or generate one if not available
		chunkID := firstChunkID
		if chunkID == "" {
			chunkID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		}

		// Send estimated usage as final chunk (only if not disabled and we have actual usage)
		if !hc.DisableStreamUsage && (inputTokens > 0 || outputTokens > 0) {
			usageChunk := map[string]interface{}{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   hc.ResponseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": nil,
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     inputTokens,
					"completion_tokens": outputTokens,
					"total_tokens":      inputTokens + outputTokens,
				},
			}

			usageChunkJSON, err := json.Marshal(usageChunk)
			if err == nil {
				c.SSEvent("", string(usageChunkJSON))
				flusher.Flush()
			}
		}
	}

	// Send the final [DONE] message
	// MENTION: must keep extra space
	c.SSEvent("", " [DONE]")
	flusher.Flush()

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// HandleOpenAIResponsesStream handles OpenAI Responses API streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesStream(hc *protocol.HandleContext, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (*protocol.TokenUsage, error) {
	defer stream.Close()

	// Set SSE headers for Responses API (different from Chat Completions)
	c := hc.GinContext
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	var inputTokens, outputTokens, cacheTokens int64
	var hasUsage bool

	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Responses streaming handler: %v", r)
			if hasUsage {
				// Track panic as error with any usage we accumulated
				// Usage tracking will be handled by caller
			}
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
	}()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, protocol.ErrorResponse{
			Error: protocol.ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return protocol.ZeroTokenUsage(), fmt.Errorf("streaming not supported")
	}

	// Process the stream with context cancellation checking
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Responses stream")
			return false
		default:
		}

		// Try to get next event
		if !stream.Next() {
			// Stream ended naturally
			return false
		}

		evt := stream.Current()

		// Accumulate usage from completed events
		if evt.Response.Usage.InputTokens > 0 {
			inputTokens = evt.Response.Usage.InputTokens
			hasUsage = true
		}
		if evt.Response.Usage.OutputTokens > 0 {
			outputTokens = evt.Response.Usage.OutputTokens
		}
		// Note: Responses API may include cache tokens in usage details
		// Check if available in the raw JSON
		var evtParsed map[string]interface{}
		if err := json.Unmarshal([]byte(evt.RawJSON()), &evtParsed); err == nil {
			if response, ok := evtParsed["response"].(map[string]interface{}); ok {
				if usage, ok := response["usage"].(map[string]interface{}); ok {
					if details, ok := usage["input_tokens_details"].(map[string]interface{}); ok {
						if cached, ok := details["cached_tokens"].(float64); ok {
							cacheTokens = int64(cached)
						}
					}
				}
			}
		}

		// Marshal event using RawJSON() to avoid serializing empty union fields
		jsonBytes := []byte(evt.RawJSON())

		// Apply model override if the event contains a response object with a model field
		if len(jsonBytes) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &parsed); err == nil {
				// Check if this event has a response field with a model
				if response, ok := parsed["response"].(map[string]interface{}); ok {
					if model, ok2 := response["model"].(string); ok2 && model != "" {
						response["model"] = responseModel
						modified, err := json.Marshal(parsed)
						if err == nil {
							jsonBytes = modified
						}
					}
				}
			}
		}

		// Send SSE event with event type (e.g., "response.created", "response.output_text.delta")
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", jsonBytes))
		flusher.Flush()
		return true
	})

	// Check for stream errors after loop completes
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("Responses stream canceled by client")
			if hasUsage {
				return protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil
			}
			return protocol.ZeroTokenUsage(), nil
		}

		logrus.Errorf("Responses stream error: %v", err)
		if hasUsage {
			return protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), err
		}

		// Send error chunk
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(errorJSON)))
		flusher.Flush()
		return protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), err
	}

	// Send final [DONE] message
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()

	// Track successful streaming completion
	if hasUsage {
		return protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil
	}

	return protocol.ZeroTokenUsage(), nil
}

// ===================================================================
// Helper Functions
// ===================================================================

// Note: The following functions are already defined in other files:
// - IsContextCanceled is in streaming.go
// - MarshalAndSendErrorEvent is in anthropic_error.go
// - SendFinishEvent is in anthropic_error.go
// - ErrorResponse and ErrorDetail are in server_types.go

// ===================================================================
// OpenAI Responses to Anthropic Transform Functions
// ===================================================================

// HandleOpenAIResponsesStreamToAnthropic handles OpenAI Responses API streaming response
// and transforms it to Anthropic message format.
// This is used for ChatGPT backend API providers when the original request was in Anthropic format.
// Returns (TokenUsage, error)
func HandleOpenAIResponsesStreamToAnthropic(c *gin.Context, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string, useV1Format bool) (*protocol.TokenUsage, error) {
	defer stream.Close()

	logrus.Info("[ChatGPT] Starting OpenAI Responses to Anthropic streaming handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[ChatGPT] Panic in streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.Info("[ChatGPT] Finished OpenAI Responses to Anthropic streaming handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), fmt.Errorf("streaming not supported by this connection")
	}

	var inputTokens, outputTokens int

	// Generate message ID for Anthropic format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Send message_start event
	if useV1Format {
		sendAnthropicV1MessageStart(c, messageID, responseModel, flusher)
	} else {
		sendAnthropicBetaMessageStart(c, messageID, responseModel, flusher)
	}

	// Send content_block_start event
	if useV1Format {
		sendAnthropicV1ContentBlockStart(c, flusher)
	} else {
		sendAnthropicBetaContentBlockStart(c, flusher)
	}

	// Process the stream using the SDK
	chunkCount := 0
	for stream.Next() {
		// Check context cancellation
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("[ChatGPT] Client disconnected, stopping stream")
			return protocol.NewTokenUsage(inputTokens, outputTokens), c.Request.Context().Err()
		default:
		}

		evt := stream.Current()

		if chunkCount < 3 {
			logrus.Debugf("[ChatGPT] SSE chunk #%d: %s", chunkCount+1, evt.RawJSON())
		}

		chunkCount++

		// Extract content from the response event
		// The event is already in OpenAI Responses API format (after transformation by chatGPTBackendRoundTripper)
		for _, item := range evt.Response.Output {
			if item.Type == "message" {
				for _, content := range item.Content {
					// Handle different content types
					if content.Type == "output_text" || content.Type == "text" {
						if content.Text != "" {
							// Send content_block_delta event
							if useV1Format {
								sendAnthropicV1ContentBlockDelta(c, content.Text, flusher)
							} else {
								sendAnthropicBetaContentBlockDelta(c, content.Text, flusher)
							}
						}
					}
				}
			}
		}

		// Extract usage from the event
		if evt.Response.Usage.InputTokens > 0 {
			inputTokens = int(evt.Response.Usage.InputTokens)
		}
		if evt.Response.Usage.OutputTokens > 0 {
			outputTokens = int(evt.Response.Usage.OutputTokens)
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("[ChatGPT] Stream canceled by client")
			return protocol.NewTokenUsage(inputTokens, outputTokens), nil
		}
		return protocol.NewTokenUsage(inputTokens, outputTokens), fmt.Errorf("stream error: %w", err)
	}

	logrus.Infof("[ChatGPT] Finished reading SSE stream: %d chunks, tokens: %d in, %d out", chunkCount, inputTokens, outputTokens)

	// Send content_block_stop event
	if useV1Format {
		sendAnthropicV1ContentBlockStop(c, flusher)
	} else {
		sendAnthropicBetaContentBlockStop(c, flusher)
	}

	// Send message_stop event with usage
	if useV1Format {
		sendAnthropicV1MessageStop(c, inputTokens, outputTokens, flusher)
	} else {
		sendAnthropicBetaMessageStop(c, inputTokens, outputTokens, flusher)
	}

	return protocol.NewTokenUsage(inputTokens, outputTokens), nil
}
