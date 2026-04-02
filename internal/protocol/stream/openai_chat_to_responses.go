package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// chatToResponsesState tracks the streaming conversion state from Chat Completions to Responses API
type chatToResponsesState struct {
	responseID       string
	createdAt        int64
	sequenceNumber   int64
	outputIndex      int
	textItemID       string
	hasTextItem      bool
	pendingToolCalls map[int]*pendingToolCallResponse
	accumulatedText  strings.Builder
	inputTokens      int64
	outputTokens     int64
	cacheTokens      int64 // Cached tokens from prompt
	hasSentCreated   bool
}

// pendingToolCallResponse tracks a tool call being assembled from stream chunks
type pendingToolCallResponse struct {
	itemID    string
	callID    string
	outputIdx int
	name      string
	arguments strings.Builder
}

// HandleOpenAIChatToResponsesStream converts OpenAI Chat Completions streaming to Responses API format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIChatToResponsesStream(c *gin.Context, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	logrus.Debug("Starting OpenAI Chat to Responses streaming conversion handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Chat to Responses streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Chat Completions stream: %v", err)
			}
		}
		logrus.Info("Finished Chat to Responses streaming conversion handler")
	}()

	// Set SSE headers for Responses API
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported by this connection")
	}

	// Initialize conversion state
	state := &chatToResponsesState{
		responseID:       fmt.Sprintf("resp_%d", time.Now().Unix()),
		createdAt:        time.Now().Unix(),
		sequenceNumber:   0,
		outputIndex:      0,
		textItemID:       fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		hasTextItem:      false,
		pendingToolCalls: make(map[int]*pendingToolCallResponse),
		inputTokens:      0,
		outputTokens:     0,
		hasSentCreated:   false,
	}

	// Track text and usage for final completion
	var finishReason string
	var hasUsage bool
	completedSent := false

	// Process the stream
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Chat to Responses stream")
			return false
		default:
		}

		// Try to get next chunk
		if !stream.Next() {
			return false
		}

		chunk := stream.Current()

		// Send response.created on first meaningful chunk
		if !state.hasSentCreated {
			sendResponsesCreatedEvent(c, state, flusher)
			state.hasSentCreated = true
		}

		// Track usage from chunks
		if chunk.Usage.PromptTokens != 0 {
			state.inputTokens = int64(chunk.Usage.PromptTokens)
			hasUsage = true
		}
		if chunk.Usage.CompletionTokens != 0 {
			state.outputTokens = int64(chunk.Usage.CompletionTokens)
			hasUsage = true
		}
		// Track cache tokens from prompt tokens details if available
		if chunk.Usage.PromptTokensDetails.CachedTokens != 0 {
			state.cacheTokens = int64(chunk.Usage.PromptTokensDetails.CachedTokens)
			hasUsage = true
		}

		// Skip empty chunks
		if len(chunk.Choices) == 0 {
			return true
		}

		choice := chunk.Choices[0]

		// Handle content delta
		if choice.Delta.Content != "" {
			if !state.hasTextItem {
				sendResponsesOutputTextItemAdded(c, state, flusher)
				state.hasTextItem = true
			}
			state.accumulatedText.WriteString(choice.Delta.Content)
			sendResponsesOutputTextDelta(c, state, choice.Delta.Content, flusher)
		}

		// Handle tool_calls delta
		if len(choice.Delta.ToolCalls) > 0 {
			for _, toolCall := range choice.Delta.ToolCalls {
				openaiIndex := int(toolCall.Index)

				// Check if this is a new tool call
				if _, exists := state.pendingToolCalls[openaiIndex]; !exists {
					// Generate item_id for Responses API
					itemID := fmt.Sprintf("fc_%d_%d", time.Now().Unix(), openaiIndex)
					if toolCall.ID != "" {
						// Use OpenAI's ID if available (may need truncation)
						itemID = truncateToolCallID(toolCall.ID)
					}

					// Start a new output_index for this tool call
					// Text gets index 0, tool calls start from 1
					toolOutputIndex := state.outputIndex
					state.outputIndex++

					state.pendingToolCalls[openaiIndex] = &pendingToolCallResponse{
						itemID:    itemID,
						callID:    toolCall.ID,
						outputIdx: toolOutputIndex,
						name:      toolCall.Function.Name,
						arguments: strings.Builder{},
					}

					// Send output_item.added event
					sendResponsesOutputItemAdded(c, state, itemID, toolCall.ID, toolCall.Function.Name, toolOutputIndex, flusher)
				}

				// Accumulate and send argument deltas
				if toolCall.Function.Arguments != "" {
					ptc := state.pendingToolCalls[openaiIndex]
					ptc.arguments.WriteString(toolCall.Function.Arguments)
					sendResponsesFunctionCallArgumentsDelta(c, state, ptc.itemID, ptc.outputIdx, toolCall.Function.Arguments, flusher)
				}
			}
		}

		// Check for completion
		if choice.FinishReason != "" {
			finishReason = string(choice.FinishReason)

			// If no usage was provided, estimate it
			if !hasUsage {
				// Estimate tokens (would need request context for better estimation)
				// For now, use what we accumulated
				if state.outputTokens == 0 {
					// Rough estimation: ~4 chars per token
					state.outputTokens = int64(state.accumulatedText.Len() / 4)
					for _, ptc := range state.pendingToolCalls {
						state.outputTokens += int64(ptc.arguments.Len() / 4)
					}
				}
			}

			// Send response.completed event
			sendResponsesCompletedEvent(c, state, responseModel, finishReason, flusher)
			completedSent = true

			// Send final [DONE] message
			c.Writer.WriteString("data: [DONE]\n\n")
			flusher.Flush()

			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Chat to Responses stream canceled by client")
			return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
		}
		logrus.Errorf("Chat to Responses stream error: %v", err)

		// Send error event
		errorEvent := map[string]interface{}{
			"type":            "error",
			"sequence_number": nextSequenceNumber(state),
			"error":           map[string]interface{}{"message": err.Error(), "type": "stream_error"},
		}
		errorJSON, _ := json.Marshal(errorEvent)
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(errorJSON)))
		flusher.Flush()

		return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), err
	}

	// Some providers end the stream without emitting a final chunk with finish_reason.
	// Ensure clients still receive response.completed and [DONE].
	if !completedSent {
		if !state.hasSentCreated {
			sendResponsesCreatedEvent(c, state, flusher)
			state.hasSentCreated = true
		}

		if finishReason == "" {
			finishReason = "stop"
		}

		sendResponsesCompletedEvent(c, state, responseModel, finishReason, flusher)
		c.Writer.WriteString("data: [DONE]\n\n")
		flusher.Flush()
	}

	return protocol.NewTokenUsageWithCache(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens)), nil
}

