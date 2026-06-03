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
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// anthropicBetaToResponsesConverter converts an Anthropic Beta stream into
// a sequence of Responses API wire events. It implements StreamConverter.
type anthropicBetaToResponsesConverter struct {
	stream        *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	responseModel string
	acc           *usagepkg.AnthropicAccumulator

	// state (formerly responsesConverterState)
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
	stopReason       string

	// internal event queue
	pending []wire.ResponsesEvent
}

// pendingResponseToolCall tracks a tool call being assembled from Anthropic stream chunks
type pendingResponseToolCall struct {
	itemID    string
	name      string
	arguments strings.Builder
}

// NewAnthropicBetaToResponsesConverter creates a converter that reads from an
// Anthropic Beta stream and yields Responses API wire events.
func NewAnthropicBetaToResponsesConverter(
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	responseModel string,
) *anthropicBetaToResponsesConverter {
	ts := time.Now().Unix()
	return &anthropicBetaToResponsesConverter{
		stream:           stream,
		responseModel:    responseModel,
		acc:              usagepkg.NewAnthropicAccumulator(),
		responseID:       fmt.Sprintf("resp_%d", ts),
		itemID:           fmt.Sprintf("item_%d", ts),
		pendingToolCalls: make(map[int]*pendingResponseToolCall),
		createdAt:        ts,
	}
}

func (c *anthropicBetaToResponsesConverter) Next() (interface{}, bool, error) {
	if len(c.pending) > 0 {
		evt := c.pending[0]
		c.pending = c.pending[1:]
		return evt, false, nil
	}

	for {
		if !c.stream.Next() {
			if err := c.stream.Err(); err != nil {
				return nil, false, err
			}
			// Stream ended without message_stop — emit fallback completion
			if !c.finished {
				c.emitCompletionEvents()
				if len(c.pending) > 0 {
					evt := c.pending[0]
					c.pending = c.pending[1:]
					return evt, false, nil
				}
			}
			return nil, true, nil
		}

		event := c.stream.Current()
		c.processEvent(&event)

		if len(c.pending) > 0 {
			evt := c.pending[0]
			c.pending = c.pending[1:]
			return evt, false, nil
		}
	}
}

func (c *anthropicBetaToResponsesConverter) Usage() *protocol.TokenUsage {
	return c.acc.Result()
}

func (c *anthropicBetaToResponsesConverter) processEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	switch event.Type {
	case "message_start":
		c.emitMessageStart()
		c.hasSentCreated = true
		c.acc.ConsumeBeta(event)

	case "content_block_start":
		c.emitContentBlockStart(event)

	case "content_block_delta":
		c.emitContentBlockDelta(event)

	case "content_block_stop":
		c.emitContentBlockStop(event)

	case "message_delta":
		if event.Delta.StopReason != "" {
			c.stopReason = string(event.Delta.StopReason)
		}
		c.acc.ConsumeBeta(event)

	case "message_stop":
		c.emitCompletionEvents()
	}
}

func (c *anthropicBetaToResponsesConverter) emitMessageStart() {
	resp := c.wireResponse("in_progress", nil)
	resp.Model = c.responseModel
	resp.Usage = nil

	c.pending = append(c.pending, wire.ResponsesCreatedEvent{
		Type:           "response.created",
		SequenceNumber: int64(c.nextSeq()),
		Response:       resp,
	})

	inProgressResp := c.wireResponse("in_progress", nil)
	inProgressResp.Model = c.responseModel
	inProgressResp.Usage = nil

	c.pending = append(c.pending, wire.ResponsesInProgressEvent{
		Type:           "response.in_progress",
		SequenceNumber: int64(c.nextSeq()),
		Response:       inProgressResp,
	})
}

