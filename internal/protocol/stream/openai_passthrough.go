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
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// ===================================================================
// OpenAI Handle Functions
// ===================================================================

// HandleOpenAIChatStream handles OpenAI chat streaming response.
// The input-token fallback (used only when the upstream reports no usage) comes
// from hc.EstimatedInputTokens, so the handler no longer takes the request.
func HandleOpenAIChatStream(hc *protocol.HandleContext, streamResp *openaistream.Stream[openai.ChatCompletionChunk]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	// Set SSE headers (mimicking OpenAI response headers)
	c := hc.GinContext
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var promptTokensTotal, inputTokens, outputTokens, cacheTokens, reasoningTokens int
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
				// Surface an error the SDK only set during this Next() (in-band
				// SSE error / pre-content failure) so the handler can emit a
				// retryable status instead of a clean finish.
				return false, streamResp.Err(), nil
			}
			return true, nil, new(streamResp.Current())
		},
		func(event interface{}) error {
			chunk := event.(*openai.ChatCompletionChunk)

			// Store the first chunk ID for usage estimation
			if firstChunkID == "" && chunk.ID != "" {
				firstChunkID = chunk.ID
			}

			// Accumulate usage from chunks (if present)
			if chunk.Usage.PromptTokens != 0 {
				promptTokensTotal = int(chunk.Usage.PromptTokens)
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
			// Track reasoning tokens from completion tokens details if available
			if chunk.Usage.CompletionTokensDetails.ReasoningTokens != 0 {
				reasoningTokens = int(chunk.Usage.CompletionTokensDetails.ReasoningTokens)
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

			// Send the chunk
			// Mark TTFT on the first content-bearing chunk; MarkFirstToken is idempotent.
			if choice.Delta.Content != "" || choice.Delta.Refusal != "" || len(choice.Delta.ToolCalls) > 0 || choice.Delta.JSON.FunctionCall.Valid() {
				protocol.MarkFirstToken(c)
			}
			OpenAISSE(c, chunkMap)
			return nil
		},
	)

	// Normalize: OpenAI prompt_tokens = total (cached + uncached); subtract cache
	// so inputTokens represents uncached-only, matching Anthropic semantics.
	if promptTokensTotal > 0 {
		inputTokens = promptTokensTotal - cacheTokens
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		if !hasUsage {
			inputTokens = hc.EstimatedInputTokens
			outputTokens = token.EstimateOutputTokens(contentBuilder.String())
		}

		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), err
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
		return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), err
	}

	if !hasUsage {
		inputTokens = hc.EstimatedInputTokens
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

	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}