// sendResponsesCreatedEvent sends the response.created event
func sendResponsesCreatedEvent(c *gin.Context, state *chatToResponsesState, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":            "response.created",
		"sequence_number": nextSequenceNumber(state),
		"response": map[string]interface{}{
			"id":         state.responseID,
			"object":     "response",
			"created_at": state.createdAt,
			"status":     "in_progress",
			"output":     []interface{}{},
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": state.outputTokens,
				"total_tokens":  state.inputTokens + state.outputTokens,
				"input_tokens_details": map[string]interface{}{
					"cached_tokens": state.cacheTokens,
				},
				"output_tokens_details": map[string]interface{}{
					"reasoning_tokens": 0,
				},
			},
		},
	}
	sendChatToResponsesEvent(c, event, flusher)
}

func sendResponsesOutputTextItemAdded(c *gin.Context, state *chatToResponsesState, flusher http.Flusher) {
	if state.outputIndex == 0 {
		state.outputIndex = 1
	}
	event := map[string]interface{}{
		"type":            "response.output_item.added",
		"sequence_number": nextSequenceNumber(state),
		"output_index":    0,
		"item": map[string]interface{}{
			"id":     state.textItemID,
			"type":   "message",
			"role":   "assistant",
			"status": "in_progress",
			"content": []map[string]interface{}{
				{
					"type":        "output_text",
					"text":        "",
					"annotations": []interface{}{},
				},
			},
		},
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesOutputTextDelta sends response.output_text.delta event
func sendResponsesOutputTextDelta(c *gin.Context, state *chatToResponsesState, delta string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":            "response.output_text.delta",
		"sequence_number": nextSequenceNumber(state),
		"item_id":         state.textItemID,
		"output_index":    0,
		"content_index":   0,
		"delta":           delta,
		"logprobs":        []interface{}{},
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesOutputItemAdded sends response.output_item.added event for tool calls
func sendResponsesOutputItemAdded(c *gin.Context, state *chatToResponsesState, itemID, callID, name string, outputIndex int, flusher http.Flusher) {
	if callID == "" {
		callID = itemID
	}
	event := map[string]interface{}{
		"type":            "response.output_item.added",
		"sequence_number": nextSequenceNumber(state),
		"item": map[string]interface{}{
			"id":        itemID,
			"call_id":   callID,
			"type":      "function_call",
			"name":      name,
			"arguments": "",
			"status":    "in_progress",
		},
		"output_index": outputIndex,
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesFunctionCallArgumentsDelta sends response.function_call_arguments.delta event
func sendResponsesFunctionCallArgumentsDelta(c *gin.Context, state *chatToResponsesState, itemID string, outputIndex int, delta string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":            "response.function_call_arguments.delta",
		"sequence_number": nextSequenceNumber(state),
		"item_id":         itemID,
		"output_index":    outputIndex,
		"delta":           delta,
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesCompletedEvent sends the response.completed event
func sendResponsesCompletedEvent(c *gin.Context, state *chatToResponsesState, model, finishReason string, flusher http.Flusher) {
	if state.hasTextItem {
		textDone := map[string]interface{}{
			"type":            "response.output_text.done",
			"sequence_number": nextSequenceNumber(state),
			"item_id":         state.textItemID,
			"output_index":    0,
			"content_index":   0,
			"text":            state.accumulatedText.String(),
			"logprobs":        []interface{}{},
		}
		sendChatToResponsesEvent(c, textDone, flusher)

		textItemDone := map[string]interface{}{
			"type":            "response.output_item.done",
			"sequence_number": nextSequenceNumber(state),
			"output_index":    0,
			"item": map[string]interface{}{
				"id":     state.textItemID,
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]interface{}{
					{
						"type":        "output_text",
						"text":        state.accumulatedText.String(),
						"annotations": []interface{}{},
					},
				},
			},
		}
		sendChatToResponsesEvent(c, textItemDone, flusher)
	}

	sortedIndexes := make([]int, 0, len(state.pendingToolCalls))
	for idx := range state.pendingToolCalls {
		sortedIndexes = append(sortedIndexes, idx)
	}
	sort.Ints(sortedIndexes)

	for _, idx := range sortedIndexes {
		ptc := state.pendingToolCalls[idx]
		callID := ptc.callID
		if callID == "" {
			callID = ptc.itemID
		}
		argumentsDone := map[string]interface{}{
			"type":            "response.function_call_arguments.done",
			"sequence_number": nextSequenceNumber(state),
			"item_id":         ptc.itemID,
			"output_index":    ptc.outputIdx,
			"name":            ptc.name,
			"arguments":       ptc.arguments.String(),
		}
		sendChatToResponsesEvent(c, argumentsDone, flusher)

		itemDone := map[string]interface{}{
			"type":            "response.output_item.done",
			"sequence_number": nextSequenceNumber(state),
			"output_index":    ptc.outputIdx,
			"item": map[string]interface{}{
				"id":        ptc.itemID,
				"call_id":   callID,
				"type":      "function_call",
				"name":      ptc.name,
				"arguments": ptc.arguments.String(),
				"status":    "completed",
			},
		}
		sendChatToResponsesEvent(c, itemDone, flusher)
	}

	// Build output array
	var output []interface{}

	// Add text content if present
	if state.accumulatedText.Len() > 0 {
		output = append(output, map[string]interface{}{
			"id":     state.textItemID,
			"type":   "message",
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]interface{}{
				{
					"type":        "output_text",
					"text":        state.accumulatedText.String(),
					"annotations": []interface{}{},
				},
			},
		})
	}

	// Add tool calls
	for _, idx := range sortedIndexes {
		ptc := state.pendingToolCalls[idx]
		callID := ptc.callID
		if callID == "" {
			callID = ptc.itemID
		}
		output = append(output, map[string]interface{}{
			"type":      "function_call",
			"id":        ptc.itemID,
			"call_id":   callID,
			"name":      ptc.name,
			"arguments": ptc.arguments.String(),
			"status":    "completed",
		})
	}

	event := map[string]interface{}{
		"type":            "response.completed",
		"sequence_number": nextSequenceNumber(state),
		"response": map[string]interface{}{
			"id":         state.responseID,
			"object":     "response",
			"created_at": state.createdAt,
			"status":     "completed",
			"output":     output,
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": state.outputTokens,
				"total_tokens":  state.inputTokens + state.outputTokens,
				"input_tokens_details": map[string]interface{}{
					"cached_tokens": state.cacheTokens,
				},
				"output_tokens_details": map[string]interface{}{
					"reasoning_tokens": 0,
				},
			},
		},
	}

	// Add model if provided
	if model != "" {
		event["response"].(map[string]interface{})["model"] = model
	}

	sendChatToResponsesEvent(c, event, flusher)
}

// sendChatToResponsesEvent sends an event in Responses API SSE format (specific to Chat → Responses conversion)
func sendChatToResponsesEvent(c *gin.Context, event map[string]interface{}, flusher http.Flusher) {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		logrus.Errorf("Failed to marshal Responses event: %v", err)
		return
	}
	// Responses API SSE format: data: <json>\n\n
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(eventJSON)))
	flusher.Flush()
}

func nextSequenceNumber(state *chatToResponsesState) int64 {
	state.sequenceNumber++
	return state.sequenceNumber
}
