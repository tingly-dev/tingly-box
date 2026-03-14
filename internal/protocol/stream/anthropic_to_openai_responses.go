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
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleAnthropicToOpenAIResponsesStream converts Anthropic streaming events
// to OpenAI Responses API format.
//
// Returns (UsageStat, error) for usage tracking and error handling.
func HandleAnthropicToOpenAIResponsesStream(
	hc *protocol.HandleContext,
	stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion],
	responseModel string,
) (*protocol.TokenUsage, error) {
	logrus.Info("Starting Anthropic to OpenAI Responses streaming converter")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to Responses converter: %v", r)
			if hc.GinContext.Writer != nil {
				hc.GinContext.Writer.WriteHeader(http.StatusInternalServerError)
				sendResponsesErrorEvent(hc.GinContext, "Internal streaming error", "internal_error")
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.Info("Finished Anthropic to Responses converter")
	}()

	// Set SSE headers for Responses API
	c := hc.GinContext
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

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

	// Initialize converter state
	state := newResponsesConverterState(time.Now().Unix())
	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool
	completedSent := false

	// Process the stream
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Anthropic to Responses stream")
			// Send completion event before returning since client is disconnecting
			if !completedSent && !state.finished {
				logrus.Info("Client disconnected, sending completion event before close")
				sendFinalCompletionEvent(c, state, flusher, inputTokens, outputTokens, cacheTokens)
				completedSent = true
			}
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
			handleMessageStart(c, state, responseModel, flusher)
			state.hasSentCreated = true

		case "content_block_start":
			handleContentBlockStart(c, state, event, flusher)

		case "content_block_delta":
			handleContentBlockDelta(c, state, event, flusher)

		case "content_block_stop":
			handleContentBlockStop(c, state, event, flusher)

		case "message_delta":
			inputTokens, outputTokens, cacheTokens, hasUsage = handleMessageDelta(
				state, event, inputTokens, outputTokens,
			)

		case "message_stop":
			handleMessageStop(c, state, flusher)
			completedSent = true
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Only send completion event if not already sent
		if !completedSent && !state.finished {
			// Send completion event for all errors including context.Canceled
			// The Stream loop's context check may not have run if stream.Next() was blocking
			logrus.WithError(err).Warn("Stream error occurred, sending completion event")
			sendFinalCompletionEvent(c, state, flusher, inputTokens, outputTokens, cacheTokens)
			completedSent = true
		}

		if errors.Is(err, context.Canceled) {
			logrus.Debug("Anthropic to Responses stream canceled by client")
			if hasUsage {
				return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
			}
			return protocol.ZeroTokenUsage(), nil
		}

		if errors.Is(err, io.EOF) {
			logrus.Info("Anthropic stream ended normally (EOF)")
			if hasUsage {
				return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
			}
			return protocol.ZeroTokenUsage(), nil
		}

		logrus.Errorf("Anthropic stream error: %v", err)
		sendResponsesErrorEvent(c, err.Error(), "stream_error", flusher)
		if hasUsage {
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
		}
		return protocol.ZeroTokenUsage(), err
	}

	// Some providers end the stream without emitting message_stop
	// Ensure clients still receive response.completed and [DONE]
	if !completedSent && !state.finished {
		if !state.hasSentCreated {
			handleMessageStart(c, state, responseModel, flusher)
			state.hasSentCreated = true
		}
		sendFinalCompletionEvent(c, state, flusher, inputTokens, outputTokens, cacheTokens)
		completedSent = true
	}

	if hasUsage {
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
	}
	return protocol.ZeroTokenUsage(), nil
}

// responsesConverterState maintains the state during stream conversion
type responsesConverterState struct {
	responseID       string
	itemID           string
	outputIndex      int
	accumulatedText  string
	inputTokens      int64
	outputTokens     int64
	cacheTokens      int64 // Cache read tokens from Anthropic
	finished         bool
	pendingToolCalls map[int]*pendingResponseToolCall
	hasSentCreated   bool
	sequenceNumber   int
	createdAt        int64
	currentBlockType string // Track the type of the current block being processed
}

// pendingResponseToolCall tracks a tool call being assembled from Anthropic stream chunks
type pendingResponseToolCall struct {
	itemID    string
	name      string
	arguments strings.Builder
}

