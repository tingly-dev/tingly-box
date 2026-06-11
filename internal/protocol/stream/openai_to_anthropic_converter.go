package stream

import (
	"fmt"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// anthropicStreamEvent wraps a single Anthropic SSE event for the converter pipeline.
type anthropicStreamEvent struct {
	eventType string
	data      map[string]interface{}
}

// openAIToAnthropicConverter is a stateful iterator that reads OpenAI Chat Completion
// chunks and emits Anthropic SSE events (map-based).
type openAIToAnthropicConverter struct {
	stream          *openaistream.Stream[openai.ChatCompletionChunk]
	responseModel   string
	req             *openai.ChatCompletionNewParams
	hooks           *OpenAIToAnthropicMCPHooks
	mapFinishReason func(string) string

	// state
	messageID            string
	estimatedInputTokens int
	messageStarted       bool
	tokenCounter         *token.StreamTokenCounter
	state                *streamState
	pendingFinishReason  string
	finishSeen           bool
	hookErr              error
	done                 bool
	pending              []interface{}
}

func newOpenAIToAnthropicConverter(
	stream *openaistream.Stream[openai.ChatCompletionChunk],
	responseModel string,
	req *openai.ChatCompletionNewParams,
	hooks *OpenAIToAnthropicMCPHooks,
	mapFinishReason func(string) string,
) *openAIToAnthropicConverter {
	c := &openAIToAnthropicConverter{
		stream:          stream,
		responseModel:   responseModel,
		req:             req,
		hooks:           hooks,
		mapFinishReason: mapFinishReason,
		messageID:       fmt.Sprintf("msg_%d", time.Now().Unix()),
		state:           newStreamState(),
	}
	// Initialize token counter; continue without it on error
	if tc, err := token.NewStreamTokenCounter(); err == nil {
		c.tokenCounter = tc
		if req != nil {
			if inputTokens, err := token.EstimateInputTokens(req); err == nil {
				c.tokenCounter.SetInputTokens(inputTokens)
				c.estimatedInputTokens = inputTokens
			}
		}
	}
	return c
}

func (c *openAIToAnthropicConverter) Next() (interface{}, bool, error) {
	if len(c.pending) > 0 {
		evt := c.pending[0]
		c.pending = c.pending[1:]
		return evt, false, nil
	}
	if c.done {
		return nil, true, nil
	}

	for {
		if !c.stream.Next() {
			if c.finishSeen && c.hookErr == nil {
				// Upstream finished normally.
				c.emitTerminalEvents()
			} else if c.messageStarted && c.hookErr == nil {
				// Upstream cut mid-stream after content started: surface an
				// honest error event rather than fabricating a clean end_turn.
				// Real SDK clients raise on it (the turn really was truncated);
				// lenient clients keep the partial content already sent.
				c.emitTruncatedError()
			}
			c.done = true
			if len(c.pending) > 0 {
				evt := c.pending[0]
				c.pending = c.pending[1:]
				return evt, false, nil
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
		if c.done {
			return nil, true, nil
		}
	}
}

func (c *openAIToAnthropicConverter) Usage() *protocol.TokenUsage {
	return protocol.NewTokenUsageFull(
		int(c.state.inputTokens),
		int(c.state.outputTokens),
		int(c.state.cacheTokens),
		int(c.state.reasoningTokens),
	)
}

func (c *openAIToAnthropicConverter) HookErr() error {
	return c.hookErr
}

func (c *openAIToAnthropicConverter) MessageStarted() bool {
	return c.messageStarted
}

func (c *openAIToAnthropicConverter) processChunk(chunk *openai.ChatCompletionChunk) {
	// Skip empty chunks; consume usage from token counter
	if len(chunk.Choices) == 0 {
		c.consumeTokenCounter(chunk)
		return
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	// Check for server_tool_use at chunk level
	if chunk.JSON.ExtraFields != nil {
		if serverToolUse, exists := chunk.JSON.ExtraFields["server_tool_use"]; exists && serverToolUse.Valid() {
			c.state.deltaExtras["server_tool_use"] = serverToolUse.Raw()
		}
	}

	// Collect extra fields
	if extras := parseRawJSON(delta.RawJSON()); extras != nil {
		extras = FilterOpenAIProtocolFields(extras)
		for k, v := range extras {
			if k == OpenaiFieldReasoningContent {
				if c.state.thinkingBlockIndex == -1 {
					c.state.thinkingBlockIndex = c.state.nextBlockIndex
					c.state.nextBlockIndex++
					c.state.thinkingBlocks[c.state.thinkingBlockIndex] = true
					c.emitEnsureMessageStart()
					c.emitContentBlockStart(c.state.thinkingBlockIndex, blockTypeThinking, map[string]interface{}{"thinking": ""})
				}
				if thinkingText := extractString(v); thinkingText != "" {
					c.emitEnsureMessageStart()
					c.emitContentBlockDelta(c.state.thinkingBlockIndex, map[string]interface{}{
						"type":     deltaTypeThinkingDelta,
						"thinking": thinkingText,
					})
				}
				continue
			}
			c.state.deltaExtras[k] = v
		}
	}

	// Handle refusal
	if delta.Refusal != "" {
		if c.state.textBlockIndex == -1 {
			c.emitCloseOpenBlock()
			c.state.textBlockIndex = c.state.nextBlockIndex
			c.state.nextBlockIndex++
			c.emitEnsureMessageStart()
			c.emitContentBlockStart(c.state.textBlockIndex, blockTypeText, map[string]interface{}{"text": ""})
		}
		c.state.hasTextContent = true
		c.emitEnsureMessageStart()
		c.emitContentBlockDelta(c.state.textBlockIndex, map[string]interface{}{
			"type": deltaTypeTextDelta,
			"text": delta.Refusal,
		})
	}

	// Handle content delta
	if delta.Content != "" {
		c.state.hasTextContent = true
		if c.state.textBlockIndex == -1 {
			c.emitCloseOpenBlock()
			c.state.textBlockIndex = c.state.nextBlockIndex
			c.state.nextBlockIndex++
			c.emitEnsureMessageStart()
			c.emitContentBlockStart(c.state.textBlockIndex, blockTypeText, map[string]interface{}{"text": ""})
		}
		c.emitEnsureMessageStart()
		c.emitContentBlockDelta(c.state.textBlockIndex, map[string]interface{}{
			"type": deltaTypeTextDelta,
			"text": delta.Content,
		})
	}

	// Handle tool_calls delta
	for _, toolCall := range delta.ToolCalls {
		openaiIndex := int(toolCall.Index)
		anthropicIndex, exists := c.state.toolIndexToBlockIndex[openaiIndex]
		if !exists {
			anthropicIndex = c.state.nextBlockIndex
			c.state.toolIndexToBlockIndex[openaiIndex] = anthropicIndex
			c.state.nextBlockIndex++
			truncatedID := rewriteToolCallIDForAnthropic(toolCall.ID)
			c.state.pendingToolCalls[anthropicIndex] = &pendingToolCall{
				id:   truncatedID,
				name: toolCall.Function.Name,
				emit: !(c.hooks != nil && c.hooks.ShouldSuppressTool != nil && c.hooks.ShouldSuppressTool(toolCall.Function.Name)),
			}
			c.emitCloseOpenBlock()
			if c.state.pendingToolCalls[anthropicIndex].emit {
				c.emitEnsureMessageStart()
				c.emitContentBlockStart(anthropicIndex, blockTypeToolUse, map[string]interface{}{
					"id":    truncatedID,
					"name":  toolCall.Function.Name,
					"input": map[string]interface{}{},
				})
			}
		}
		if toolCall.ID != "" {
			c.state.pendingToolCalls[anthropicIndex].id = truncateToolCallID(toolCall.ID)
		}
		if toolCall.Function.Name != "" {
			c.state.pendingToolCalls[anthropicIndex].name = toolCall.Function.Name
		}
		if toolCall.Function.Arguments != "" {
			c.state.pendingToolCalls[anthropicIndex].input += toolCall.Function.Arguments
			if c.state.pendingToolCalls[anthropicIndex].emit {
				c.emitEnsureMessageStart()
				c.emitContentBlockDelta(anthropicIndex, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": toolCall.Function.Arguments,
				})
			}
		}
	}

	c.consumeTokenCounter(chunk)

	// Handle finish_reason: record it but keep looping to consume trailing usage chunks.
	if choice.FinishReason != "" {
		c.pendingFinishReason = choice.FinishReason
		c.finishSeen = true
		if c.hooks != nil && c.hooks.OnToolCallsFinal != nil && len(c.state.pendingToolCalls) > 0 {
			toolCalls := make([]OpenAIToAnthropicToolCall, 0, len(c.state.pendingToolCalls))
			for _, tc := range c.state.pendingToolCalls {
				toolCalls = append(toolCalls, OpenAIToAnthropicToolCall{
					ID:        tc.id,
					Name:      tc.name,
					Arguments: tc.input,
				})
			}
			if err := c.hooks.OnToolCallsFinal(toolCalls); err != nil {
				c.hookErr = err
				c.done = true
				return
			}
		}
		if !c.messageStarted && isErr(c.hookErr, ErrMCPStreamContinue) {
			c.done = true
			return
		}
	}
}

func (c *openAIToAnthropicConverter) consumeTokenCounter(chunk *openai.ChatCompletionChunk) {
	if c.tokenCounter == nil {
		return
	}
	_, _, _ = c.tokenCounter.ConsumeOpenAIChunk(chunk)
	inputTokens, outputTokens := c.tokenCounter.GetCounts()
	if inputTokens > 0 {
		c.state.inputTokens = int64(inputTokens)
	}
	if outputTokens > 0 {
		c.state.outputTokens = int64(outputTokens)
	}
}

func (c *openAIToAnthropicConverter) emitTerminalEvents() {
	if c.tokenCounter != nil {
		inputTokens, outputTokens := c.tokenCounter.GetCounts()
		c.state.inputTokens = int64(inputTokens)
		c.state.outputTokens = int64(outputTokens)
		cacheTokens, reasoningTokens := c.tokenCounter.GetUpstreamDetails()
		if cacheTokens > 0 {
			c.state.cacheTokens = int64(cacheTokens)
		}
		if reasoningTokens > 0 {
			c.state.reasoningTokens = int64(reasoningTokens)
		}
	}
	logrus.Debugf("OpenAI->Anthropic stream usage: model=%s in=%d out=%d cache=%d reasoning=%d stop=%s",
		c.responseModel, c.state.inputTokens, c.state.outputTokens,
		c.state.cacheTokens, c.state.reasoningTokens, c.pendingFinishReason)
	c.emitEnsureMessageStart()
	c.emitStopEvents()
	stopReason := c.mapFinishReason(c.pendingFinishReason)
	c.emitMessageDelta(stopReason)
	c.emitMessageStop()
}

// emit helpers — these build the event maps and push to pending.

// emitTruncatedError queues an Anthropic `error` event for a stream that
// ended mid-content without a finish signal (truncated upstream).
func (c *openAIToAnthropicConverter) emitTruncatedError() {
	c.emitAnthropic("error", map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "stream_error",
			"message": "upstream stream ended before completion",
		},
	})
}

func (c *openAIToAnthropicConverter) emitAnthropic(eventType string, data map[string]interface{}) {
	c.pending = append(c.pending, anthropicStreamEvent{eventType: eventType, data: data})
}

func (c *openAIToAnthropicConverter) emitEnsureMessageStart() {
	if c.messageStarted {
		return
	}
	c.emitAnthropic(eventTypeMessageStart, map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            c.messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         c.responseModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  c.estimatedInputTokens,
				"output_tokens": 0,
			},
		},
	})
	c.messageStarted = true
}

