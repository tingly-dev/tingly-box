package stream

import (
	"errors"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol/ids"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// errAnthropicStreamTruncated reports an upstream Anthropic stream that ended
// (with no transport error) before delivering message_stop. Shared by every
// converter that consumes an AnthropicBetaStream so callers can errors.Is it.
var errAnthropicStreamTruncated = errors.New("anthropic stream ended without message_stop")

// anthropicBetaToResponsesConverter converts an Anthropic Beta stream into
// a sequence of Responses API wire events. It implements StreamConverter.
type anthropicBetaToResponsesConverter struct {
	stream        AnthropicBetaStream
	responseModel string
	acc           *usagepkg.AnthropicAccumulator

	// state (formerly responsesConverterState)
	responseID        string
	outputIndex       int
	finished          bool
	pendingTextBlocks map[int]*pendingResponseTextBlock
	pendingToolCalls  map[int]*pendingResponseToolCall
	hasSentCreated    bool
	sequenceNumber    int
	createdAt         int64
	stopReason        string

	// internal event queue
	pending []wire.ResponsesEvent
}

// pendingResponseTextBlock tracks one Anthropic text content block as one
// Responses message output item. Anthropic may interleave multiple text and
// tool-use blocks, so text state cannot be shared across the whole response.
type pendingResponseTextBlock struct {
	itemID      string
	outputIndex int
	text        strings.Builder
}

// pendingResponseToolCall tracks a tool call being assembled from Anthropic stream chunks
type pendingResponseToolCall struct {
	itemID      string
	callID      string
	name        string
	outputIndex int
	arguments   strings.Builder
}

// newAnthropicBetaToResponsesConverter creates a converter that reads from an
// Anthropic Beta stream and yields Responses API wire events.
func newAnthropicBetaToResponsesConverter(
	stream AnthropicBetaStream,
	responseModel string,
) *anthropicBetaToResponsesConverter {
	ts := time.Now().Unix()
	return &anthropicBetaToResponsesConverter{
		stream:            stream,
		responseModel:     responseModel,
		acc:               usagepkg.NewAnthropicAccumulator(),
		responseID:        ids.Response(),
		pendingTextBlocks: make(map[int]*pendingResponseTextBlock),
		pendingToolCalls:  make(map[int]*pendingResponseToolCall),
		createdAt:         ts,
	}
}

// NewAnthropicBetaToOpenAIResponsesConverter creates a transport-neutral
// converter. HTTP/SSE framing and stream close ownership remain with the
// caller, so the same converter can serve legacy handlers and Stage Bridges.
func NewAnthropicBetaToOpenAIResponsesConverter(
	stream AnthropicBetaStream,
	responseModel string,
) StreamConverter {
	return newAnthropicBetaToResponsesConverter(stream, responseModel)
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
			if !c.finished {
				return nil, false, errAnthropicStreamTruncated
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
	index := int(event.Index)
	blockType := event.ContentBlock.Type
	currentOutputIndex := c.outputIndex

	if blockType == "text" {
		itemID := ids.Message()
		c.pendingTextBlocks[index] = &pendingResponseTextBlock{
			itemID:      itemID,
			outputIndex: currentOutputIndex,
		}
		c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(c.nextSeq()),
			OutputIndex:    currentOutputIndex,
			Item: wire.ResponsesOutputItemWire{
				ID:      itemID,
				Type:    "message",
				Status:  "in_progress",
				Role:    "assistant",
				Content: []wire.ResponsesContentPartWire{},
			},
		})
		c.pending = append(c.pending, wire.ResponsesContentPartAddedEvent{
			Type:           "response.content_part.added",
			SequenceNumber: int64(c.nextSeq()),
			ItemID:         itemID,
			OutputIndex:    currentOutputIndex,
			ContentIndex:   0,
			Part:           wire.ResponsesContentPartWire{Type: "output_text", Text: ""},
		})
		c.outputIndex++
	} else if blockType == "tool_use" {
		toolID := event.ContentBlock.ID
		toolName := event.ContentBlock.Name
		c.pendingToolCalls[index] = &pendingResponseToolCall{itemID: ids.FunctionCall(), callID: toolID, name: toolName, outputIndex: currentOutputIndex}

		arguments := ""
		c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
			Type:           "response.output_item.added",
			SequenceNumber: int64(c.nextSeq()),
			OutputIndex:    currentOutputIndex,
			Item: wire.ResponsesOutputItemWire{
				Type:      "function_call",
				ID:        c.pendingToolCalls[index].itemID,
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
	index := int(event.Index)

	if deltaType == "text_delta" {
		block, exists := c.pendingTextBlocks[index]
		if !exists {
			return
		}
		delta := event.Delta.Text
		block.text.WriteString(delta)
		c.pending = append(c.pending, wire.ResponsesOutputTextDeltaEvent{
			Type:           "response.output_text.delta",
			Delta:          delta,
			ItemID:         block.itemID,
			OutputIndex:    block.outputIndex,
			ContentIndex:   0,
			SequenceNumber: int64(c.nextSeq()),
		})
	} else if deltaType == "input_json_delta" {
		if pending, exists := c.pendingToolCalls[index]; exists {
			argsDelta := event.Delta.PartialJSON
			pending.arguments.WriteString(argsDelta)
			c.pending = append(c.pending, wire.ResponsesFunctionCallArgumentsDeltaEvent{
				Type:           "response.function_call_arguments.delta",
				Delta:          argsDelta,
				ItemID:         pending.itemID,
				OutputIndex:    pending.outputIndex,
				SequenceNumber: int64(c.nextSeq()),
			})
		}
	}
}

func (c *anthropicBetaToResponsesConverter) emitContentBlockStop(event *anthropic.BetaRawMessageStreamEventUnion) {
	index := int(event.Index)

	if block, exists := c.pendingTextBlocks[index]; exists {
		text := block.text.String()
		c.pending = append(c.pending,
			wire.ResponsesOutputTextDoneEvent{
				Type:           "response.output_text.done",
				ItemID:         block.itemID,
				OutputIndex:    block.outputIndex,
				ContentIndex:   0,
				Text:           text,
				SequenceNumber: int64(c.nextSeq()),
			},
			wire.ResponsesContentPartDoneEvent{
				Type:           "response.content_part.done",
				SequenceNumber: int64(c.nextSeq()),
				ItemID:         block.itemID,
				OutputIndex:    block.outputIndex,
				ContentIndex:   0,
				Part:           wire.ResponsesContentPartWire{Type: "output_text", Text: text},
			},
			wire.ResponsesOutputItemDoneEvent{
				Type:           "response.output_item.done",
				SequenceNumber: int64(c.nextSeq()),
				OutputIndex:    block.outputIndex,
				Item: wire.ResponsesOutputItemWire{
					ID:     block.itemID,
					Type:   "message",
					Status: "completed",
					Role:   "assistant",
					Content: []wire.ResponsesContentPartWire{
						{Type: "output_text", Text: text},
					},
				},
			},
		)
	} else if pending, exists := c.pendingToolCalls[index]; exists {
		argumentsStr := pending.arguments.String()
		c.pending = append(c.pending,
			wire.ResponsesFunctionCallArgumentsDoneEvent{
				Type:           "response.function_call_arguments.done",
				ItemID:         pending.itemID,
				OutputIndex:    pending.outputIndex,
				Arguments:      argumentsStr,
				SequenceNumber: int64(c.nextSeq()),
			},
			wire.ResponsesOutputItemDoneEvent{
				Type:           "response.output_item.done",
				SequenceNumber: int64(c.nextSeq()),
				OutputIndex:    pending.outputIndex,
				Item: wire.ResponsesOutputItemWire{
					Type:      "function_call",
					ID:        pending.itemID,
					CallID:    pending.callID,
					Name:      pending.name,
					Arguments: &argumentsStr,
					Status:    "completed",
				},
			},
		)
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

	output := make([]wire.ResponsesOutputItemWire, c.outputIndex)
	for _, block := range c.pendingTextBlocks {
		output[block.outputIndex] = wire.ResponsesOutputItemWire{
			ID:     block.itemID,
			Type:   "message",
			Status: itemStatus,
			Role:   "assistant",
			Content: []wire.ResponsesContentPartWire{
				{Type: "output_text", Text: block.text.String()},
			},
		}
	}
	for _, pending := range c.pendingToolCalls {
		argumentsStr := pending.arguments.String()
		output[pending.outputIndex] = wire.ResponsesOutputItemWire{
			Type:      "function_call",
			ID:        pending.itemID,
			CallID:    pending.callID,
			Name:      pending.name,
			Arguments: &argumentsStr,
			Status:    itemStatus,
		}
	}

	u := c.acc.Result()
	resp := wire.ResponsesWireResponse{
		ID:          c.responseID,
		Object:      "response",
		CreatedAt:   c.createdAt,
		CompletedAt: c.createdAt,
		Output:      output,
		Usage:       usagepkg.ToResponsesUsageWire(u),
		Model:       c.responseModel,
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
