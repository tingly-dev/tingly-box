package stream

import (
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// responsesToChatConverter converts a Responses API stream into a sequence of
// OpenAI Chat Completions chunks. It implements StreamConverter.
type responsesToChatConverter struct {
	stream        ResponsesStreamIter
	responseModel string
	disableUsage  bool

	// state
	chatID          string
	createdAt       int64
	usage           *protocol.TokenUsage
	totalTokens     int64
	hasSentCreated  bool
	serviceTier     *string // stamped on every chunk; refined at completion
	hasToolCalls    bool
	completed       bool
	toolCallIndexes map[string]int
	toolCalls       map[int]*responsesToChatToolCall
	textParts       responseTextTracker

	// internal event queue
	pending []interface{}
}

type responsesToChatToolCall struct {
	id               string
	callID           string
	name             string
	chatIndex        int
	initialArguments string
	arguments        strings.Builder
}

// newResponsesToChatConverter creates a converter that reads from a Responses
// API stream and yields OpenAI Chat Completions wire chunks.
func newResponsesToChatConverter(stream ResponsesStreamIter, responseModel string, disableUsage bool) *responsesToChatConverter {
	return &responsesToChatConverter{
		stream:          stream,
		responseModel:   responseModel,
		disableUsage:    disableUsage,
		chatID:          fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		createdAt:       time.Now().Unix(),
		usage:           protocol.ZeroTokenUsage(),
		toolCallIndexes: make(map[string]int),
		toolCalls:       make(map[int]*responsesToChatToolCall),
		textParts:       make(responseTextTracker),
	}
}

// NewOpenAIResponsesToChatConverter creates a transport-neutral Responses to
// Chat stream converter. The caller owns stream closure and SSE framing.
func NewOpenAIResponsesToChatConverter(
	stream ResponsesStreamIter,
	responseModel string,
	disableUsage bool,
) StreamConverter {
	return newResponsesToChatConverter(stream, responseModel, disableUsage)
}

func (c *responsesToChatConverter) Next() (interface{}, bool, error) {
	// Drain buffered events first
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
			if !c.completed {
				return nil, false, fmt.Errorf("responses stream ended without a terminal event")
			}
			return nil, true, nil
		}

		evt := c.stream.Current()
		c.processEvent(&evt)

		if len(c.pending) > 0 {
			evt := c.pending[0]
			c.pending = c.pending[1:]
			return evt, false, nil
		}
	}
}

func (c *responsesToChatConverter) Usage() *protocol.TokenUsage {
	return c.usage
}