func (c *openAIToAnthropicConverter) emitContentBlockStart(index int, blockType string, initialContent map[string]interface{}) {
	contentBlock := map[string]interface{}{"type": blockType}
	for k, v := range initialContent {
		contentBlock[k] = v
	}
	c.emitAnthropic(eventTypeContentBlockStart, map[string]interface{}{
		"type":          eventTypeContentBlockStart,
		"index":         index,
		"content_block": contentBlock,
	})
}

func (c *openAIToAnthropicConverter) emitContentBlockDelta(index int, content map[string]interface{}) {
	c.emitAnthropic(eventTypeContentBlockDelta, map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": content,
	})
}

func (c *openAIToAnthropicConverter) emitContentBlockStop(index int) {
	c.emitAnthropic(eventTypeContentBlockStop, map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	})
	c.state.stoppedBlocks[index] = true
}

func (c *openAIToAnthropicConverter) emitThinkingSignature(index int) {
	c.emitContentBlockDelta(index, map[string]interface{}{
		"type":      "signature_delta",
		"signature": GenerateObfuscationString(),
	})
}

func (c *openAIToAnthropicConverter) emitCloseOpenBlock() {
	if c.state.thinkingBlockIndex != -1 && !c.state.stoppedBlocks[c.state.thinkingBlockIndex] {
		c.emitThinkingSignature(c.state.thinkingBlockIndex)
		c.emitContentBlockStop(c.state.thinkingBlockIndex)
		c.state.thinkingBlockIndex = -1
		return
	}
	if c.state.reasoningSummaryBlockIndex != -1 && !c.state.stoppedBlocks[c.state.reasoningSummaryBlockIndex] {
		c.emitThinkingSignature(c.state.reasoningSummaryBlockIndex)
		c.emitContentBlockStop(c.state.reasoningSummaryBlockIndex)
		c.state.reasoningSummaryBlockIndex = -1
		return
	}
	if c.state.refusalBlockIndex != -1 && !c.state.stoppedBlocks[c.state.refusalBlockIndex] {
		c.emitContentBlockStop(c.state.refusalBlockIndex)
		c.state.refusalBlockIndex = -1
		return
	}
	if c.state.textBlockIndex != -1 && !c.state.stoppedBlocks[c.state.textBlockIndex] {
		c.emitContentBlockStop(c.state.textBlockIndex)
		c.state.textBlockIndex = -1
		return
	}
}

