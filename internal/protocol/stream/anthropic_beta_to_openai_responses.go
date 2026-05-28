package stream

import (
	"context"
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

// HandleAnthropicBetaToOpenAIResponsesStream converts Anthropic streaming events
// to OpenAI Responses API format.
//
// Returns (UsageStat, error) for usage tracking and error handling.
func HandleAnthropicBetaToOpenAIResponsesStream(
	hc *protocol.HandleContext,
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	responseModel string,
) (*protocol.TokenUsage, error) {
	logrus.WithContext(hc.GinContext.Request.Context()).Info("Starting Anthropic to OpenAI Responses streaming converter")
	defer func() {
		if r := recover(); r != nil {
			logrus.WithContext(hc.GinContext.Request.Context()).Errorf("Panic in Anthropic to Responses converter: %v", r)
			if hc.GinContext.Writer != nil {
				hc.GinContext.Writer.WriteHeader(http.StatusInternalServerError)
				sendResponsesErrorEvent(hc.GinContext, "Internal streaming error", "internal_error")
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.WithContext(hc.GinContext.Request.Context()).Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.WithContext(hc.GinContext.Request.Context()).Info("Finished Anthropic to Responses converter")
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
	StreamLoop(c, func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.WithContext(c.Request.Context()).Debug("Client disconnected, stopping Anthropic to Responses stream")
			// Send completion event before returning since client is disconnecting
			if !completedSent && !state.finished {
				logrus.WithContext(c.Request.Context()).Info("Client disconnected, sending completion event before close")
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
		// Pre-content failure: nothing reached the client yet (not a cancel
		// or normal EOF). Surface a retryable 5xx instead of synthesizing a
		// completion event, so mid-request failover can try the next tier.
		if !c.Writer.Written() && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Errorf("Anthropic to Responses pre-stream error: %v", err)
			SendStreamingError(c, err)
			return protocol.ZeroTokenUsage(), err
		}

		// Only send completion event if not already sent
		if !completedSent && !state.finished {
			// Send completion event for all errors including context.Canceled
			// The Stream loop's context check may not have run if stream.Next() was blocking
			logrus.WithContext(c.Request.Context()).WithError(err).Warn("Stream error occurred, sending completion event")
			sendFinalCompletionEvent(c, state, flusher, inputTokens, outputTokens, cacheTokens)
			completedSent = true
		}

		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to Responses stream canceled by client")
			if hasUsage {
				return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
			}
			return protocol.ZeroTokenUsage(), nil
		}

		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			if hasUsage {
				return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
			}
			return protocol.ZeroTokenUsage(), nil
		}

		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
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
	resp := newResponsesWireResponseFromState(state, "in_progress", nil)
	resp.Model = model
	resp.Usage = nil

	event := responsesCreatedEvent{
		Type:           "response.created",
		SequenceNumber: int64(state.nextSequenceNumber()),
		Response:       resp,
	}
	sendResponsesEvent(c, event, flusher)

	// Also send response.in_progress event as per the real API
	inProgressResp := newResponsesWireResponseFromState(state, "in_progress", nil)
	inProgressResp.Model = model
	inProgressResp.Usage = nil

	inProgressEvent := responsesInProgressEvent{
		Type:           "response.in_progress",
		SequenceNumber: int64(state.nextSequenceNumber()),
		Response:       inProgressResp,
	}
	sendResponsesEvent(c, inProgressEvent, flusher)
}

// handleContentBlockStart sends the response.output_item.added event
func handleContentBlockStart(
	c *gin.Context,
	state *responsesConverterState,
	event anthropic.BetaRawMessageStreamEventUnion,
	flusher http.Flusher,
) {
	index := event.Index
	blockType := event.ContentBlock.Type
	state.currentBlockType = blockType

	if blockType == "text" {
		// Handle text output - send response.output_item.added with message type
		outputEvent := responsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(state.nextSequenceNumber()),
			OutputIndex:    state.outputIndex,
			Item: responsesOutputItemWire{
				ID:      state.itemID,
				Type:    "message",
				Status:  "in_progress",
				Role:    "assistant",
				Content: []responsesContentPartWire{},
			},
		}
		sendResponsesEvent(c, outputEvent, flusher)

		// Also send response.content_part.added for the text part
		contentPartEvent := responsesContentPartAddedEvent{
			Type:           "response.content_part.added",
			SequenceNumber: int64(state.nextSequenceNumber()),
			ItemID:         state.itemID,
			OutputIndex:    state.outputIndex,
			ContentIndex:   0,
			Part: responsesContentPartWire{
				Type: "output_text",
				Text: "",
			},
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

		arguments := ""
		outputEvent := responsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(state.nextSequenceNumber()),
			OutputIndex:    state.outputIndex,
			Item: responsesOutputItemWire{
				Type:      "function_call",
				ID:        toolID,
				CallID:    toolID,
				Name:      toolName,
				Arguments: &arguments,
				Status:    "in_progress",
			},
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
	event anthropic.BetaRawMessageStreamEventUnion,
	flusher http.Flusher,
) {
	deltaType := event.Delta.Type
	index := event.Index

	if deltaType == "text_delta" {
		// Handle text delta
		text := event.Delta.Text
		state.accumulatedText += text

		deltaEvent := responsesOutputTextDeltaEvent{
			Type:           "response.output_text.delta",
			Delta:          text,
			ItemID:         state.itemID,
			OutputIndex:    state.outputIndex,
			ContentIndex:   0,
			SequenceNumber: int64(state.nextSequenceNumber()),
		}
		sendResponsesEvent(c, deltaEvent, flusher)
	} else if deltaType == "input_json_delta" {
		// Handle tool call arguments delta
		if pending, exists := state.pendingToolCalls[int(index)]; exists {
			argsDelta := event.Delta.PartialJSON
			pending.arguments.WriteString(argsDelta)

			deltaEvent := responsesFunctionCallArgumentsDeltaEvent{
				Type:           "response.function_call_arguments.delta",
				Delta:          argsDelta,
				ItemID:         pending.itemID,
				OutputIndex:    state.outputIndex,
				SequenceNumber: int64(state.nextSequenceNumber()),
			}
			sendResponsesEvent(c, deltaEvent, flusher)
		}
	}
}

// handleContentBlockStop sends the appropriate completion events based on block type
func handleContentBlockStop(
	c *gin.Context,
	state *responsesConverterState,
	event anthropic.BetaRawMessageStreamEventUnion,
	flusher http.Flusher,
) {
	index := event.Index
	blockType := state.currentBlockType

	if blockType == "text" {
		// Send response.output_text.done event
		textDoneEvent := responsesOutputTextDoneEvent{
			Type:           "response.output_text.done",
			ItemID:         state.itemID,
			OutputIndex:    state.outputIndex,
			ContentIndex:   0,
			Text:           state.accumulatedText,
			SequenceNumber: int64(state.nextSequenceNumber()),
		}
		sendResponsesEvent(c, textDoneEvent, flusher)

		// Send response.content_part.done event
		contentPartDoneEvent := responsesContentPartDoneEvent{
			Type:           "response.content_part.done",
			SequenceNumber: int64(state.nextSequenceNumber()),
			ItemID:         state.itemID,
			OutputIndex:    state.outputIndex,
			ContentIndex:   0,
			Part: responsesContentPartWire{
				Type: "output_text",
				Text: state.accumulatedText,
			},
		}
		sendResponsesEvent(c, contentPartDoneEvent, flusher)

		// Send response.output_item.done event
		itemDoneEvent := responsesOutputItemDoneEvent{
			Type:           "response.output_item.done",
			SequenceNumber: int64(state.nextSequenceNumber()),
			OutputIndex:    state.outputIndex,
			Item: responsesOutputItemWire{
				ID:     state.itemID,
				Type:   "message",
				Status: "completed",
				Role:   "assistant",
				Content: []responsesContentPartWire{
					{
						Type: "output_text",
						Text: state.accumulatedText,
					},
				},
			},
		}
		sendResponsesEvent(c, itemDoneEvent, flusher)
	} else if blockType == "tool_use" {
		// Handle tool call completion
		if pending, exists := state.pendingToolCalls[int(index)]; exists {
			// Send response.function_call_arguments.done event
			argsDoneEvent := responsesFunctionCallArgumentsDoneEvent{
				Type:           "response.function_call_arguments.done",
				ItemID:         pending.itemID,
				OutputIndex:    state.outputIndex,
				Arguments:      pending.arguments.String(),
				SequenceNumber: int64(state.nextSequenceNumber()),
			}
			sendResponsesEvent(c, argsDoneEvent, flusher)

			argumentsStr := pending.arguments.String()
			// Send response.output_item.done event for the function call
			itemDoneEvent := responsesOutputItemDoneEvent{
				Type:           "response.output_item.done",
				SequenceNumber: int64(state.nextSequenceNumber()),
				OutputIndex:    state.outputIndex,
				Item: responsesOutputItemWire{
					Type:      "function_call",
					ID:        pending.itemID,
					CallID:    pending.itemID,
					Name:      pending.name,
					Arguments: &argumentsStr,
					Status:    "completed",
				},
			}
			sendResponsesEvent(c, itemDoneEvent, flusher)
		}
	}
}

// handleMessageDelta updates usage information
func handleMessageDelta(
	state *responsesConverterState,
	event anthropic.BetaRawMessageStreamEventUnion,
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
	var output []responsesOutputItemWire

	// Add text content as a message item if present
	if state.accumulatedText != "" {
		output = append(output, responsesOutputItemWire{
			ID:     state.itemID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []responsesContentPartWire{
				{
					Type: "output_text",
					Text: state.accumulatedText,
				},
			},
		})
	}

	// Add tool calls with proper structure including call_id
	for _, pending := range state.pendingToolCalls {
		argumentsStr := pending.arguments.String()
		output = append(output, responsesOutputItemWire{
			Type:      "function_call",
			ID:        pending.itemID,
			CallID:    pending.itemID,
			Name:      pending.name,
			Arguments: &argumentsStr,
			Status:    "completed",
		})
	}

	// Build usage info from state
	inputTokens := state.inputTokens
	outputTokens := state.outputTokens
	cacheTokens := state.cacheTokens

	doneEvent := responsesCompletedEvent{
		Type:           "response.completed",
		SequenceNumber: int64(state.nextSequenceNumber()),
		Response: responsesWireResponse{
			ID:          state.responseID,
			Object:      "response",
			CreatedAt:   state.createdAt,
			Status:      "completed",
			CompletedAt: state.createdAt,
			Model:       "",
			Output:      output,
			Usage: &responsesUsageWire{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TotalTokens:  inputTokens + outputTokens,
				InputTokensDetails: responsesInputTokensDetailsWire{
					CachedTokens: cacheTokens,
				},
			},
		},
	}
	sendResponsesEvent(c, doneEvent, flusher)

	// Send final [DONE] message
	OpenAISSEDone(c)
}

// sendResponsesEvent sends a single Responses API event as SSE
func sendResponsesEvent(c *gin.Context, event any, flusher http.Flusher) {
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

	if e, ok := event.(responsesEvent); ok {
		OpenAIResponsesEvent(c, e.EventType(), event)
	} else {
		OpenAISSE(c, event)
	}
}

// sendResponsesErrorEvent sends an error event in Responses API format
func sendResponsesErrorEvent(c *gin.Context, message string, errorType string, flusher ...http.Flusher) {
	f := http.Flusher(nil)
	if len(flusher) > 0 {
		f = flusher[0]
	}

	errorEvent := responsesStreamErrorEvent{
		Type: "error",
		Error: responsesStreamErrorBody{
			Type:    errorType,
			Message: message,
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
	var output []responsesOutputItemWire

	// Add text content as a message item if present
	if state.accumulatedText != "" {
		output = append(output, responsesOutputItemWire{
			ID:     state.itemID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []responsesContentPartWire{
				{
					Type: "output_text",
					Text: state.accumulatedText,
				},
			},
		})
	}

	// Add tool calls with proper structure including call_id
	for _, pending := range state.pendingToolCalls {
		argumentsStr := pending.arguments.String()
		output = append(output, responsesOutputItemWire{
			Type:      "function_call",
			ID:        pending.itemID,
			CallID:    pending.itemID,
			Name:      pending.name,
			Arguments: &argumentsStr,
			Status:    "completed",
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

	doneEvent := responsesCompletedEvent{
		Type:           "response.completed",
		SequenceNumber: int64(state.nextSequenceNumber()),
		Response: responsesWireResponse{
			ID:          state.responseID,
			Object:      "response",
			CreatedAt:   state.createdAt,
			Status:      "completed",
			CompletedAt: state.createdAt,
			Model:       "",
			Output:      output,
			Usage: &responsesUsageWire{
				InputTokens:  int64(inputTokensFinal),
				OutputTokens: int64(outputTokensFinal),
				TotalTokens:  int64(inputTokensFinal + outputTokensFinal),
				InputTokensDetails: responsesInputTokensDetailsWire{
					CachedTokens: int64(cacheTokensFinal),
				},
			},
		},
	}
	sendResponsesEvent(c, doneEvent, flusher)

	// Send final [DONE] message
	OpenAISSEDone(c)
}

func newResponsesWireResponseFromState(state *responsesConverterState, status string, output []responsesOutputItemWire) responsesWireResponse {
	if output == nil {
		output = []responsesOutputItemWire{}
	}
	return responsesWireResponse{
		ID:        state.responseID,
		Object:    "response",
		CreatedAt: state.createdAt,
		Status:    status,
		Output:    output,
	}
}
