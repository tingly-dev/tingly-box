package stream

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// chatToResponsesConverter converts an OpenAI Chat Completions stream into
// a sequence of Responses API events. It implements StreamConverter.
type chatToResponsesConverter struct {
	stream        *openaistream.Stream[openai.ChatCompletionChunk]
	responseModel string

	// internal state
	responseID       string
	createdAt        int64
	sequenceNumber   int64
	outputIndex      int
	textItemID       string
	hasTextItem      bool
	pendingToolCalls map[int]*pendingToolCallResponse
	accumulatedText  strings.Builder
	promptTokensTotal int64
	inputTokens       int64
	outputTokens      int64
	cacheTokens       int64
	reasoningTokens   int64
	hasSentCreated   bool
	hasUsage         bool
	completedSent    bool
	finishReason     string

	// pending is an internal queue of events to yield one-by-one
	pending []wire.ResponsesEvent
}

// pendingToolCallResponse tracks a tool call being assembled from stream chunks
type pendingToolCallResponse struct {
	itemID    string
	callID    string
	outputIdx int
	name      string
	arguments strings.Builder
}

// NewChatToResponsesConverter creates a converter that reads from an OpenAI
// Chat Completions stream and yields Responses API wire events.
func NewChatToResponsesConverter(stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) *chatToResponsesConverter {
	return &chatToResponsesConverter{
		stream:        stream,
		responseModel: responseModel,
		responseID:    fmt.Sprintf("resp_%d", time.Now().Unix()),
		createdAt:     time.Now().Unix(),
		textItemID:    fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		pendingToolCalls: make(map[int]*pendingToolCallResponse),
	}
}

func (c *chatToResponsesConverter) Next() (interface{}, bool, error) {
	// Drain buffered events first
	if len(c.pending) > 0 {
		evt := c.pending[0]
		c.pending = c.pending[1:]
		return evt, false, nil
	}

	// Read upstream chunks until we have at least one event to yield
	for {
		if !c.stream.Next() {
			// Stream ended — emit completion events if not yet sent
			if err := c.stream.Err(); err != nil {
				return nil, false, err
			}
			if !c.completedSent {
				c.emitCompletionEvents()
				if len(c.pending) > 0 {
					evt := c.pending[0]
					c.pending = c.pending[1:]
					return evt, false, nil
				}
			}
			return nil, true, nil
		}

		chunk := c.stream.Current()
		c.processChunk(&chunk)

		if len(c.pending) > 0 {
			evt := c.pending[0]
			c.pending = c.pending[1:]
			return evt, false, nil
		}
	}
}

func (c *chatToResponsesConverter) Usage() *protocol.TokenUsage {
	return protocol.NewTokenUsageFull(int(c.inputTokens), int(c.outputTokens), int(c.cacheTokens), int(c.reasoningTokens))
}

// processChunk handles a single upstream ChatCompletionChunk and appends
// zero or more Responses API events to c.pending.
func (c *chatToResponsesConverter) processChunk(chunk *openai.ChatCompletionChunk) {
	// Emit response.created on first chunk
	if !c.hasSentCreated {
		c.pending = append(c.pending, wire.ResponsesCreatedEvent{
			Type:           "response.created",
			SequenceNumber: c.nextSeq(),
			Response:       c.wireResponse("in_progress", nil),
		})
		c.hasSentCreated = true
	}

	// Track usage
	if chunk.Usage.PromptTokens != 0 {
		c.promptTokensTotal = int64(chunk.Usage.PromptTokens)
		c.hasUsage = true
	}
	if chunk.Usage.CompletionTokens != 0 {
		c.outputTokens = int64(chunk.Usage.CompletionTokens)
		c.hasUsage = true
	}
	if chunk.Usage.PromptTokensDetails.CachedTokens != 0 {
		c.cacheTokens = int64(chunk.Usage.PromptTokensDetails.CachedTokens)
		c.hasUsage = true
	}
	if chunk.Usage.CompletionTokensDetails.ReasoningTokens != 0 {
		c.reasoningTokens = int64(chunk.Usage.CompletionTokensDetails.ReasoningTokens)
		c.hasUsage = true
	}
	if c.promptTokensTotal > 0 {
		c.inputTokens = c.promptTokensTotal - c.cacheTokens
	}

	if len(chunk.Choices) == 0 {
		return
	}

	choice := chunk.Choices[0]

	// Handle content delta
	if choice.Delta.Content != "" {
		if !c.hasTextItem {
			c.emitTextItemAdded()
			c.hasTextItem = true
		}
		c.accumulatedText.WriteString(choice.Delta.Content)
		c.pending = append(c.pending, wire.ResponsesOutputTextDeltaEvent{
			Type:           "response.output_text.delta",
			SequenceNumber: c.nextSeq(),
			ItemID:         c.textItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Delta:          choice.Delta.Content,
			Logprobs:       []interface{}{},
		})
	}

	// Handle tool_calls delta
	for _, toolCall := range choice.Delta.ToolCalls {
		openaiIndex := int(toolCall.Index)

		if _, exists := c.pendingToolCalls[openaiIndex]; !exists {
			itemID := fmt.Sprintf("fc_%d_%d", time.Now().Unix(), openaiIndex)
			if toolCall.ID != "" {
				itemID = truncateToolCallID(toolCall.ID)
			}

			toolOutputIndex := c.outputIndex
			c.outputIndex++

			c.pendingToolCalls[openaiIndex] = &pendingToolCallResponse{
				itemID:    itemID,
				callID:    toolCall.ID,
				outputIdx: toolOutputIndex,
				name:      toolCall.Function.Name,
			}

			callID := toolCall.ID
			if callID == "" {
				callID = itemID
			}
			c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
				Type:           "response.output_item.added",
				SequenceNumber: c.nextSeq(),
				OutputIndex:    toolOutputIndex,
				Item:           newResponsesFunctionCallItem(itemID, callID, toolCall.Function.Name, "", "in_progress"),
			})
		}

		if toolCall.Function.Arguments != "" {
			ptc := c.pendingToolCalls[openaiIndex]
			ptc.arguments.WriteString(toolCall.Function.Arguments)
			c.pending = append(c.pending, wire.ResponsesFunctionCallArgumentsDeltaEvent{
				Type:           "response.function_call_arguments.delta",
				SequenceNumber: c.nextSeq(),
				ItemID:         ptc.itemID,
				OutputIndex:    ptc.outputIdx,
				Delta:          toolCall.Function.Arguments,
			})
		}
	}

	// Check for completion
	if choice.FinishReason != "" {
		c.finishReason = string(choice.FinishReason)

		if !c.hasUsage && c.outputTokens == 0 {
			c.outputTokens = int64(c.accumulatedText.Len() / 4)
			for _, ptc := range c.pendingToolCalls {
				c.outputTokens += int64(ptc.arguments.Len() / 4)
			}
		}

		c.emitCompletionEvents()
	}
}