func (c *openAIToAnthropicConverter) emitStopEvents() {
	var blockIndices []int
	if c.state.thinkingBlockIndex != -1 && !c.state.stoppedBlocks[c.state.thinkingBlockIndex] {
		blockIndices = append(blockIndices, c.state.thinkingBlockIndex)
	}
	if c.state.refusalBlockIndex != -1 && !c.state.stoppedBlocks[c.state.refusalBlockIndex] {
		blockIndices = append(blockIndices, c.state.refusalBlockIndex)
	}
	if c.state.reasoningSummaryBlockIndex != -1 && !c.state.stoppedBlocks[c.state.reasoningSummaryBlockIndex] {
		blockIndices = append(blockIndices, c.state.reasoningSummaryBlockIndex)
	}
	if c.state.textBlockIndex != -1 && !c.state.stoppedBlocks[c.state.textBlockIndex] {
		blockIndices = append(blockIndices, c.state.textBlockIndex)
	}
	for i := range c.state.pendingToolCalls {
		if !c.state.stoppedBlocks[i] {
			blockIndices = append(blockIndices, i)
		}
	}
	sort.Ints(blockIndices)
	for _, idx := range blockIndices {
		if c.state.thinkingBlocks[idx] {
			c.emitThinkingSignature(idx)
		}
		c.emitContentBlockStop(idx)
	}
}