// HandleOpenAIResponsesStream handles OpenAI Responses API streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesStream(hc *protocol.HandleContext, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	defer stream.Close()

	// Set SSE headers for Responses API (different from Chat Completions)
	c := hc.GinContext
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	var promptTokensTotal, inputTokens, outputTokens, cacheTokens, reasoningTokens int64
	var hasUsage bool

	protocol.RunLoop(c, func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Responses stream")
			return false
		default:
		}

		if !stream.Next() {
			return false
		}

		evt := stream.Current()

		// Marshal event using RawJSON() to avoid serializing empty union fields
		eventRaw := evt.RawJSON()
		eventType := evt.Type

		// Accumulate usage from the raw JSON (SDK struct fields may be zero for
		// providers that use custom serialisation or in unit-test fake decoders).
		if it := gjson.Get(eventRaw, "response.usage.input_tokens"); it.Exists() && it.Int() > 0 {
			promptTokensTotal = it.Int()
			hasUsage = true
		}
		if ot := gjson.Get(eventRaw, "response.usage.output_tokens"); ot.Exists() && ot.Int() > 0 {
			outputTokens = ot.Int()
		}

		// Note: Responses API may include cache tokens in usage details
		// Check if available in the raw JSON
		if gjson.Get(eventRaw, "response.usage.input_tokens_details").Exists() {
			if cachedTokens := gjson.Get(eventRaw, "response.usage.input_tokens_details.cached_tokens"); cachedTokens.Exists() {
				cacheTokens = cachedTokens.Int()
				logrus.WithContext(c.Request.Context()).Debugf("cached tokens: %v", cacheTokens)
			} else {
				// set raw use "0" as 0
				if modified, err := sjson.SetRaw(eventRaw, "response.usage.input_tokens_details.cached_tokens", "0"); err == nil {
					eventRaw = modified
				} else {
					logrus.WithContext(c.Request.Context()).WithError(err).Error("Failed to set cached tokens")
				}
			}
		}

		// Codex CLI's ResponseCompleted decoder rejects events whose
		// output_tokens_details lacks reasoning_tokens. Backfill 0 when the
		// upstream provider (e.g. DeepSeek) omits it.
		if gjson.Get(eventRaw, "response.usage").Exists() {
			if rt := gjson.Get(eventRaw, "response.usage.output_tokens_details.reasoning_tokens"); rt.Exists() {
				reasoningTokens = rt.Int()
			} else {
				if modified, err := sjson.SetRaw(eventRaw, "response.usage.output_tokens_details.reasoning_tokens", "0"); err == nil {
					eventRaw = modified
				} else {
					logrus.WithContext(c.Request.Context()).WithError(err).Error("Failed to set reasoning tokens")
				}
			}
		}

		if len(eventRaw) > 0 {
			if model := gjson.Get(eventRaw, "response.model"); model.Exists() && model.String() != "" {
				if modified, err := sjson.Set(eventRaw, "response.model", responseModel); err == nil {
					eventRaw = modified
				}
			}
		}

		OpenAIResponsesEvent(c, eventType, eventRaw)
		return true
	})

	// Normalize: OpenAI input_tokens = total; subtract cache for uncached-only semantics.
	if promptTokensTotal > 0 {
		inputTokens = promptTokensTotal - cacheTokens
	}

	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.WithContext(c.Request.Context()).Debug("Responses stream canceled by client")
			return protocol.NewTokenUsageFull(int(inputTokens), int(outputTokens), int(cacheTokens), int(reasoningTokens)), nil
		}

		logrus.WithContext(c.Request.Context()).Errorf("Responses stream error: %v", err)
		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return protocol.NewTokenUsageFull(int(inputTokens), int(outputTokens), int(cacheTokens), int(reasoningTokens)), err
		}
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}
		OpenAIResponsesEvent(c, "error", errorChunk)
		return protocol.NewTokenUsageFull(int(inputTokens), int(outputTokens), int(cacheTokens), int(reasoningTokens)), err
	}

	_ = hasUsage
	return protocol.NewTokenUsageFull(int(inputTokens), int(outputTokens), int(cacheTokens), int(reasoningTokens)), nil
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
func HandleOpenAIResponsesStreamToAnthropic(c *gin.Context, stream ResponsesStreamIter, responseModel string) (*protocol.TokenUsage, error) {
	defer stream.Close()

	logrus.WithContext(c.Request.Context()).Debug("[ChatGPT] Starting OpenAI Responses to Anthropic streaming handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("[ChatGPT] Panic in streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		logrus.WithContext(c.Request.Context()).Info("[ChatGPT] Finished OpenAI Responses to Anthropic streaming handler")
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

	var promptTokensTotal, inputTokens, outputTokens, cacheTokens, reasoningTokens int

	// Generate message ID for Anthropic format
	messageID := fmt.Sprintf("msg_%d", time.Now().Unix())

	// Send message_start event
	sendAnthropicV1MessageStart(c, messageID, responseModel, flusher)

	// Send content_block_start event
	sendAnthropicV1ContentBlockStart(c, flusher)

	// Process the stream using the SDK
	chunkCount := 0
	for stream.Next() {
		// Check context cancellation
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("[ChatGPT] Client disconnected, stopping stream")
			return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), c.Request.Context().Err()
		default:
		}

		evt := stream.Current()
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
							sendAnthropicV1ContentBlockDelta(c, content.Text, flusher)
						}
					}
				}
			}
		}

		// Extract usage from the event
		if evt.Response.Usage.InputTokens > 0 {
			promptTokensTotal = int(evt.Response.Usage.InputTokens)
		}
		if evt.Response.Usage.OutputTokens > 0 {
			outputTokens = int(evt.Response.Usage.OutputTokens)
		}
		if evt.Response.Usage.InputTokensDetails.CachedTokens > 0 {
			cacheTokens = int(evt.Response.Usage.InputTokensDetails.CachedTokens)
		}
		if evt.Response.Usage.OutputTokensDetails.ReasoningTokens > 0 {
			reasoningTokens = int(evt.Response.Usage.OutputTokensDetails.ReasoningTokens)
		}
	}

	// Normalize: OpenAI input_tokens = total; subtract cache for uncached-only semantics.
	if promptTokensTotal > 0 {
		inputTokens = promptTokensTotal - cacheTokens
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.WithContext(c.Request.Context()).Debug("[ChatGPT] Stream canceled by client")
			return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
		}
		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier.
		if !c.Writer.Written() {
			SendStreamingError(c, err)
		}
		return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), fmt.Errorf("stream error: %w", err)
	}

	logrus.WithContext(c.Request.Context()).Infof("[ChatGPT] Finished reading SSE stream: %d chunks, tokens: %d in, %d out", chunkCount, inputTokens, outputTokens)

	// Send content_block_stop event
	sendAnthropicV1ContentBlockStop(c, flusher)

	// Send message_stop event with usage
	sendAnthropicV1MessageStop(c, inputTokens, outputTokens, flusher)

	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}
