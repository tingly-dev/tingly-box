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
) (protocol.UsageStat, error) {
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
		return protocol.ZeroUsageStat(), fmt.Errorf("streaming not supported")
	}

	// Initialize converter state
	state := newResponsesConverterState(time.Now().Unix())
	var inputTokens, outputTokens int
	var hasUsage bool

	// Process the stream
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Anthropic to Responses stream")
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

		case "content_block_start":
			handleContentBlockStart(c, state, event, flusher)

		case "content_block_delta":
			handleContentBlockDelta(c, state, event, flusher)

		case "content_block_stop":
			handleContentBlockStop(c, state, flusher)

		case "message_delta":
			inputTokens, outputTokens, hasUsage = handleMessageDelta(
				state, event, inputTokens, outputTokens,
			)

		case "message_stop":
			handleMessageStop(c, state, flusher)
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Anthropic to Responses stream canceled by client")
			if hasUsage {
				return protocol.NewUsageStat(inputTokens, outputTokens), nil
			}
			return protocol.ZeroUsageStat(), nil
		}

		if errors.Is(err, io.EOF) {
			logrus.Info("Anthropic stream ended normally (EOF)")
			if hasUsage {
				return protocol.NewUsageStat(inputTokens, outputTokens), nil
			}
			return protocol.ZeroUsageStat(), nil
		}

		logrus.Errorf("Anthropic stream error: %v", err)
		sendResponsesErrorEvent(c, err.Error(), "stream_error", flusher)
		if hasUsage {
			return protocol.NewUsageStat(inputTokens, outputTokens), err
		}
		return protocol.ZeroUsageStat(), err
	}

	if hasUsage {
		return protocol.NewUsageStat(inputTokens, outputTokens), nil
	}
	return protocol.ZeroUsageStat(), nil
}

// responsesConverterState maintains the state during stream conversion
type responsesConverterState struct {
	responseID       string
	itemID           string
	outputIndex      int
	accumulatedText  string
	inputTokens      int64
	outputTokens     int64
	finished         bool
	pendingToolCalls map[int]*pendingResponseToolCall
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
	}
}

// handleMessageStart sends the response.created event
func handleMessageStart(c *gin.Context, state *responsesConverterState, model string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":     state.responseID,
			"status": "in_progress",
			"model":  model,
			"output": []interface{}{},
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	}
	sendResponsesEvent(c, event, flusher)
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

	if blockType == "text" {
		// Handle text output
		outputEvent := map[string]interface{}{
			"type":         "response.output_item.added",
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
			"item": map[string]interface{}{
				"type":   "output_text",
				"text":   "",
				"status": "in_progress",
			},
		}
		sendResponsesEvent(c, outputEvent, flusher)
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
			"type":  "response.output_item.added",
			"item": map[string]interface{}{
				"type":   "function_call",
				"id":     toolID,
				"name":   toolName,
				"arguments": "",
				"status": "in_progress",
			},
			"output_index": state.outputIndex,
		}
		sendResponsesEvent(c, outputEvent, flusher)
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
			"type":         "response.output_text.delta",
			"delta":        text,
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
		}
		sendResponsesEvent(c, deltaEvent, flusher)
	} else if deltaType == "input_json_delta" {
		// Handle tool call arguments delta
		if pending, exists := state.pendingToolCalls[int(index)]; exists {
			argsDelta := event.Delta.PartialJSON
			pending.arguments.WriteString(argsDelta)

			deltaEvent := map[string]interface{}{
				"type":   "response.function_call_arguments.delta",
				"item_id": pending.itemID,
				"delta":  argsDelta,
			}
			sendResponsesEvent(c, deltaEvent, flusher)
		}
	}
}

// handleContentBlockStop sends the response.output_item.done event
func handleContentBlockStop(
	c *gin.Context,
	state *responsesConverterState,
	flusher http.Flusher,
) {
	doneEvent := map[string]interface{}{
		"type":         "response.output_item.done",
		"item_id":      state.itemID,
		"output_index": state.outputIndex,
	}
	sendResponsesEvent(c, doneEvent, flusher)
}

// handleMessageDelta updates usage information
func handleMessageDelta(
	state *responsesConverterState,
	event anthropic.MessageStreamEventUnion,
	inputTokens, outputTokens int,
) (int, int, bool) {
	if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
		state.inputTokens = event.Usage.InputTokens
		state.outputTokens = event.Usage.OutputTokens
		inputTokens = int(event.Usage.InputTokens)
		outputTokens = int(event.Usage.OutputTokens)
		return inputTokens, outputTokens, true
	}
	return inputTokens, outputTokens, false
}

// handleMessageStop sends the response.done event
func handleMessageStop(
	c *gin.Context,
	state *responsesConverterState,
	flusher http.Flusher,
) {
	state.finished = true

	// Build the final output array
	var output []map[string]interface{}

	// Add text content if present
	if state.accumulatedText != "" {
		output = append(output, map[string]interface{}{
			"type":   "output_text",
			"text":   state.accumulatedText,
			"status": "completed",
		})
	}

	// Add tool calls
	for _, pending := range state.pendingToolCalls {
		output = append(output, map[string]interface{}{
			"type":      "function_call",
			"id":        pending.itemID,
			"name":      pending.name,
			"arguments": pending.arguments.String(),
			"status":    "completed",
		})
	}

	// Build usage info from state
	inputTokens := state.inputTokens
	outputTokens := state.outputTokens

	doneEvent := map[string]interface{}{
		"type": "response.done",
		"response": map[string]interface{}{
			"id":     state.responseID,
			"status": "completed",
			"output": output,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
			},
		},
	}
	sendResponsesEvent(c, doneEvent, flusher)

	// Send final [DONE] message
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}

// sendResponsesEvent sends a single Responses API event as SSE
func sendResponsesEvent(c *gin.Context, event map[string]interface{}, flusher http.Flusher) {
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
