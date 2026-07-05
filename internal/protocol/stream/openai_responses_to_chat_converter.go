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
	accumulated     strings.Builder
	usage           *protocol.TokenUsage
	totalTokens     int64
	hasSentCreated  bool
	hasToolCalls    bool
	completed       bool
	toolCallIndexes map[string]int
	toolCalls       map[int]*responsesToChatToolCall

	// internal event queue
	pending []interface{}
}

type responsesToChatToolCall struct {
	id        string
	callID    string
	name      string
	arguments strings.Builder
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
	}
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
			// Stream ended without response.completed — emit fallback completion
			if !c.completed {
				c.emitFallbackCompletion()
				if len(c.pending) > 0 {
					evt := c.pending[0]
					c.pending = c.pending[1:]
					return evt, false, nil
				}
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
		if !c.hasSentCreated {
			c.pending = append(c.pending, c.roleChunk())
		}

	case "response.output_text.delta":
		if !c.hasSentCreated {
			c.pending = append(c.pending, c.roleChunk())
		}
		c.accumulated.WriteString(evt.Delta)
		c.pending = append(c.pending, c.textChunk(evt.Delta))

	case "response.output_text.done":
		// handled in response.completed

	case "response.output_item.added":
		if evt.Item.Type == "function_call" {
			if !c.hasSentCreated {
				c.pending = append(c.pending, c.roleChunk())
			}
			index := int(evt.OutputIndex)
			callID := evt.Item.CallID
			if callID == "" {
				callID = evt.Item.ID
			}
			c.toolCallIndexes[evt.Item.ID] = index
			c.toolCalls[index] = &responsesToChatToolCall{id: evt.Item.ID, callID: callID, name: evt.Item.Name}
			c.hasToolCalls = true
			c.pending = append(c.pending, c.toolCallStartChunk(index, callID, evt.Item.Name))
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
		c.pending = append(c.pending, c.toolCallDeltaChunk(index, evt.Delta))

	case "response.function_call_arguments.done":
		index, ok := c.toolCallIndexes[evt.ItemID]
		if !ok {
			index = int(evt.OutputIndex)
		}
		if toolCall, ok := c.toolCalls[index]; ok && toolCall.arguments.Len() == 0 {
			toolCall.arguments.WriteString(evt.Arguments)
		}

	case "response.output_item.done":
		if evt.Item.Type == "function_call" {
			index, ok := c.toolCallIndexes[evt.Item.ID]
			if !ok {
				index = int(evt.OutputIndex)
			}
			callID := evt.Item.CallID
			if callID == "" {
				callID = evt.Item.ID
			}
			toolCall, ok := c.toolCalls[index]
			if !ok {
				toolCall = &responsesToChatToolCall{id: evt.Item.ID}
				c.toolCalls[index] = toolCall
			}
			toolCall.id = evt.Item.ID
			toolCall.callID = callID
			toolCall.name = evt.Item.Name
			if toolCall.arguments.Len() == 0 {
				toolCall.arguments.WriteString(evt.Item.Arguments.OfString)
			}
			c.hasToolCalls = true
		}

	case "response.completed", "response.incomplete":
		// `response.incomplete` is a terminal Responses API event, not a
		// transport interruption. Long Codex tasks can legitimately stop here
		// because of max_output_tokens/content_filter while still carrying
		// assistant output and final usage. Treat it identically to
		// response.completed so the terminal chunk preserves output + usage
		// rather than falling through to the post-loop usage=0 fallback.
		c.usage = protocolusage.FromOpenAIResponses(evt.Response.Usage)
		if evt.Response.Usage.TotalTokens != 0 {
			c.totalTokens = evt.Response.Usage.TotalTokens
		} else {
			c.totalTokens = int64(c.usage.InputTokens + c.usage.CacheInputTokens + c.usage.OutputTokens)
		}
		c.emitCompletedOutput(evt.Response.Output)
		finishReason := responsesToChatFinishReason(&evt.Response, c.hasToolCalls)
		c.pending = append(c.pending, c.finalChunk(finishReason))
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
	if c.accumulated.Len() > 0 || len(c.toolCalls) > 0 {
		return
	}
	for outputIndex, item := range output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					if !c.hasSentCreated {
						c.pending = append(c.pending, c.roleChunk())
					}
					c.accumulated.WriteString(content.Text)
					c.pending = append(c.pending, c.textChunk(content.Text))
				}
			}
		case "function_call":
			if !c.hasSentCreated {
				c.pending = append(c.pending, c.roleChunk())
			}
			index := outputIndex
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			c.hasToolCalls = true
			c.pending = append(c.pending, c.toolCallStartChunk(index, callID, item.Name))
			if item.Arguments.OfString != "" {
				c.pending = append(c.pending, c.toolCallDeltaChunk(index, item.Arguments.OfString))
			}
		}
	}
}

func (c *responsesToChatConverter) emitFallbackCompletion() {
	// Mark completed first: this runs when the upstream stream ended without a
	// response.completed event (e.g. a truncated / mid-stream-cut stream). The
	// terminal chunk is emitted exactly once — without this flag the Next() loop
	// would re-enter on the next exhausted stream.Next() and re-emit the
	// fallback forever, flushing chunks unboundedly until the process OOMs.
	c.completed = true
	if !c.hasSentCreated {
		c.pending = append(c.pending, c.roleChunk())
	}
	finishReason := "stop"
	if c.hasToolCalls {
		finishReason = openaiFinishReasonToolCalls
	}
	c.pending = append(c.pending, c.finalChunk(finishReason))
}

// responsesToChatFinishReason maps a terminal Responses API response to the
// OpenAI Chat finish_reason. Tool calls take precedence; an incomplete
// response is mapped to "length"/"content_filter" by its incomplete reason.
func responsesToChatFinishReason(resp *responses.Response, hasToolCalls bool) string {
	if hasToolCalls {
		return openaiFinishReasonToolCalls
	}
	if resp != nil && resp.Status == responses.ResponseStatusIncomplete {
		switch resp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return "length"
		case "content_filter":
			return "content_filter"
		}
	}
	return "stop"
}

// chunk builders

func (c *responsesToChatConverter) roleChunk() wire.ChatStreamChunk {
	c.hasSentCreated = true
	return c.newChunk(wire.ChatStreamDelta{Role: "assistant"}, nil)
}

func (c *responsesToChatConverter) textChunk(delta string) wire.ChatStreamChunk {
	return c.newChunk(wire.ChatStreamDelta{Content: delta}, nil)
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
		u := c.Usage()
		totalInput := int64(u.InputTokens + u.CacheInputTokens)
		total := c.totalTokens
		if total == 0 {
			total = totalInput + int64(u.OutputTokens)
		}
		usage := &wire.ChatStreamUsage{
			PromptTokens:     totalInput,
			CompletionTokens: int64(u.OutputTokens),
			TotalTokens:      total,
		}
		if u.CacheInputTokens != 0 {
			usage.PromptTokensDetails = &wire.ChatStreamPromptTokenDetails{CachedTokens: int64(u.CacheInputTokens)}
		}
		if u.ReasoningTokens != 0 {
			usage.CompletionTokensDetails = &wire.ChatStreamOutputTokenDetails{ReasoningTokens: int64(u.ReasoningTokens)}
		}
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
	}
}