func (c *anthropicBetaToResponsesConverter) emitContentBlockStart(event *anthropic.BetaRawMessageStreamEventUnion) {
	index := event.Index
	blockType := event.ContentBlock.Type
	c.currentBlockType = blockType

	if blockType == "text" {
		c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(c.nextSeq()),
			OutputIndex:    c.outputIndex,
			Item: wire.ResponsesOutputItemWire{
				ID:      c.itemID,
				Type:    "message",
				Status:  "in_progress",
				Role:    "assistant",
				Content: []wire.ResponsesContentPartWire{},
			},
		})
		c.pending = append(c.pending, wire.ResponsesContentPartAddedEvent{
			Type:           "response.content_part.added",
			SequenceNumber: int64(c.nextSeq()),
			ItemID:         c.itemID,
			OutputIndex:    c.outputIndex,
			ContentIndex:   0,
			Part:           wire.ResponsesContentPartWire{Type: "output_text", Text: ""},
		})
	} else if blockType == "tool_use" {
		toolID := event.ContentBlock.ID
		toolName := event.ContentBlock.Name
		c.pendingToolCalls[int(index)] = &pendingResponseToolCall{itemID: toolID, name: toolName}

		arguments := ""
		c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(c.nextSeq()),
			OutputIndex:    c.outputIndex,
			Item: wire.ResponsesOutputItemWire{
				Type:      "function_call",
				ID:        toolID,
				CallID:    toolID,
				Name:      toolName,
				Arguments: &arguments,
				Status:    "in_progress",
			},
		})
		c.outputIndex++
	}
}

func (c *anthropicBetaToResponsesConverter) emitContentBlockDelta(event *anthropic.BetaRawMessageStreamEventUnion) {
	deltaType := event.Delta.Type
	index := event.Index

	if deltaType == "text_delta" {
		text := event.Delta.Text
		c.accumulatedText += text
		c.pending = append(c.pending, wire.ResponsesOutputTextDeltaEvent{
			Type:           "response.output_text.delta",
			Delta:          text,
			ItemID:         c.itemID,
			OutputIndex:    c.outputIndex,
			ContentIndex:   0,
			SequenceNumber: int64(c.nextSeq()),
		})
	} else if deltaType == "input_json_delta" {
		if pending, exists := c.pendingToolCalls[int(index)]; exists {
			argsDelta := event.Delta.PartialJSON
			pending.arguments.WriteString(argsDelta)
			c.pending = append(c.pending, wire.ResponsesFunctionCallArgumentsDeltaEvent{
				Type:           "response.function_call_arguments.delta",
				Delta:          argsDelta,
				ItemID:         pending.itemID,
				OutputIndex:    c.outputIndex,
				SequenceNumber: int64(c.nextSeq()),
			})
		}
	}
}

func (c *anthropicBetaToResponsesConverter) emitContentBlockStop(event *anthropic.BetaRawMessageStreamEventUnion) {
	index := event.Index
	blockType := c.currentBlockType

	if blockType == "text" {
		c.pending = append(c.pending,
			wire.ResponsesOutputTextDoneEvent{
				Type:           "response.output_text.done",
				ItemID:         c.itemID,
				OutputIndex:    c.outputIndex,
				ContentIndex:   0,
				Text:           c.accumulatedText,
				SequenceNumber: int64(c.nextSeq()),
			},
			wire.ResponsesContentPartDoneEvent{
				Type:           "response.content_part.done",
				SequenceNumber: int64(c.nextSeq()),
				ItemID:         c.itemID,
				OutputIndex:    c.outputIndex,
				ContentIndex:   0,
				Part:           wire.ResponsesContentPartWire{Type: "output_text", Text: c.accumulatedText},
			},
			wire.ResponsesOutputItemDoneEvent{
				Type:           "response.output_item.done",
				SequenceNumber: int64(c.nextSeq()),
				OutputIndex:    c.outputIndex,
				Item: wire.ResponsesOutputItemWire{
					ID:     c.itemID,
					Type:   "message",
					Status: "completed",
					Role:   "assistant",
					Content: []wire.ResponsesContentPartWire{
						{Type: "output_text", Text: c.accumulatedText},
					},
				},
			},
		)
	} else if blockType == "tool_use" {
		if pending, exists := c.pendingToolCalls[int(index)]; exists {
			argumentsStr := pending.arguments.String()
			c.pending = append(c.pending,
				wire.ResponsesFunctionCallArgumentsDoneEvent{
					Type:           "response.function_call_arguments.done",
					ItemID:         pending.itemID,
					OutputIndex:    c.outputIndex,
					Arguments:      argumentsStr,
					SequenceNumber: int64(c.nextSeq()),
				},
				wire.ResponsesOutputItemDoneEvent{
					Type:           "response.output_item.done",
					SequenceNumber: int64(c.nextSeq()),
					OutputIndex:    c.outputIndex,
					Item: wire.ResponsesOutputItemWire{
						Type:      "function_call",
						ID:        pending.itemID,
						CallID:    pending.itemID,
						Name:      pending.name,
						Arguments: &argumentsStr,
						Status:    "completed",
					},
				},
			)
		}
	}
}