func (c *openAIToAnthropicConverter) emitMessageDelta(stopReason string) {
	deltaMap := map[string]interface{}{
		"stop_reason":   stopReason,
		"stop_sequence": nil,
	}
	for k, v := range c.state.deltaExtras {
		deltaMap[k] = v
	}
	usageMap := map[string]interface{}{
		"output_tokens": c.state.outputTokens,
	}
	if c.state.cacheTokens > 0 {
		usageMap["cache_read_input_tokens"] = c.state.cacheTokens
	}
	c.emitAnthropic(eventTypeMessageDelta, map[string]interface{}{
		"type":  eventTypeMessageDelta,
		"delta": deltaMap,
		"usage": usageMap,
	})
}

func (c *openAIToAnthropicConverter) emitMessageStop() {
	c.emitAnthropic(eventTypeMessageStop, map[string]interface{}{
		"type": eventTypeMessageStop,
	})
}

// anthropicSSEWriter returns a writer that sends Anthropic SSE events using
// c.SSEvent (no spaces after colons) and mirrors events to stream_event_recorder.
func anthropicSSEWriter(c *gin.Context) func(interface{}) error {
	return func(event interface{}) error {
		e, ok := event.(anthropicStreamEvent)
		if !ok {
			return nil
		}
		sendAnthropicStreamEvent(c, e.eventType, e.data, nopFlusher{})
		return nil
	}
}

// anthropicSSEWriterWithFirstChunk wraps anthropicSSEWriter and calls CommitFirstChunk
// on the first event, signalling that upstream is healthy before any byte hits the wire.
func anthropicSSEWriterWithFirstChunk(c *gin.Context) func(interface{}) error {
	first := true
	inner := anthropicSSEWriter(c)
	return func(event interface{}) error {
		if first {
			protocol.CommitFirstChunk(c)
			first = false
		}
		return inner(event)
	}
}

// nopFlusher satisfies http.Flusher with a no-op; gin's ResponseWriter handles flushing.
type nopFlusher struct{}

func (nopFlusher) Flush() {}

// isErr is a helper to avoid errors package in this file.
func isErr(err, target error) bool {
	if err == nil {
		return false
	}
	return err == target
}