// processEvent handles a single upstream event and appends resulting chunks to c.pending.
func (c *responsesToChatConverter) processEvent(evt *responses.ResponseStreamEventUnion) {
	switch evt.Type {
	case "response.created":
		c.chatID = evt.Response.ID
		c.serviceTier = protocol.ChatServiceTierFromResponses(evt.Response.ServiceTier)
		if !c.hasSentCreated {
			c.pending = append(c.pending, c.roleChunk())
		}

	case "response.output_text.delta", "response.refusal.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		c.emitTextPart(int(evt.OutputIndex), responsePartIndex(*evt), responseTextKind(evt.Type), evt.Delta, false)
	case "response.output_text.done", "response.refusal.done", "response.reasoning_text.done", "response.reasoning_summary_text.done":
		text := evt.Text
		if evt.Type == "response.refusal.done" {
			text = evt.Refusal
		}
		c.emitTextPart(int(evt.OutputIndex), responsePartIndex(*evt), responseTextKind(evt.Type), text, true)

	case "response.content_part.added", "response.content_part.done":
		kind, text := "text", evt.Part.Text
		if evt.Part.Type == "refusal" {
			kind, text = "refusal", evt.Part.Refusal
		}
		c.emitTextPart(int(evt.OutputIndex), int(evt.ContentIndex), kind, text, true)
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		c.emitTextPart(int(evt.OutputIndex), int(evt.SummaryIndex), "summary", evt.Part.Text, true)
	case "response.output_item.added":
		if evt.Item.Type == "function_call" {
			item := evt.Item
			initial := item.Arguments.OfString
			item.Arguments.OfString = ""
			c.emitOutputItem(int(evt.OutputIndex), item)
			c.toolCalls[int(evt.OutputIndex)].initialArguments = initial
		}

	case "response.function_call_arguments.delta":
		if !c.hasSentCreated {
			c.pending = append(c.pending, c.roleChunk())
		}
		index, ok := c.toolCallIndexes[evt.ItemID]
		if !ok {
			index = int(evt.OutputIndex)
		}
		if toolCall, ok := c.toolCalls[index]; ok {
			toolCall.arguments.WriteString(evt.Delta)
		}
		c.hasToolCalls = true
		if tc := c.toolCalls[index]; tc != nil {
			c.pending = append(c.pending, c.toolCallDeltaChunk(tc.chatIndex, evt.Delta))
		}

	case "response.function_call_arguments.done":
		index, ok := c.toolCallIndexes[evt.ItemID]
		if !ok {
			index = int(evt.OutputIndex)
		}
		if toolCall, ok := c.toolCalls[index]; ok {
			c.emitToolArguments(index, toolCall, evt.Arguments)
		}

	case "response.output_item.done":
		c.emitOutputItem(int(evt.OutputIndex), evt.Item)

	case "response.completed", "response.incomplete":
		// `response.incomplete` is a terminal Responses API event, not a
		// transport interruption. Long Codex tasks can legitimately stop here
		// because of max_output_tokens/content_filter while still carrying
		// assistant output and final usage. Treat it identically to
		// response.completed so the terminal chunk preserves output + usage
		// rather than falling through to the post-loop usage=0 fallback.
		c.usage = protocolusage.FromOpenAIResponses(evt.Response.Usage)
		// Override total_tokens with the value the upstream Responses stream
		// actually reported (per .design/usage-tracking.md — cover one
		// key, do not fork the constructor), falling back to the computed
		// input+cacheRead+output when the upstream did not report one.
		if evt.Response.Usage.TotalTokens != 0 {
			c.totalTokens = evt.Response.Usage.TotalTokens
		} else {
			c.totalTokens = int64(c.usage.InputTokens + c.usage.CacheReadTokens + c.usage.OutputTokens)
		}
		c.emitCompletedOutput(evt.Response.Output)
		for index, tc := range c.toolCalls {
			if tc.arguments.Len() == 0 {
				c.emitToolArguments(index, tc, tc.initialArguments)
			}
		}
		// The terminal response carries the tier actually used (created may
		// only say "auto") and any moderation verdict.
		if tier := protocol.ChatServiceTierFromResponses(evt.Response.ServiceTier); tier != nil {
			c.serviceTier = tier
		}
		finishReason := responsesToChatFinishReason(&evt.Response, c.hasToolCalls)
		final := c.finalChunk(finishReason)
		final.Moderation = protocol.ChatModerationFromResponses(evt.Response.Moderation)
		c.pending = append(c.pending, final)
		c.completed = true

	case "error":
		c.pending = append(c.pending, wire.ChatStreamErrorChunk{
			Error: wire.ChatStreamError{
				Message: evt.Message,
				Type:    "error",
				Code:    evt.Param,
			},
		})
	}

}

// emitCompletedOutput appends chunks for any output items not yet streamed
// (used when the provider sends all content in response.completed).
func (c *responsesToChatConverter) emitCompletedOutput(output []responses.ResponseOutputItemUnion) {
	for index, item := range output {
		c.emitOutputItem(index, item)
	}
}

// Snapshot events can carry fields omitted from deltas. Reconcile per item and
// content part; seeing one streamed item must not suppress every other item.
func (c *responsesToChatConverter) emitOutputItem(index int, item responses.ResponseOutputItemUnion) {
	switch item.Type {
	case "message":
		for partIndex, part := range item.Content {
			switch part.Type {
			case "output_text":
				c.emitTextPart(index, partIndex, "text", part.Text, true)
			case "refusal":
				c.emitTextPart(index, partIndex, "refusal", part.Refusal, true)
			}
		}
	case "reasoning":
		for partIndex, part := range item.Summary {
			c.emitTextPart(index, partIndex, "summary", part.Text, true)
		}
	case "function_call":
		tc, ok := c.toolCalls[index]
		if !ok {
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			tc = &responsesToChatToolCall{id: item.ID, callID: callID, name: item.Name, chatIndex: len(c.toolCalls)}
			c.toolCalls[index] = tc
			c.toolCallIndexes[item.ID] = index
			c.hasToolCalls = true
			if !c.hasSentCreated {
				c.pending = append(c.pending, c.roleChunk())
			}
			c.pending = append(c.pending, c.toolCallStartChunk(tc.chatIndex, callID, item.Name))
		}
		c.emitToolArguments(index, tc, item.Arguments.OfString)
	}
}

