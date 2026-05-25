package stream

import (
	"context"
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
	reasoningTokens  int64 // Reasoning tokens from output
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
	logrus.WithContext(c.Request.Context()).Debug("Starting OpenAI Chat to Responses streaming conversion handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(c.Request.Context()).Errorf("Panic in Chat to Responses streaming handler: %v", r)
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
				logrus.WithContext(c.Request.Context()).Errorf("Error closing Chat Completions stream: %v", err)
			}
		}
		logrus.WithContext(c.Request.Context()).Info("Finished Chat to Responses streaming conversion handler")
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
	StreamLoop(c, func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Chat to Responses stream")
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
		// Track reasoning tokens from completion tokens details if available
		if chunk.Usage.CompletionTokensDetails.ReasoningTokens != 0 {
			state.reasoningTokens = int64(chunk.Usage.CompletionTokensDetails.ReasoningTokens)
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
			OpenAISSEDone(c)
			return false
		}

		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Chat to Responses stream canceled by client")
			return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Chat to Responses stream error: %v", err)

		errorEvent := responsesStreamErrorEvent{
			Type:           "error",
			SequenceNumber: nextSequenceNumber(state),
			Error: responsesStreamErrorBody{
				Message: err.Error(),
				Type:    "stream_error",
			},
		}
		OpenAIResponsesEvent(c, errorEvent.EventType(), errorEvent)

		return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), err
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
		OpenAISSEDone(c)
	}

	return protocol.NewTokenUsageFull(int(state.inputTokens), int(state.outputTokens), int(state.cacheTokens), int(state.reasoningTokens)), nil
}

// sendResponsesCreatedEvent sends the response.created event
func sendResponsesCreatedEvent(c *gin.Context, state *chatToResponsesState, flusher http.Flusher) {
	event := responsesCreatedEvent{
		Type:           "response.created",
		SequenceNumber: nextSequenceNumber(state),
		Response:       newResponsesWireResponse(state, "in_progress", nil, ""),
	}
	OpenAIResponsesEvent(c, event.EventType(), event)
}

func sendResponsesOutputTextItemAdded(c *gin.Context, state *chatToResponsesState, flusher http.Flusher) {
	if state.outputIndex == 0 {
		state.outputIndex = 1
	}
	event := responsesOutputItemAddedEvent{
		Type:           "response.output_item.added",
		SequenceNumber: nextSequenceNumber(state),
		OutputIndex:    0,
		Item:           newResponsesMessageItem(state.textItemID, "in_progress", ""),
	}
	OpenAIResponsesEvent(c, event.EventType(), event)
}

// sendResponsesOutputTextDelta sends response.output_text.delta event
func sendResponsesOutputTextDelta(c *gin.Context, state *chatToResponsesState, delta string, flusher http.Flusher) {
	event := responsesOutputTextDeltaEvent{
		Type:           "response.output_text.delta",
		SequenceNumber: nextSequenceNumber(state),
		ItemID:         state.textItemID,
		OutputIndex:    0,
		ContentIndex:   0,
		Delta:          delta,
		Logprobs:       []interface{}{},
	}
	OpenAIResponsesEvent(c, event.EventType(), event)
}

// sendResponsesOutputItemAdded sends response.output_item.added event for tool calls
func sendResponsesOutputItemAdded(c *gin.Context, state *chatToResponsesState, itemID, callID, name string, outputIndex int, flusher http.Flusher) {
	if callID == "" {
		callID = itemID
	}
	event := responsesOutputItemAddedEvent{
		Type:           "response.output_item.added",
		SequenceNumber: nextSequenceNumber(state),
		OutputIndex:    outputIndex,
		Item:           newResponsesFunctionCallItem(itemID, callID, name, "", "in_progress"),
	}
	OpenAIResponsesEvent(c, event.EventType(), event)
}

// sendResponsesFunctionCallArgumentsDelta sends response.function_call_arguments.delta event
func sendResponsesFunctionCallArgumentsDelta(c *gin.Context, state *chatToResponsesState, itemID string, outputIndex int, delta string, flusher http.Flusher) {
	event := responsesFunctionCallArgumentsDeltaEvent{
		Type:           "response.function_call_arguments.delta",
		SequenceNumber: nextSequenceNumber(state),
		ItemID:         itemID,
		OutputIndex:    outputIndex,
		Delta:          delta,
	}
	OpenAIResponsesEvent(c, event.EventType(), event)
}