// emitCompletionEvents appends the terminal sequence of events (text done,
// tool call done, response.completed) to c.pending.
func (c *chatToResponsesConverter) emitCompletionEvents() {
	if c.completedSent {
		return
	}
	c.completedSent = true

	if !c.hasSentCreated {
		c.pending = append(c.pending, wire.ResponsesCreatedEvent{
			Type:           "response.created",
			SequenceNumber: c.nextSeq(),
			Response:       c.wireResponse("in_progress", nil),
		})
		c.hasSentCreated = true
	}

	if c.finishReason == "" {
		c.finishReason = "stop"
	}

	if c.hasTextItem {
		text := c.accumulatedText.String()
		c.pending = append(c.pending, wire.ResponsesOutputTextDoneEvent{
			Type:           "response.output_text.done",
			SequenceNumber: c.nextSeq(),
			ItemID:         c.textItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Text:           text,
			Logprobs:       []interface{}{},
		})
		c.pending = append(c.pending, wire.ResponsesOutputItemDoneEvent{
			Type:           "response.output_item.done",
			SequenceNumber: c.nextSeq(),
			OutputIndex:    0,
			Item:           newResponsesMessageItem(c.textItemID, "completed", text),
		})
	}

	sortedIndexes := make([]int, 0, len(c.pendingToolCalls))
	for idx := range c.pendingToolCalls {
		sortedIndexes = append(sortedIndexes, idx)
	}
	sort.Ints(sortedIndexes)

	for _, idx := range sortedIndexes {
		ptc := c.pendingToolCalls[idx]
		callID := ptc.callID
		if callID == "" {
			callID = ptc.itemID
		}
		arguments := ptc.arguments.String()
		c.pending = append(c.pending, wire.ResponsesFunctionCallArgumentsDoneEvent{
			Type:           "response.function_call_arguments.done",
			SequenceNumber: c.nextSeq(),
			ItemID:         ptc.itemID,
			OutputIndex:    ptc.outputIdx,
			Name:           ptc.name,
			Arguments:      arguments,
		})
		c.pending = append(c.pending, wire.ResponsesOutputItemDoneEvent{
			Type:           "response.output_item.done",
			SequenceNumber: c.nextSeq(),
			OutputIndex:    ptc.outputIdx,
			Item:           newResponsesFunctionCallItem(ptc.itemID, callID, ptc.name, arguments, "completed"),
		})
	}

	isIncomplete, incompleteReason := chatFinishReasonToIncomplete(c.finishReason)
	itemStatus := "completed"
	if isIncomplete {
		itemStatus = "incomplete"
	}

	var output []wire.ResponsesOutputItemWire
	if c.accumulatedText.Len() > 0 {
		output = append(output, newResponsesMessageItem(c.textItemID, itemStatus, c.accumulatedText.String()))
	}
	for _, idx := range sortedIndexes {
		ptc := c.pendingToolCalls[idx]
		callID := ptc.callID
		if callID == "" {
			callID = ptc.itemID
		}
		output = append(output, newResponsesFunctionCallItem(ptc.itemID, callID, ptc.name, ptc.arguments.String(), itemStatus))
	}

	if isIncomplete {
		resp := c.wireResponse("incomplete", output)
		resp.IncompleteDetails = &wire.ResponsesIncompleteDetailsWire{Reason: incompleteReason}
		c.pending = append(c.pending, wire.ResponsesIncompleteEvent{
			Type:           "response.incomplete",
			SequenceNumber: c.nextSeq(),
			Response:       resp,
		})
	} else {
		c.pending = append(c.pending, wire.ResponsesCompletedEvent{
			Type:           "response.completed",
			SequenceNumber: c.nextSeq(),
			Response:       c.wireResponse("completed", output),
		})
	}
}