// newResponsesConverterState creates a new converter state with generated IDs
func newResponsesConverterState(timestamp int64) *responsesConverterState {
	return &responsesConverterState{
		responseID:       fmt.Sprintf("resp_%d", timestamp),
		itemID:           fmt.Sprintf("item_%d", timestamp),
		outputIndex:      0,
		pendingToolCalls: make(map[int]*pendingResponseToolCall),
		hasSentCreated:   false,
		sequenceNumber:   0,
		createdAt:        timestamp,
	}
}

// nextSequenceNumber returns the next sequence number and increments it
func (s *responsesConverterState) nextSequenceNumber() int {
	seq := s.sequenceNumber
	s.sequenceNumber++
	return seq
}

// handleMessageStart sends the response.created event
func handleMessageStart(c *gin.Context, state *responsesConverterState, model string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":         state.responseID,
			"object":     "response",
			"created_at": state.createdAt,
			"status":     "in_progress",
			"model":      model,
			"output":     []interface{}{},
			"usage":      nil,
		},
		"sequence_number": state.nextSequenceNumber(),
	}
	sendResponsesEvent(c, event, flusher)

	// Also send response.in_progress event as per the real API
	inProgressEvent := map[string]interface{}{
		"type": "response.in_progress",
		"response": map[string]interface{}{
			"id":         state.responseID,
			"object":     "response",
			"created_at": state.createdAt,
			"status":     "in_progress",
			"model":      model,
			"output":     []interface{}{},
			"usage":      nil,
		},
		"sequence_number": state.nextSequenceNumber(),
	}
	sendResponsesEvent(c, inProgressEvent, flusher)
}

// handleContentBlockStart sends the response.output_item.added event
func handleContentBlockStart(
	c *gin.Context,
	state *responsesConverterState,
	event anthropic.MessageStreamEventUnion,
	flusher http.Flusher,
) {
	index := event.Index
	blockType := event.ContentBlock.Type
	state.currentBlockType = blockType

	if blockType == "text" {
		// Handle text output - send response.output_item.added with message type
		outputEvent := map[string]interface{}{
			"type":         "response.output_item.added",
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
			"item": map[string]interface{}{
				"id":      state.itemID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []interface{}{},
			},
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, outputEvent, flusher)

		// Also send response.content_part.added for the text part
		contentPartEvent := map[string]interface{}{
			"type":          "response.content_part.added",
			"item_id":       state.itemID,
			"output_index":  state.outputIndex,
			"content_index": 0,
			"part": map[string]interface{}{
				"type": "output_text",
				"text": "",
			},
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, contentPartEvent, flusher)
	} else if blockType == "tool_use" {
		// Handle tool use - create a new pending tool call
		// The ID and Name are in ContentBlock fields for tool_use type
		toolID := event.ContentBlock.ID
		toolName := event.ContentBlock.Name

		state.pendingToolCalls[int(index)] = &pendingResponseToolCall{
			itemID:    toolID,
			name:      toolName,
			arguments: strings.Builder{},
		}

		outputEvent := map[string]interface{}{
			"type": "response.output_item.added",
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        toolID,
				"call_id":   toolID,
				"name":      toolName,
				"arguments": "",
				"status":    "in_progress",
			},
			"output_index":    state.outputIndex,
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, outputEvent, flusher)
		state.outputIndex++
	}
	// Ignore other block types (thinking, etc.)
}

// handleContentBlockDelta sends the appropriate delta event
func handleContentBlockDelta(
	c *gin.Context,
	state *responsesConverterState,
	event anthropic.MessageStreamEventUnion,
	flusher http.Flusher,
) {
	deltaType := event.Delta.Type
	index := event.Index

	if deltaType == "text_delta" {
		// Handle text delta
		text := event.Delta.Text
		state.accumulatedText += text

		deltaEvent := map[string]interface{}{
			"type":            "response.output_text.delta",
			"delta":           text,
			"item_id":         state.itemID,
			"output_index":    state.outputIndex,
			"content_index":   0,
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, deltaEvent, flusher)
	} else if deltaType == "input_json_delta" {
		// Handle tool call arguments delta
		if pending, exists := state.pendingToolCalls[int(index)]; exists {
			argsDelta := event.Delta.PartialJSON
			pending.arguments.WriteString(argsDelta)

			deltaEvent := map[string]interface{}{
				"type":            "response.function_call_arguments.delta",
				"delta":           argsDelta,
				"item_id":         pending.itemID,
				"output_index":    state.outputIndex,
				"sequence_number": state.nextSequenceNumber(),
			}
			sendResponsesEvent(c, deltaEvent, flusher)
		}
	}
}