func (c *responsesToChatConverter) emitToolArguments(index int, tc *responsesToChatToolCall, snapshot string) {
	if suffix := snapshotSuffix(tc.arguments.String(), snapshot); suffix != "" {
		tc.arguments.WriteString(suffix)
		c.pending = append(c.pending, c.toolCallDeltaChunk(tc.chatIndex, suffix))
	}
}

func (c *responsesToChatConverter) emitTextPart(index, partIndex int, kind, text string, snapshot bool) {
	text = c.textParts.retain(index, partIndex, kind, text, snapshot)
	if text == "" {
		return
	}
	if !c.hasSentCreated {
		c.pending = append(c.pending, c.roleChunk())
	}
	delta := wire.ChatStreamDelta{}
	switch kind {
	case "refusal":
		delta.Refusal = text
	case "summary", "reasoning":
		delta.ReasoningContent = text
	default:
		delta.Content = text
	}
	c.pending = append(c.pending, c.newChunk(delta, nil))
}

// responsesToChatFinishReason maps a terminal Responses API response to the
// OpenAI Chat finish_reason. An incomplete
// response is mapped to "length"/"content_filter" by its incomplete reason.
func responsesToChatFinishReason(resp *responses.Response, hasToolCalls bool) string {
	if resp != nil && resp.Status == responses.ResponseStatusIncomplete {
		switch resp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return "length"
		case "content_filter":
			return "content_filter"
		}
	}
	if hasToolCalls {
		return openaiFinishReasonToolCalls
	}
	return "stop"
}

// chunk builders

func (c *responsesToChatConverter) roleChunk() wire.ChatStreamChunk {
	c.hasSentCreated = true
	return c.newChunk(wire.ChatStreamDelta{Role: "assistant"}, nil)
}

func (c *responsesToChatConverter) toolCallStartChunk(index int, id, name string) wire.ChatStreamChunk {
	args := ""
	return c.newChunk(wire.ChatStreamDelta{
		ToolCalls: []wire.ChatStreamToolCall{
			{Index: index, ID: id, Type: "function", Function: wire.ChatStreamToolFunction{Name: name, Arguments: &args}},
		},
	}, nil)
}

func (c *responsesToChatConverter) toolCallDeltaChunk(index int, delta string) wire.ChatStreamChunk {
	return c.newChunk(wire.ChatStreamDelta{
		ToolCalls: []wire.ChatStreamToolCall{
			{Index: index, Function: wire.ChatStreamToolFunction{Arguments: &delta}},
		},
	}, nil)
}

func (c *responsesToChatConverter) finalChunk(finishReason string) wire.ChatStreamChunk {
	chunk := c.newChunk(wire.ChatStreamDelta{}, &finishReason)
	if !c.disableUsage {
		// Same wire shape as the Anthropic→Chat path; only total_tokens is
		// overridden with the total the upstream Responses stream already
		// reported (captured into c.totalTokens), per the cover-one-key rule.
		usage := protocolusage.ToChatStreamUsageWire(c.Usage())
		usage.TotalTokens = c.totalTokens
		chunk.Usage = usage
	}
	return chunk
}

func (c *responsesToChatConverter) newChunk(delta wire.ChatStreamDelta, finishReason *string) wire.ChatStreamChunk {
	return wire.ChatStreamChunk{
		ID:      c.chatID,
		Object:  "chat.completion.chunk",
		Created: c.createdAt,
		Model:   c.responseModel,
		Choices: []wire.ChatStreamChoice{
			{Index: 0, Delta: delta, FinishReason: finishReason},
		},
		ServiceTier: c.serviceTier,
	}
}