func (c *chatToResponsesConverter) emitTextItemAdded() {
	if c.outputIndex == 0 {
		c.outputIndex = 1
	}
	c.pending = append(c.pending, wire.ResponsesOutputItemAddedEvent{
		Type:           "response.output_item.added",
		SequenceNumber: c.nextSeq(),
		OutputIndex:    0,
		Item:           newResponsesMessageItem(c.textItemID, "in_progress", ""),
	})
}

func (c *chatToResponsesConverter) nextSeq() int64 {
	c.sequenceNumber++
	return c.sequenceNumber
}

func (c *chatToResponsesConverter) wireResponse(status string, output []wire.ResponsesOutputItemWire) wire.ResponsesWireResponse {
	if output == nil {
		output = []wire.ResponsesOutputItemWire{}
	}
	return wire.ResponsesWireResponse{
		ID:        c.responseID,
		Object:    "response",
		CreatedAt: c.createdAt,
		Status:    status,
		Output:    output,
		Usage: &wire.ResponsesUsageWire{
			InputTokens:  c.inputTokens,
			OutputTokens: c.outputTokens,
			TotalTokens:  c.inputTokens + c.outputTokens,
			InputTokensDetails: wire.ResponsesInputTokensDetailsWire{
				CachedTokens: c.cacheTokens,
			},
			OutputTokensDetails: wire.ResponsesOutputTokensDetailsWire{
				ReasoningTokens: c.reasoningTokens,
			},
		},
		Model: c.responseModel,
	}
}

// HandleOpenAIChatToResponsesStream converts OpenAI Chat Completions streaming
// to Responses API format using the chain pipeline architecture.
func HandleOpenAIChatToResponsesStream(hc *protocol.HandleContext, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	conv := NewChatToResponsesConverter(stream, responseModel)

	usage, err := RunConverter(hc, conv, responsesSSEWriter(c))

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Chat to Responses stream canceled by client")
			return conv.Usage(), nil
		}

		logrus.WithContext(c.Request.Context()).Errorf("Chat to Responses stream error: %v", err)

		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return conv.Usage(), err
		}

		errorEvent := wire.ResponsesStreamErrorEvent{
			Type:           "error",
			SequenceNumber: conv.nextSeq(),
			Error: wire.ResponsesStreamErrorBody{
				Message: err.Error(),
				Type:    "stream_error",
			},
		}
		OpenAIResponsesEvent(c, errorEvent.EventType(), errorEvent)
		return conv.Usage(), err
	}

	OpenAISSEDone(c)
	return usage, nil
}

// chatFinishReasonToIncomplete maps an OpenAI Chat finish_reason to the
// Responses API incomplete status. Returns (true, reason) when the response
// should be marked incomplete, or (false, "") for normal completion.
func chatFinishReasonToIncomplete(finishReason string) (bool, string) {
	switch finishReason {
	case "length":
		return true, "max_output_tokens"
	case "content_filter":
		return true, "content_filter"
	default:
		return false, ""
	}
}

// responsesSSEWriter returns a handleFunc that writes Responses API wire
// events as SSE to the gin context.
func responsesSSEWriter(c *gin.Context) func(event interface{}) error {
	return func(event interface{}) error {
		evt, ok := event.(wire.ResponsesEvent)
		if !ok {
			return fmt.Errorf("unexpected event type %T", event)
		}
		OpenAIResponsesEvent(c, evt.EventType(), evt)
		return nil
	}
}

func newResponsesMessageItem(itemID, status, text string) wire.ResponsesOutputItemWire {
	return wire.ResponsesOutputItemWire{
		ID:     itemID,
		Type:   "message",
		Role:   "assistant",
		Status: status,
		Content: []wire.ResponsesContentPartWire{
			{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			},
		},
	}
}

func newResponsesFunctionCallItem(itemID, callID, name, arguments, status string) wire.ResponsesOutputItemWire {
	return wire.ResponsesOutputItemWire{
		ID:        itemID,
		CallID:    callID,
		Type:      "function_call",
		Name:      name,
		Arguments: &arguments,
		Status:    status,
	}
}