// handleContentBlockStop sends the appropriate completion events based on block type
func handleContentBlockStop(
	c *gin.Context,
	state *responsesConverterState,
	event anthropic.MessageStreamEventUnion,
	flusher http.Flusher,
) {
	index := event.Index
	blockType := state.currentBlockType

	if blockType == "text" {
		// Send response.output_text.done event
		textDoneEvent := map[string]interface{}{
			"type":            "response.output_text.done",
			"item_id":         state.itemID,
			"output_index":    state.outputIndex,
			"content_index":   0,
			"text":            state.accumulatedText,
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, textDoneEvent, flusher)

		// Send response.content_part.done event
		contentPartDoneEvent := map[string]interface{}{
			"type":          "response.content_part.done",
			"item_id":       state.itemID,
			"output_index":  state.outputIndex,
			"content_index": 0,
			"part": map[string]interface{}{
				"type": "output_text",
				"text": state.accumulatedText,
			},
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, contentPartDoneEvent, flusher)

		// Send response.output_item.done event
		itemDoneEvent := map[string]interface{}{
			"type":         "response.output_item.done",
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
			"item": map[string]interface{}{
				"id":     state.itemID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]interface{}{
					{
						"type": "output_text",
						"text": state.accumulatedText,
					},
				},
			},
			"sequence_number": state.nextSequenceNumber(),
		}
		sendResponsesEvent(c, itemDoneEvent, flusher)
	} else if blockType == "tool_use" {
		// Handle tool call completion
		if pending, exists := state.pendingToolCalls[int(index)]; exists {
			// Send response.function_call_arguments.done event
			argsDoneEvent := map[string]interface{}{
				"type":            "response.function_call_arguments.done",
				"item_id":         pending.itemID,
				"output_index":    state.outputIndex,
				"arguments":       pending.arguments.String(),
				"sequence_number": state.nextSequenceNumber(),
			}
			sendResponsesEvent(c, argsDoneEvent, flusher)

			// Send response.output_item.done event for the function call
			itemDoneEvent := map[string]interface{}{
				"type":         "response.output_item.done",
				"item_id":      pending.itemID,
				"output_index": state.outputIndex,
				"item": map[string]interface{}{
					"type":      "function_call",
					"id":        pending.itemID,
					"call_id":   pending.itemID,
					"name":      pending.name,
					"arguments": pending.arguments.String(),
					"status":    "completed",
				},
				"sequence_number": state.nextSequenceNumber(),
			}
			sendResponsesEvent(c, itemDoneEvent, flusher)
		}
	}
}

// handleMessageDelta updates usage information
func handleMessageDelta(
	state *responsesConverterState,
	event anthropic.MessageStreamEventUnion,
	inputTokens, outputTokens int,
) (int, int, int, bool) {
	if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 || event.Usage.CacheReadInputTokens != 0 {
		state.inputTokens = event.Usage.InputTokens
		state.outputTokens = event.Usage.OutputTokens
		state.cacheTokens = event.Usage.CacheReadInputTokens
		inputTokens = int(event.Usage.InputTokens)
		outputTokens = int(event.Usage.OutputTokens)
		cacheTokens := int(event.Usage.CacheReadInputTokens)
		return inputTokens, outputTokens, cacheTokens, true
	}
	return inputTokens, outputTokens, int(state.cacheTokens), false
}