// sendResponsesCompletedEvent sends the response.completed event
func sendResponsesCompletedEvent(c *gin.Context, state *chatToResponsesState, model, finishReason string, flusher http.Flusher) {
	if state.hasTextItem {
		text := state.accumulatedText.String()
		textDone := responsesOutputTextDoneEvent{
			Type:           "response.output_text.done",
			SequenceNumber: nextSequenceNumber(state),
			ItemID:         state.textItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Text:           text,
			Logprobs:       []interface{}{},
		}
		OpenAIResponsesEvent(c, textDone.EventType(), textDone)

		textItemDone := responsesOutputItemDoneEvent{
			Type:           "response.output_item.done",
			SequenceNumber: nextSequenceNumber(state),
			OutputIndex:    0,
			Item:           newResponsesMessageItem(state.textItemID, "completed", text),
		}
		OpenAIResponsesEvent(c, textItemDone.EventType(), textItemDone)
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
		arguments := ptc.arguments.String()
		argumentsDone := responsesFunctionCallArgumentsDoneEvent{
			Type:           "response.function_call_arguments.done",
			SequenceNumber: nextSequenceNumber(state),
			ItemID:         ptc.itemID,
			OutputIndex:    ptc.outputIdx,
			Name:           ptc.name,
			Arguments:      arguments,
		}
		OpenAIResponsesEvent(c, argumentsDone.EventType(), argumentsDone)

		itemDone := responsesOutputItemDoneEvent{
			Type:           "response.output_item.done",
			SequenceNumber: nextSequenceNumber(state),
			OutputIndex:    ptc.outputIdx,
			Item:           newResponsesFunctionCallItem(ptc.itemID, callID, ptc.name, arguments, "completed"),
		}
		OpenAIResponsesEvent(c, itemDone.EventType(), itemDone)
	}

	var output []responsesOutputItemWire
	if state.accumulatedText.Len() > 0 {
		output = append(output, newResponsesMessageItem(state.textItemID, "completed", state.accumulatedText.String()))
	}

	for _, idx := range sortedIndexes {
		ptc := state.pendingToolCalls[idx]
		callID := ptc.callID
		if callID == "" {
			callID = ptc.itemID
		}
		output = append(output, newResponsesFunctionCallItem(ptc.itemID, callID, ptc.name, ptc.arguments.String(), "completed"))
	}

	event := responsesCompletedEvent{
		Type:           "response.completed",
		SequenceNumber: nextSequenceNumber(state),
		Response:       newResponsesWireResponse(state, "completed", output, model),
	}

	OpenAIResponsesEvent(c, event.EventType(), event)
}

func newResponsesWireResponse(state *chatToResponsesState, status string, output []responsesOutputItemWire, model string) responsesWireResponse {
	if output == nil {
		output = []responsesOutputItemWire{}
	}
	return responsesWireResponse{
		ID:        state.responseID,
		Object:    "response",
		CreatedAt: state.createdAt,
		Status:    status,
		Output:    output,
		Usage: &responsesUsageWire{
			InputTokens:  state.inputTokens,
			OutputTokens: state.outputTokens,
			TotalTokens:  state.inputTokens + state.outputTokens,
			InputTokensDetails: responsesInputTokensDetailsWire{
				CachedTokens: state.cacheTokens,
			},
			OutputTokensDetails: responsesOutputTokensDetailsWire{
				ReasoningTokens: state.reasoningTokens,
			},
		},
		Model: model,
	}
}

func newResponsesMessageItem(itemID, status, text string) responsesOutputItemWire {
	return responsesOutputItemWire{
		ID:     itemID,
		Type:   "message",
		Role:   "assistant",
		Status: status,
		Content: []responsesContentPartWire{
			{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			},
		},
	}
}

func newResponsesFunctionCallItem(itemID, callID, name, arguments, status string) responsesOutputItemWire {
	return responsesOutputItemWire{
		ID:        itemID,
		CallID:    callID,
		Type:      "function_call",
		Name:      name,
		Arguments: &arguments,
		Status:    status,
	}
}
