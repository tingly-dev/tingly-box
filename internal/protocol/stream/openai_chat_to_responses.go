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

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// chatToResponsesState tracks the streaming conversion state from Chat Completions to Responses API
type chatToResponsesState struct {
	responseID       string
	outputIndex      int
	pendingToolCalls map[int]*pendingToolCallResponse
	accumulatedText  strings.Builder
	inputTokens      int64
	outputTokens     int64
	hasSentCreated   bool
}

// pendingToolCallResponse tracks a tool call being assembled from stream chunks
type pendingToolCallResponse struct {
	itemID    string
	name      string
	arguments strings.Builder
}

// HandleOpenAIChatToResponsesStream converts OpenAI Chat Completions streaming to Responses API format.
// Returns UsageStat containing token usage information for tracking.
func HandleOpenAIChatToResponsesStream(c *gin.Context, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (protocol.UsageStat, error) {
	logrus.Info("Starting OpenAI Chat to Responses streaming conversion handler")
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
		return protocol.ZeroUsageStat(), errors.New("streaming not supported by this connection")
	}

	// Initialize conversion state
	state := &chatToResponsesState{
		responseID:       fmt.Sprintf("resp_%d", time.Now().Unix()),
		outputIndex:      0,
		pendingToolCalls: make(map[int]*pendingToolCallResponse),
		inputTokens:      0,
		outputTokens:     0,
		hasSentCreated:   false,
	}

	// Track text and usage for final completion
	var finishReason string
	var hasUsage bool

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

		// Skip empty chunks
		if len(chunk.Choices) == 0 {
			return true
		}

		choice := chunk.Choices[0]

		// Handle content delta
		if choice.Delta.Content != "" {
			state.accumulatedText.WriteString(choice.Delta.Content)
			sendResponsesOutputTextDelta(c, choice.Delta.Content, state.outputIndex, flusher)
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
					toolOutputIndex := 1
					if state.accumulatedText.Len() == 0 {
						// No text content, so this tool call gets index 0
						toolOutputIndex = state.outputIndex
						state.outputIndex++
					} else {
						// Text has index 0, tool calls start from 1
						toolOutputIndex = state.outputIndex
						state.outputIndex++
					}

					state.pendingToolCalls[openaiIndex] = &pendingToolCallResponse{
						itemID:    itemID,
						name:      toolCall.Function.Name,
						arguments: strings.Builder{},
					}

					// Send output_item.added event
					sendResponsesOutputItemAdded(c, itemID, toolCall.Function.Name, toolOutputIndex, flusher)
				}

				// Accumulate and send argument deltas
				if toolCall.Function.Arguments != "" {
					ptc := state.pendingToolCalls[openaiIndex]
					ptc.arguments.WriteString(toolCall.Function.Arguments)
					sendResponsesFunctionCallArgumentsDelta(c, ptc.itemID, toolCall.Function.Arguments, flusher)
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
			return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
		}
		logrus.Errorf("Chat to Responses stream error: %v", err)

		// Send error event
		errorEvent := map[string]interface{}{
			"type":  "error",
			"error": map[string]interface{}{"message": err.Error(), "type": "stream_error"},
		}
		errorJSON, _ := json.Marshal(errorEvent)
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(errorJSON)))
		flusher.Flush()

		return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), err
	}

	return protocol.NewUsageStat(int(state.inputTokens), int(state.outputTokens)), nil
}

// sendResponsesCreatedEvent sends the response.created event
func sendResponsesCreatedEvent(c *gin.Context, state *chatToResponsesState, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":     state.responseID,
			"status": "in_progress",
			"status_details": map[string]interface{}{
				"type": "in_progress",
			},
			"output": []interface{}{},
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesOutputTextDelta sends response.output_text.delta event
func sendResponsesOutputTextDelta(c *gin.Context, delta string, outputIndex int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":         "response.output_text.delta",
		"delta":        delta,
		"output_index": outputIndex,
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesOutputItemAdded sends response.output_item.added event for tool calls
func sendResponsesOutputItemAdded(c *gin.Context, itemID, name string, outputIndex int, flusher http.Flusher) {
	event := map[string]interface{}{
		"type": "response.output_item.added",
		"item": map[string]interface{}{
			"id":        itemID,
			"type":      "function_call",
			"name":      name,
			"arguments": "",
		},
		"output_index": outputIndex,
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesFunctionCallArgumentsDelta sends response.function_call_arguments.delta event
func sendResponsesFunctionCallArgumentsDelta(c *gin.Context, itemID, delta string, flusher http.Flusher) {
	event := map[string]interface{}{
		"type":   "response.function_call_arguments.delta",
		"item_id": itemID,
		"delta":  delta,
	}
	sendChatToResponsesEvent(c, event, flusher)
}

// sendResponsesCompletedEvent sends the response.completed event
func sendResponsesCompletedEvent(c *gin.Context, state *chatToResponsesState, model, finishReason string, flusher http.Flusher) {
	// Build output array
	var output []interface{}

	// Add text content if present
	if state.accumulatedText.Len() > 0 {
		output = append(output, map[string]interface{}{
			"type":         "output_text",
			"text":         state.accumulatedText.String(),
			"output_index": 0,
		})
	}

	// Add tool calls
	// Rebuild output_index for tool calls
	toolIndex := 0
	if state.accumulatedText.Len() > 0 {
		toolIndex = 1
	}

	for _, ptc := range state.pendingToolCalls {
		output = append(output, map[string]interface{}{
			"type":         "function_call",
			"id":           ptc.itemID,
			"name":         ptc.name,
			"arguments":    ptc.arguments.String(),
			"output_index": toolIndex,
		})
		toolIndex++
	}

	event := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":     state.responseID,
			"status": "completed",
			"output": output,
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": state.outputTokens,
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
