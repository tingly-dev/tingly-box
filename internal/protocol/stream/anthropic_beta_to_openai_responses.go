package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleAnthropicBetaToOpenAIResponsesStream converts Anthropic Beta streaming
// to Responses API format using the chain pipeline architecture.
func HandleAnthropicBetaToOpenAIResponsesStream(
	hc *protocol.HandleContext,
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	responseModel string,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	conv := NewAnthropicBetaToResponsesConverter(stream, responseModel)

	usage, err := RunConverter(hc, conv, responsesSSEWriter(c))

	if err != nil {
		if !c.Writer.Written() && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Errorf("Anthropic to Responses pre-stream error: %v", err)
			SendStreamingError(c, err)
			return protocol.ZeroTokenUsage(), err
		}
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to Responses stream canceled by client")
			return conv.Usage(), nil
		}
		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			OpenAISSEDone(c)
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		sendResponsesErrorEvent(c, err.Error(), "stream_error")
		return conv.Usage(), err
	}

	OpenAISSEDone(c)
	return usage, nil
}

// sendResponsesEvent sends a single Responses API event as SSE.
func sendResponsesEvent(c *gin.Context, event any, _ interface{ Flush() }) {
	if e, ok := event.(wire.ResponsesEvent); ok {
		OpenAIResponsesEvent(c, e.EventType(), event)
	} else {
		OpenAISSE(c, event)
	}
}

// sendResponsesErrorEvent sends an error event in Responses API format.
func sendResponsesErrorEvent(c *gin.Context, message string, errorType string, _ ...http.Flusher) {
	errorEvent := wire.ResponsesStreamErrorEvent{
		Type: "error",
		Error: wire.ResponsesStreamErrorBody{
			Type:    errorType,
			Message: message,
		},
	}
	OpenAIResponsesEvent(c, errorEvent.EventType(), errorEvent)
}

func responsesUsageWire(u *protocol.TokenUsage) *wire.ResponsesUsageWire {
	totalInput := int64(u.InputTokens + u.CacheInputTokens)
	return &wire.ResponsesUsageWire{
		InputTokens:  totalInput,
		OutputTokens: int64(u.OutputTokens),
		TotalTokens:  totalInput + int64(u.OutputTokens),
		InputTokensDetails: wire.ResponsesInputTokensDetailsWire{
			CachedTokens: int64(u.CacheInputTokens),
		},
		OutputTokensDetails: wire.ResponsesOutputTokensDetailsWire{
			ReasoningTokens: int64(u.ReasoningTokens),
		},
	}
}

// responsesConverterState and newResponsesWireResponseFromState are kept for
// any callers outside this file that still use them.
type responsesConverterState struct {
	responseID       string
	itemID           string
	outputIndex      int
	accumulatedText  string
	finished         bool
	pendingToolCalls map[int]*pendingResponseToolCall
	hasSentCreated   bool
	sequenceNumber   int
	createdAt        int64
	currentBlockType string
}

func newResponsesConverterState(timestamp int64) *responsesConverterState {
	return &responsesConverterState{
		responseID:       fmt.Sprintf("resp_%d", timestamp),
		itemID:           fmt.Sprintf("item_%d", timestamp),
		pendingToolCalls: make(map[int]*pendingResponseToolCall),
		createdAt:        timestamp,
	}
}

func (s *responsesConverterState) nextSequenceNumber() int {
	seq := s.sequenceNumber
	s.sequenceNumber++
	return seq
}

func newResponsesWireResponseFromState(state *responsesConverterState, status string, output []wire.ResponsesOutputItemWire) wire.ResponsesWireResponse {
	if output == nil {
		output = []wire.ResponsesOutputItemWire{}
	}
	return wire.ResponsesWireResponse{
		ID:        state.responseID,
		Object:    "response",
		CreatedAt: state.createdAt,
		Status:    status,
		Output:    output,
	}
}

// sendCompletionEvent is kept for callers that use the old responsesConverterState.
func sendCompletionEvent(c *gin.Context, state *responsesConverterState, flusher http.Flusher, u *protocol.TokenUsage) {
	if c == nil || c.Writer == nil || flusher == nil {
		return
	}
	state.finished = true
	var output []wire.ResponsesOutputItemWire
	if state.accumulatedText != "" {
		output = append(output, wire.ResponsesOutputItemWire{
			ID:     state.itemID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []wire.ResponsesContentPartWire{
				{Type: "output_text", Text: state.accumulatedText},
			},
		})
	}
	for _, pending := range state.pendingToolCalls {
		argumentsStr := pending.arguments.String()
		output = append(output, wire.ResponsesOutputItemWire{
			Type:      "function_call",
			ID:        pending.itemID,
			CallID:    pending.itemID,
			Name:      pending.name,
			Arguments: &argumentsStr,
			Status:    "completed",
		})
	}
	doneEvent := wire.ResponsesCompletedEvent{
		Type:           "response.completed",
		SequenceNumber: int64(state.nextSequenceNumber()),
		Response: wire.ResponsesWireResponse{
			ID:          state.responseID,
			Object:      "response",
			CreatedAt:   state.createdAt,
			Status:      "completed",
			CompletedAt: state.createdAt,
			Output:      output,
			Usage:       responsesUsageWire(u),
		},
	}
	sendResponsesEvent(c, doneEvent, flusher)
	OpenAISSEDone(c)
}