func (c *anthropicBetaToResponsesConverter) emitCompletionEvents() {
	if c.finished {
		return
	}
	c.finished = true

	if !c.hasSentCreated {
		c.emitMessageStart()
		c.hasSentCreated = true
	}

	isIncomplete, incompleteReason := anthropicStopReasonToIncomplete(c.stopReason)
	itemStatus := "completed"
	if isIncomplete {
		itemStatus = "incomplete"
	}

	var output []wire.ResponsesOutputItemWire
	if c.accumulatedText != "" {
		output = append(output, wire.ResponsesOutputItemWire{
			ID:     c.itemID,
			Type:   "message",
			Status: itemStatus,
			Role:   "assistant",
			Content: []wire.ResponsesContentPartWire{
				{Type: "output_text", Text: c.accumulatedText},
			},
		})
	}
	for _, pending := range c.pendingToolCalls {
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

	u := c.acc.Result()
	resp := wire.ResponsesWireResponse{
		ID:          c.responseID,
		Object:      "response",
		CreatedAt:   c.createdAt,
		CompletedAt: c.createdAt,
		Output:      output,
		Usage:       toResponsesUsageWire(u),
	}

	if isIncomplete {
		resp.Status = "incomplete"
		resp.IncompleteDetails = &wire.ResponsesIncompleteDetailsWire{Reason: incompleteReason}
		c.pending = append(c.pending, wire.ResponsesIncompleteEvent{
			Type:           "response.incomplete",
			SequenceNumber: int64(c.nextSeq()),
			Response:       resp,
		})
	} else {
		resp.Status = "completed"
		c.pending = append(c.pending, wire.ResponsesCompletedEvent{
			Type:           "response.completed",
			SequenceNumber: int64(c.nextSeq()),
			Response:       resp,
		})
	}
}

// anthropicStopReasonToIncomplete maps an Anthropic stop_reason to the
// Responses API incomplete status. Returns (true, reason) when the response
// should be marked incomplete, or (false, "") for normal completion.
func anthropicStopReasonToIncomplete(stopReason string) (bool, string) {
	switch stopReason {
	case "max_tokens":
		return true, "max_output_tokens"
	default:
		return false, ""
	}
}

func (c *anthropicBetaToResponsesConverter) nextSeq() int {
	seq := c.sequenceNumber
	c.sequenceNumber++
	return seq
}

func (c *anthropicBetaToResponsesConverter) wireResponse(status string, output []wire.ResponsesOutputItemWire) wire.ResponsesWireResponse {
	if output == nil {
		output = []wire.ResponsesOutputItemWire{}
	}
	return wire.ResponsesWireResponse{
		ID:        c.responseID,
		Object:    "response",
		CreatedAt: c.createdAt,
		Status:    status,
		Output:    output,
	}
}

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

// toResponsesUsageWire converts normalized TokenUsage to the Responses API wire
// usage struct. InputTokens on the wire = total (uncached + cached).
func toResponsesUsageWire(u *protocol.TokenUsage) *wire.ResponsesUsageWire {
	totalInput := int64(u.InputTokens + u.CacheInputTokens)
	return &wire.ResponsesUsageWire{
		InputTokens:  totalInput,
		OutputTokens: int64(u.OutputTokens),
		TotalTokens:  totalInput + int64(u.OutputTokens),
		InputTokensDetails: wire.ResponsesInputTokensDetailsWire{
			CachedTokens: int64(u.CacheInputTokens),
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
			Usage:       toResponsesUsageWire(u),
		},
	}
	sendResponsesEvent(c, doneEvent, flusher)
	OpenAISSEDone(c)
}
