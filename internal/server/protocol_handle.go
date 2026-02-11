package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
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

// generateObfuscationString generates a random string similar to "KOJz1A"
func generateObfuscationString() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto rand fails
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:6]
	}
	return base64.URLEncoding.EncodeToString(b)[:6]
}

// ===================================================================
// Anthropic Handle Functions
// ===================================================================

// HandleAnthropicV1Stream handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1Stream(hc *HandleContext, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (protocol.UsageStat, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}

			// Send SSE event
			eventJSON, err := json.Marshal(evt)
			if err != nil {
				return err
			}
			hc.GinContext.SSEvent(evt.Type, string(eventJSON))
			hc.GinContext.Writer.Flush()
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 stream canceled by client")
			if !hasUsage {
				return protocol.ZeroUsageStat(), nil
			}
			return protocol.NewUsageStat(inputTokens, outputTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleAnthropicV1BetaStream handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaStream(hc *HandleContext, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (protocol.UsageStat, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.BetaRawMessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}

			// Send SSE event
			eventJSON, err := json.Marshal(evt)
			if err != nil {
				return err
			}
			hc.GinContext.SSEvent(evt.Type, string(eventJSON))
			hc.GinContext.Writer.Flush()
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 beta stream canceled by client")
			if !hasUsage {
				return protocol.ZeroUsageStat(), nil
			}
			return protocol.NewUsageStat(inputTokens, outputTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleAnthropicV1NonStream handles Anthropic v1 non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1NonStream(hc *HandleContext, resp *anthropic.Message) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	resp.Model = anthropic.Model(hc.ResponseModel)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleAnthropicV1BetaNonStream handles Anthropic v1 beta non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaNonStream(hc *HandleContext, resp *anthropic.BetaMessage) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	resp.Model = anthropic.Model(hc.ResponseModel)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================

// HandleOpenAIChatStream handles OpenAI chat streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatStream(hc *HandleContext, stream *openaistream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams) (protocol.UsageStat, error) {
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
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
			obfuscationValue := generateObfuscationString() // Generate obfuscation value once per stream
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

// HandleOpenAIChatNonStream handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatNonStream(hc *HandleContext, resp *openai.ChatCompletion) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.PromptTokens)
	outputTokens := int(resp.Usage.CompletionTokens)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroUsageStat(), err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		hc.SendError(err, "api_error", "unmarshal_failed")
		return protocol.ZeroUsageStat(), err
	}

	// Update response model
	responseMap["model"] = hc.ResponseModel

	hc.GinContext.JSON(http.StatusOK, responseMap)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleOpenAIResponsesStream handles OpenAI Responses API streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesStream(hc *HandleContext, stream *openaistream.Stream[responses.ResponseStreamEventUnion]) (protocol.UsageStat, error) {
	defer stream.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !stream.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := stream.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			// Handle Responses API stream event
			// TODO: Implement proper event handling
			return nil
		},
	)

	if err != nil {
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesNonStream(hc *HandleContext, resp *responses.Response) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// ===================================================================
// Helper Functions
// ===================================================================

// Note: The following functions are already defined in other files:
// - IsContextCanceled is in streaming.go
// - MarshalAndSendErrorEvent is in anthropic_error.go
// - SendFinishEvent is in anthropic_error.go
// - ErrorResponse and ErrorDetail are in server_types.go
