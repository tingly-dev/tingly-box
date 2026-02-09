package server

import (
	"context"
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

	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// ===================================================================
// Anthropic Handle Functions
// ===================================================================

// HandleAnthropicV1Stream handles Anthropic v1 streaming response.
func HandleAnthropicV1Stream(hc *HandleContext, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) error {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int
	var hasUsage bool
	streamRec := newStreamRecorder(hc.Recorder)

	err := hc.ProcessStream(
		func() (bool, error) {
			if !streamResp.Next() {
				return false, nil
			}
			return true, nil
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			streamRec.RecordV1Event(evt)

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

	streamRec.Finish(hc.ResponseModel, inputTokens, outputTokens)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 stream canceled by client")
			if hasUsage {
				hc.TrackUsage(inputTokens, outputTokens, err)
			}
			streamRec.RecordError(err)
			return nil
		}

		hc.TrackUsage(inputTokens, outputTokens, err)
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		streamRec.RecordError(err)
		return err
	}

	if hasUsage {
		hc.TrackUsage(inputTokens, outputTokens, nil)
	}

	SendFinishEvent(hc.GinContext)
	streamRec.RecordResponse(hc.Provider, hc.ActualModel)

	return nil
}

// HandleAnthropicV1BetaStream handles Anthropic v1 beta streaming response.
func HandleAnthropicV1BetaStream(hc *HandleContext, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) error {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int
	var hasUsage bool
	streamRec := newStreamRecorder(hc.Recorder)

	err := hc.ProcessStream(
		func() (bool, error) {
			if !streamResp.Next() {
				return false, nil
			}
			return true, nil
		},
		func(event interface{}) error {
			evt := event.(*anthropic.BetaRawMessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			streamRec.RecordV1BetaEvent(evt)

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

	streamRec.Finish(hc.ResponseModel, inputTokens, outputTokens)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 beta stream canceled by client")
			if hasUsage {
				hc.TrackUsage(inputTokens, outputTokens, err)
			}
			streamRec.RecordError(err)
			return nil
		}

		hc.TrackUsage(inputTokens, outputTokens, err)
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		streamRec.RecordError(err)
		return err
	}

	if hasUsage {
		hc.TrackUsage(inputTokens, outputTokens, nil)
	}

	SendFinishEvent(hc.GinContext)
	streamRec.RecordResponse(hc.Provider, hc.ActualModel)

	return nil
}

// HandleAnthropicV1NonStream handles Anthropic v1 non-streaming response.
func HandleAnthropicV1NonStream(hc *HandleContext, resp *anthropic.Message) error {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	hc.TrackUsage(inputTokens, outputTokens, nil)

	resp.Model = anthropic.Model(hc.ResponseModel)

	if hc.Recorder != nil {
		hc.Recorder.SetAssembledResponse(resp)
		hc.Recorder.RecordResponse(hc.Provider, hc.ActualModel)
	}

	hc.GinContext.JSON(http.StatusOK, resp)
	return nil
}

// HandleAnthropicV1BetaNonStream handles Anthropic v1 beta non-streaming response.
func HandleAnthropicV1BetaNonStream(hc *HandleContext, resp *anthropic.BetaMessage) error {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	hc.TrackUsage(inputTokens, outputTokens, nil)

	resp.Model = anthropic.Model(hc.ResponseModel)

	if hc.Recorder != nil {
		hc.Recorder.SetAssembledResponse(resp)
		hc.Recorder.RecordResponse(hc.Provider, hc.ActualModel)
	}

	hc.GinContext.JSON(http.StatusOK, resp)
	return nil
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================

// HandleOpenAIChatStream handles OpenAI chat streaming response.
func HandleOpenAIChatStream(hc *HandleContext, stream *openaistream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams) error {
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
		return fmt.Errorf("streaming not supported")
	}

	err := hc.ProcessStream(
		func() (bool, error) {
			if !stream.Next() {
				return false, nil
			}
			return true, nil
		},
		func(event interface{}) error {
			chunk := event.(*openai.ChatCompletionChunk)

			if chunk.Usage.PromptTokens != 0 {
				inputTokens = int(chunk.Usage.PromptTokens)
				hasUsage = true
			}
			if chunk.Usage.CompletionTokens != 0 {
				outputTokens = int(chunk.Usage.CompletionTokens)
				hasUsage = true
			}

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				if choice.Delta.Content != "" {
					contentBuilder.WriteString(choice.Delta.Content)
				}
			}

			// Send the chunk
			chunkJSON, err := json.Marshal(chunk)
			if err != nil {
				return err
			}
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
		hc.TrackUsage(inputTokens, outputTokens, err)

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
		return err
	}

	if !hasUsage {
		inputTokens, _ = token.EstimateInputTokens(req)
		outputTokens = token.EstimateOutputTokens(contentBuilder.String())

		// Send estimated usage as final chunk
		chunkID := firstChunkID
		if chunkID == "" {
			chunkID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		}

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

	hc.TrackUsage(inputTokens, outputTokens, nil)

	// Send the final [DONE] message
	c.SSEvent("", " [DONE]")
	flusher.Flush()

	return nil
}

// HandleOpenAIChatNonStream handles OpenAI chat non-streaming response.
func HandleOpenAIChatNonStream(hc *HandleContext, resp *openai.ChatCompletion) error {
	inputTokens := int(resp.Usage.PromptTokens)
	outputTokens := int(resp.Usage.CompletionTokens)

	hc.TrackUsage(inputTokens, outputTokens, nil)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		hc.SendError(err, "api_error", "unmarshal_failed")
		return err
	}

	// Update response model
	responseMap["model"] = hc.ResponseModel

	hc.GinContext.JSON(http.StatusOK, responseMap)
	return nil
}

// HandleOpenAIResponsesStream handles OpenAI Responses API streaming response.
func HandleOpenAIResponsesStream(hc *HandleContext, stream *openaistream.Stream[responses.ResponseStreamEventUnion]) error {
	defer stream.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens int
	var hasUsage bool
	streamRec := newStreamRecorder(hc.Recorder)

	err := hc.ProcessStream(
		func() (bool, error) {
			if !stream.Next() {
				return false, nil
			}
			return true, nil
		},
		func(event interface{}) error {
			// Handle Responses API stream event
			// TODO: Implement proper event handling
			return nil
		},
	)

	streamRec.Finish(hc.ResponseModel, inputTokens, outputTokens)

	if err != nil {
		hc.TrackUsage(inputTokens, outputTokens, err)
		streamRec.RecordError(err)
		return err
	}

	if hasUsage {
		hc.TrackUsage(inputTokens, outputTokens, nil)
	}

	streamRec.RecordResponse(hc.Provider, hc.ActualModel)

	return nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
func HandleOpenAIResponsesNonStream(hc *HandleContext, resp *responses.Response) error {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	hc.TrackUsage(inputTokens, outputTokens, nil)

	hc.GinContext.JSON(http.StatusOK, resp)
	return nil
}

// ===================================================================
// Helper Functions
// ===================================================================

// Note: The following functions are already defined in other files:
// - IsContextCanceled is in streaming.go
// - MarshalAndSendErrorEvent is in anthropic_error.go
// - SendFinishEvent is in anthropic_error.go
// - ErrorResponse and ErrorDetail are in server_types.go
