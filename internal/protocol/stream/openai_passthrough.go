package stream

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// ===================================================================
// Helper Functions
// ===================================================================

// GenerateObfuscationString generates a random string similar to "KOJz1A"
func GenerateObfuscationString() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto rand fails
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:6]
	}
	return base64.URLEncoding.EncodeToString(b)[:6]
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================

// HandleOpenAIChatStream handles OpenAI chat streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatStream(hc *protocol.HandleContext, stream *openaistream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams) (protocol.UsageStat, error) {
	defer stream.Close()

	// Set SSE headers (mimicking OpenAI response headers)
	c := hc.GinContext
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens int
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
		return protocol.ZeroUsageStat(), fmt.Errorf("streaming not supported")
	}

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !stream.Next() {
				return false, nil, nil
			}
			chunk := stream.Current()
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
			if choice.Delta.Content != "" {
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
			if len(choice.Delta.ToolCalls) > 0 {
				delta["tool_calls"] = choice.Delta.ToolCalls
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

			// Add usage if present (usually only in the last chunk)
			if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 {
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
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	if !hasUsage {
		inputTokens, _ = token.EstimateInputTokens(req)
		outputTokens = token.EstimateOutputTokens(contentBuilder.String())

		// Use the first chunk ID, or generate one if not available
		chunkID := firstChunkID
		if chunkID == "" {
			chunkID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		}

		// Send estimated usage as final chunk
		usageChunk := map[string]interface{}{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": 0,
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

	// Send the final [DONE] message
	// MENTION: must keep extra space
	c.SSEvent("", " [DONE]")
	flusher.Flush()

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleOpenAIResponsesStream handles OpenAI Responses API streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesStream(hc *protocol.HandleContext, stream *openaistream.Stream[responses.ResponseStreamEventUnion], responseModel string) (protocol.UsageStat, error) {
	defer stream.Close()

	// Set SSE headers for Responses API (different from Chat Completions)
	c := hc.GinContext
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	var inputTokens, outputTokens int64
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
		return protocol.ZeroUsageStat(), fmt.Errorf("streaming not supported")
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
				return protocol.NewUsageStat(int(inputTokens), int(outputTokens)), nil
			}
			return protocol.ZeroUsageStat(), nil
		}

		logrus.Errorf("Responses stream error: %v", err)
		if hasUsage {
			return protocol.NewUsageStat(int(inputTokens), int(outputTokens)), err
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
		return protocol.NewUsageStat(int(inputTokens), int(outputTokens)), err
	}

	// Send final [DONE] message
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()

	// Track successful streaming completion
	if hasUsage {
		return protocol.NewUsageStat(int(inputTokens), int(outputTokens)), nil
	}

	return protocol.ZeroUsageStat(), nil
}

// ===================================================================
// Helper Functions
// ===================================================================

// Note: The following functions are already defined in other files:
// - IsContextCanceled is in streaming.go
// - MarshalAndSendErrorEvent is in anthropic_error.go
// - SendFinishEvent is in anthropic_error.go
// - ErrorResponse and ErrorDetail are in server_types.go