// handleMessageStop sends the response.completed event
func handleMessageStop(
	c *gin.Context,
	state *responsesConverterState,
	flusher http.Flusher,
) {
	state.finished = true

	// Build the final output array with proper message structure
	var output []map[string]interface{}

	// Add text content as a message item if present
	if state.accumulatedText != "" {
		output = append(output, map[string]interface{}{
			"id":     state.itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]interface{}{
				{
					"type": "output_text",
					"text": state.accumulatedText,
				},
			},
		})
	}

	// Add tool calls with proper structure including call_id
	for _, pending := range state.pendingToolCalls {
		output = append(output, map[string]interface{}{
			"type":      "function_call",
			"id":        pending.itemID,
			"call_id":   pending.itemID,
			"name":      pending.name,
			"arguments": pending.arguments.String(),
			"status":    "completed",
		})
	}

	// Build usage info from state
	inputTokens := state.inputTokens
	outputTokens := state.outputTokens
	cacheTokens := state.cacheTokens

	doneEvent := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":           state.responseID,
			"object":       "response",
			"created_at":   state.createdAt,
			"status":       "completed",
			"completed_at": state.createdAt, // Use same timestamp for simplicity
			"model":        "",              // Will be filled by caller if needed
			"output":       output,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
				"input_tokens_details": map[string]interface{}{
					"cached_tokens": cacheTokens,
				},
			},
		},
		"sequence_number": state.nextSequenceNumber(),
	}
	sendResponsesEvent(c, doneEvent, flusher)

	// Send final [DONE] message
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}

// sendResponsesEvent sends a single Responses API event as SSE
func sendResponsesEvent(c *gin.Context, event map[string]interface{}, flusher http.Flusher) {
	// Check if connection is still valid before writing
	if c.Writer == nil || flusher == nil {
		return
	}

	// Check if context is canceled - don't try to write
	select {
	case <-c.Request.Context().Done():
		return
	default:
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		logrus.Errorf("Failed to marshal Responses event: %v", err)
		return
	}
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", eventJSON))
	flusher.Flush()
}

// sendResponsesErrorEvent sends an error event in Responses API format
func sendResponsesErrorEvent(c *gin.Context, message string, errorType string, flusher ...http.Flusher) {
	f := http.Flusher(nil)
	if len(flusher) > 0 {
		f = flusher[0]
	}

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}
	sendResponsesEvent(c, errorEvent, f)
}

// sendFinalCompletionEvent sends the response.completed event with the current state
// This is used when the stream ends unexpectedly to ensure clients receive a completion event
func sendFinalCompletionEvent(c *gin.Context, state *responsesConverterState, flusher http.Flusher, inputTokens, outputTokens, cacheTokens int) {
	// Check if connection is still valid before writing
	if c == nil || c.Writer == nil || flusher == nil {
		logrus.Warn("Cannot send completion event: connection is nil")
		return
	}

	state.finished = true

	// Build the final output array with proper message structure
	var output []map[string]interface{}

	// Add text content as a message item if present
	if state.accumulatedText != "" {
		output = append(output, map[string]interface{}{
			"id":     state.itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]interface{}{
				{
					"type": "output_text",
					"text": state.accumulatedText,
				},
			},
		})
	}

	// Add tool calls with proper structure including call_id
	for _, pending := range state.pendingToolCalls {
		output = append(output, map[string]interface{}{
			"type":      "function_call",
			"id":        pending.itemID,
			"call_id":   pending.itemID,
			"name":      pending.name,
			"arguments": pending.arguments.String(),
			"status":    "completed",
		})
	}

	// Build usage info from state - use provided values if state values are zero
	inputTokensFinal := int(state.inputTokens)
	outputTokensFinal := int(state.outputTokens)
	cacheTokensFinal := int(state.cacheTokens)
	if inputTokensFinal == 0 && inputTokens > 0 {
		inputTokensFinal = inputTokens
	}
	if outputTokensFinal == 0 && outputTokens > 0 {
		outputTokensFinal = outputTokens
	}
	if cacheTokensFinal == 0 && cacheTokens > 0 {
		cacheTokensFinal = cacheTokens
	}

	doneEvent := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":           state.responseID,
			"object":       "response",
			"created_at":   state.createdAt,
			"status":       "completed",
			"completed_at": state.createdAt,
			"model":        "", // Will be filled by caller if needed
			"output":       output,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokensFinal,
				"output_tokens": outputTokensFinal,
				"total_tokens":  inputTokensFinal + outputTokensFinal,
				"input_tokens_details": map[string]interface{}{
					"cached_tokens": cacheTokensFinal,
				},
			},
		},
		"sequence_number": state.nextSequenceNumber(),
	}
	sendResponsesEvent(c, doneEvent, flusher)

	// Send final [DONE] message
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}
