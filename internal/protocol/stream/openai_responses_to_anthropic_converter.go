package stream

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// responsesToAnthropicToolCall tracks a Responses API tool call being assembled from stream events.
type responsesToAnthropicToolCall struct {
	blockIndex  int
	itemID      string
	truncatedID string
	name        string
	completed   bool
}

// responsesToAnthropicConverter is a stateful iterator that reads OpenAI Responses API
// stream events and emits Anthropic SSE events.
type responsesToAnthropicConverter struct {
	ctx           context.Context
	stream        ResponsesStreamIter
	responseModel string

	// message state
	messageStartSent   bool
	messageID          string
	state              *streamState
	toolCalls          map[string]*responsesToAnthropicToolCall
	lastOutputItemType string

	// iterator
	pending []interface{}
	done    bool
	convErr error // set on protocol-level errors (response.failed, etc.)

	// usage (set at response.completed)
	usage *protocol.TokenUsage
}

func newResponsesToAnthropicConverter(ctx context.Context, stream ResponsesStreamIter, responseModel string) *responsesToAnthropicConverter {
	return &responsesToAnthropicConverter{
		ctx:           ctx,
		stream:        stream,
		responseModel: responseModel,
		messageID:     fmt.Sprintf("msg_%d", time.Now().Unix()),
		state:         newStreamState(),
		toolCalls:     make(map[string]*responsesToAnthropicToolCall),
		usage:         protocol.ZeroTokenUsage(),
	}
}

func (r *responsesToAnthropicConverter) Next() (interface{}, bool, error) {
	if !r.messageStartSent {
		r.emitMessageStart()
		r.messageStartSent = true
	}

	if len(r.pending) > 0 {
		evt := r.pending[0]
		r.pending = r.pending[1:]
		return evt, false, nil
	}

	if r.done {
		// convErr accessible via HookErr(); return done with no error so
		// ProcessStream calls OnStreamCompleteHooks rather than OnStreamErrorHooks.
		return nil, true, nil
	}

	for {
		if !r.stream.Next() {
			// Upstream cut without a terminal Responses event: surface an
			// honest error event rather than fabricating a clean message_stop.
			// Real SDK clients raise on it (the turn was truncated); lenient
			// clients keep the partial content already sent.
			if r.convErr == nil && !r.done {
				r.emitAnthropic("error", map[string]interface{}{
					"type": "error",
					"error": map[string]interface{}{
						"type":    "stream_error",
						"message": "upstream stream ended before completion",
					},
				})
			}
			r.done = true
			if len(r.pending) > 0 {
				evt := r.pending[0]
				r.pending = r.pending[1:]
				return evt, false, nil
			}
			return nil, true, nil
		}
		r.processEvent(r.stream.Current())

		if len(r.pending) > 0 {
			evt := r.pending[0]
			r.pending = r.pending[1:]
			return evt, false, nil
		}
		if r.done {
			return nil, true, nil
		}
	}
}

func (r *responsesToAnthropicConverter) Usage() *protocol.TokenUsage {
	return r.usage
}

// HookErr returns a protocol-level stream error (e.g. response.failed) if one occurred.
// The error SSE event has already been emitted to the client before this is set.
func (r *responsesToAnthropicConverter) HookErr() error { return r.convErr }

// MessageStarted reports whether the message_start event has been queued.
func (r *responsesToAnthropicConverter) MessageStarted() bool { return r.messageStartSent }

// emit helpers

func (r *responsesToAnthropicConverter) emitAnthropic(eventType string, data any) {
	r.pending = append(r.pending, anthropicStreamEvent{eventType: eventType, data: data})
}

func (r *responsesToAnthropicConverter) emitMessageStart() {
	// MENTION: usage in message_start is a placeholder only
	r.emitAnthropic(eventTypeMessageStart, newAnthropicMessageStartEvent(r.messageID, r.responseModel, 0))
}

func (r *responsesToAnthropicConverter) emitContentBlockStart(index int, block anthropicWireContentBlock) {
	r.emitAnthropic(eventTypeContentBlockStart, anthropicContentBlockStartEvent{
		Type:         eventTypeContentBlockStart,
		Index:        index,
		ContentBlock: block,
	})
}

func (r *responsesToAnthropicConverter) emitContentBlockDelta(index int, delta anthropicWireDelta) {
	r.emitAnthropic(eventTypeContentBlockDelta, anthropicContentBlockDeltaEvent{
		Type:  eventTypeContentBlockDelta,
		Index: index,
		Delta: delta,
	})
}

func (r *responsesToAnthropicConverter) emitContentBlockStop(index int) {
	r.emitAnthropic(eventTypeContentBlockStop, anthropicContentBlockStopEvent{
		Type:  eventTypeContentBlockStop,
		Index: index,
	})
	r.state.stoppedBlocks[index] = true
}

func (r *responsesToAnthropicConverter) emitThinkingSignature(index int) {
	r.emitContentBlockDelta(index, anthropicSignatureDelta(GenerateObfuscationString()))
}

func (r *responsesToAnthropicConverter) emitStopEvents() {
	var blockIndices []int
	if r.state.thinkingBlockIndex != -1 && !r.state.stoppedBlocks[r.state.thinkingBlockIndex] {
		blockIndices = append(blockIndices, r.state.thinkingBlockIndex)
	}
	if r.state.refusalBlockIndex != -1 && !r.state.stoppedBlocks[r.state.refusalBlockIndex] {
		blockIndices = append(blockIndices, r.state.refusalBlockIndex)
	}
	if r.state.reasoningSummaryBlockIndex != -1 && !r.state.stoppedBlocks[r.state.reasoningSummaryBlockIndex] {
		blockIndices = append(blockIndices, r.state.reasoningSummaryBlockIndex)
	}
	if r.state.textBlockIndex != -1 && !r.state.stoppedBlocks[r.state.textBlockIndex] {
		blockIndices = append(blockIndices, r.state.textBlockIndex)
	}
	for i := range r.state.pendingToolCalls {
		if !r.state.stoppedBlocks[i] {
			blockIndices = append(blockIndices, i)
		}
	}
	sort.Ints(blockIndices)
	for _, idx := range blockIndices {
		if r.state.thinkingBlocks[idx] {
			r.emitThinkingSignature(idx)
		}
		r.emitContentBlockStop(idx)
	}
}

func (r *responsesToAnthropicConverter) emitMessageDelta(stopReason string) {
	deltaMap := map[string]interface{}{
		"stop_reason":   stopReason,
		"stop_sequence": nil,
	}
	for k, v := range r.state.deltaExtras {
		deltaMap[k] = v
	}
	usageMap := r.Usage().ToAnthropicMessageDeltaUsageMap()
	r.emitAnthropic(eventTypeMessageDelta, map[string]interface{}{
		"type":  eventTypeMessageDelta,
		"delta": deltaMap,
		"usage": usageMap,
	})
}

func (r *responsesToAnthropicConverter) emitMessageStop() {
	r.emitAnthropic(eventTypeMessageStop, anthropicMessageStopEvent{Type: eventTypeMessageStop})
	r.done = true
}

func (r *responsesToAnthropicConverter) processEvent(currentEvent responses.ResponseStreamEventUnion) {
	switch currentEvent.Type {
	case "response.created", "response.in_progress", "response.queued":
		return

	case "response.content_part.added":
		partAdded := currentEvent.AsResponseContentPartAdded()
		if partAdded.Part.Type == "output_text" {
			if r.state.textBlockIndex == -1 {
				r.state.textBlockIndex = r.state.nextBlockIndex
				r.state.hasTextContent = true
				r.state.nextBlockIndex++
				r.emitContentBlockStart(r.state.textBlockIndex, anthropicTextBlockStart())
			}
			if partAdded.Part.Text != "" {
				r.emitContentBlockDelta(r.state.textBlockIndex, anthropicTextDelta(partAdded.Part.Text))
			}
			r.lastOutputItemType = "text"
		}

	case "response.output_text.delta":
		if r.state.textBlockIndex == -1 {
			r.state.textBlockIndex = r.state.nextBlockIndex
			r.state.hasTextContent = true
			r.state.nextBlockIndex++
			r.emitContentBlockStart(r.state.textBlockIndex, anthropicTextBlockStart())
		}
		textDelta := currentEvent.AsResponseOutputTextDelta()
		r.emitContentBlockDelta(r.state.textBlockIndex, anthropicTextDelta(textDelta.Delta))
		r.lastOutputItemType = "text"

	case "response.output_text.done", "response.content_part.done":
		if r.state.textBlockIndex != -1 {
			r.emitContentBlockStop(r.state.textBlockIndex)
			r.state.textBlockIndex = -1
		}

	case "response.reasoning_text.delta":
		reasoningDelta := currentEvent.AsResponseReasoningTextDelta()
		if r.state.thinkingBlockIndex == -1 {
			r.state.thinkingBlockIndex = r.state.nextBlockIndex
			r.state.nextBlockIndex++
			r.state.thinkingBlocks[r.state.thinkingBlockIndex] = true
			logrus.WithContext(r.ctx).Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", r.state.thinkingBlockIndex)
			r.emitContentBlockStart(r.state.thinkingBlockIndex, anthropicThinkingBlockStart())
		}
		r.emitContentBlockDelta(r.state.thinkingBlockIndex, anthropicThinkingDelta(reasoningDelta.Delta))

	case "response.reasoning_text.done":
		logrus.WithContext(r.ctx).Debugf("[Thinking][ResponsesAPI] Thinking block done at index %d", r.state.thinkingBlockIndex)
		if r.state.thinkingBlockIndex != -1 && !r.state.stoppedBlocks[r.state.thinkingBlockIndex] {
			r.emitThinkingSignature(r.state.thinkingBlockIndex)
			r.emitContentBlockStop(r.state.thinkingBlockIndex)
			r.state.thinkingBlockIndex = -1
		}

	case "response.reasoning_summary_text.delta":
		summaryDelta := currentEvent.AsResponseReasoningSummaryTextDelta()
		if r.state.reasoningSummaryBlockIndex == -1 {
			r.state.reasoningSummaryBlockIndex = r.state.nextBlockIndex
			r.state.hasTextContent = true
			r.state.nextBlockIndex++
			r.state.thinkingBlocks[r.state.reasoningSummaryBlockIndex] = true
			r.emitContentBlockStart(r.state.reasoningSummaryBlockIndex, anthropicThinkingBlockStart())
		}
		r.emitContentBlockDelta(r.state.reasoningSummaryBlockIndex, anthropicThinkingDelta(summaryDelta.Delta))

	case "response.reasoning_summary_text.done":
		if r.state.reasoningSummaryBlockIndex != -1 && !r.state.stoppedBlocks[r.state.reasoningSummaryBlockIndex] {
			r.emitThinkingSignature(r.state.reasoningSummaryBlockIndex)
			r.emitContentBlockStop(r.state.reasoningSummaryBlockIndex)
			r.state.reasoningSummaryBlockIndex = -1
		}

	case "response.refusal.delta":
		refusalDelta := currentEvent.AsResponseRefusalDelta()
		if r.state.refusalBlockIndex == -1 {
			r.state.refusalBlockIndex = r.state.nextBlockIndex
			r.state.nextBlockIndex++
			r.emitContentBlockStart(r.state.refusalBlockIndex, anthropicTextBlockStart())
		}
		r.emitContentBlockDelta(r.state.refusalBlockIndex, anthropicTextDelta(refusalDelta.Delta))

	case "response.refusal.done":
		if r.state.refusalBlockIndex != -1 {
			r.emitContentBlockStop(r.state.refusalBlockIndex)
			r.state.refusalBlockIndex = -1
		}

	case "response.output_item.added":
		itemAdded := currentEvent.AsResponseOutputItemAdded()
		switch itemAdded.Item.Type {
		case "reasoning":
			reasoningDelta := currentEvent.AsResponseReasoningTextDelta()
			if r.state.thinkingBlockIndex == -1 {
				r.state.thinkingBlockIndex = r.state.nextBlockIndex
				r.state.nextBlockIndex++
				r.state.thinkingBlocks[r.state.thinkingBlockIndex] = true
				logrus.WithContext(r.ctx).Debugf("[Thinking][ResponsesAPI] Initializing thinking block at index %d", r.state.thinkingBlockIndex)
				r.emitContentBlockStart(r.state.thinkingBlockIndex, anthropicThinkingBlockStart())
			}
			r.emitContentBlockDelta(r.state.thinkingBlockIndex, anthropicThinkingDelta(reasoningDelta.Delta))
		case "message":
			if r.state.textBlockIndex == -1 {
				r.state.textBlockIndex = r.state.nextBlockIndex
				r.state.hasTextContent = true
				r.state.nextBlockIndex++
				r.emitContentBlockStart(r.state.textBlockIndex, anthropicTextBlockStart())
			}
			textDelta := currentEvent.AsResponseOutputTextDelta()
			r.emitContentBlockDelta(r.state.textBlockIndex, anthropicTextDelta(textDelta.Delta))
		case "function_call", "custom_tool_call", "mcp_call":
			itemID := itemAdded.Item.ID
			truncatedID := truncateToolCallID(itemID)
			blockIndex := r.state.nextBlockIndex
			r.state.nextBlockIndex++

			toolName := itemAdded.Item.Name
			r.toolCalls[itemID] = &responsesToAnthropicToolCall{
				blockIndex:  blockIndex,
				itemID:      itemID,
				truncatedID: truncatedID,
				name:        toolName,
			}
			r.lastOutputItemType = "function_call"
			r.emitContentBlockStart(blockIndex, anthropicToolUseBlockStart(truncatedID, toolName))
		default:
			logrus.WithContext(r.ctx).Warnf("missing process for stream chunk: %s, %s", itemAdded.Type, itemAdded.Item.Type)
		}

	case "response.function_call_arguments.delta":
		argsDelta := currentEvent.AsResponseFunctionCallArgumentsDelta()
		if tc, ok := r.toolCalls[argsDelta.ItemID]; ok {
			r.emitContentBlockDelta(tc.blockIndex, anthropicInputJSONDelta(argsDelta.Delta))
		}

	case "response.function_call_arguments.done":
		argsDone := currentEvent.AsResponseFunctionCallArgumentsDone()
		if tc, ok := r.toolCalls[argsDone.ItemID]; ok {
			if tc.name == "" && argsDone.Name != "" {
				tc.name = argsDone.Name
			}
			r.emitContentBlockStop(tc.blockIndex)
			tc.completed = true
		}

	case "response.custom_tool_call_input.delta":
		customDelta := currentEvent.AsResponseCustomToolCallInputDelta()
		if tc, ok := r.toolCalls[customDelta.ItemID]; ok {
			r.emitContentBlockDelta(tc.blockIndex, anthropicInputJSONDelta(customDelta.Delta))
		}

	case "response.custom_tool_call_input.done":
		customDone := currentEvent.AsResponseCustomToolCallInputDone()
		if tc, ok := r.toolCalls[customDone.ItemID]; ok {
			r.emitContentBlockStop(tc.blockIndex)
			tc.completed = true
		}

	case "response.mcp_call_arguments.delta":
		mcpDelta := currentEvent.AsResponseMcpCallArgumentsDelta()
		if tc, ok := r.toolCalls[mcpDelta.ItemID]; ok {
			r.emitContentBlockDelta(tc.blockIndex, anthropicInputJSONDelta(mcpDelta.Delta))
		}

	case "response.mcp_call_arguments.done":
		mcpDone := currentEvent.AsResponseMcpCallArgumentsDone()
		if tc, ok := r.toolCalls[mcpDone.ItemID]; ok {
			r.emitContentBlockStop(tc.blockIndex)
			tc.completed = true
		}

	case "response.output_item.done":
		// handled by respective .done events above

	case "response.completed":
		completed := currentEvent.AsResponseCompleted()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Response completed")
		r.finalize(&completed.Response, "")

	case "response.incomplete":
		// response.incomplete is a terminal Responses status (max_output_tokens,
		// content_filter), not a transport error. Preserve the partial output and
		// map to Anthropic's stop reason (max_tokens, or end_turn for content_filter).
		incomplete := currentEvent.AsResponseIncomplete()
		stopReason := anthropicStopReasonMaxTokens
		if incomplete.Response.IncompleteDetails.Reason == "content_filter" {
			stopReason = anthropicStopReasonEndTurn
		}
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Response incomplete: reason=%s, stop_reason=%s", incomplete.Response.IncompleteDetails.Reason, stopReason)
		r.finalize(&incomplete.Response, stopReason)

	case "response.output_text.annotation.added":
		// pass-through silently

	case "response.text.done":
		// finalized by content_part.done

	case "response.reasoning_summary_part.added":
		summaryPartAdded := currentEvent.AsResponseReasoningSummaryPartAdded()
		if summaryPartAdded.Part.Type == "text" {
			if r.state.reasoningSummaryBlockIndex == -1 {
				r.state.reasoningSummaryBlockIndex = r.state.nextBlockIndex
				r.state.hasTextContent = true
				r.state.nextBlockIndex++
				r.state.thinkingBlocks[r.state.reasoningSummaryBlockIndex] = true
				r.emitContentBlockStart(r.state.reasoningSummaryBlockIndex, anthropicThinkingBlockStart())
			}
			if summaryPartAdded.Part.Text != "" {
				r.emitContentBlockDelta(r.state.reasoningSummaryBlockIndex, anthropicThinkingDelta(summaryPartAdded.Part.Text))
			}
		}

	case "response.reasoning_summary_part.done":
		if r.state.reasoningSummaryBlockIndex != -1 && !r.state.stoppedBlocks[r.state.reasoningSummaryBlockIndex] {
			r.emitThinkingSignature(r.state.reasoningSummaryBlockIndex)
			r.emitContentBlockStop(r.state.reasoningSummaryBlockIndex)
			r.state.reasoningSummaryBlockIndex = -1
		}

	case "response.audio.delta", "response.audio.done",
		"response.audio.transcript.delta", "response.audio.transcript.done",
		"response.code_interpreter_call_code.delta", "response.code_interpreter_call_code.done":
		// ignore silently

	case "response.code_interpreter_call.in_progress":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Code interpreter in progress")

	case "response.code_interpreter_call.interpreting":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Code interpreter interpreting")

	case "response.code_interpreter_call.completed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Code interpreter completed")

	case "response.file_search_call.in_progress":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] File search in progress")

	case "response.file_search_call.searching":
		// status event; elided

	case "response.file_search_call.completed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] File search completed")

	case "response.web_search_call.in_progress":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Web search in progress")

	case "response.web_search_call.searching":
		searching := currentEvent.AsResponseWebSearchCallSearching()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Web search searching: %v", searching)

	case "response.web_search_call.completed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Web search completed")

	case "response.image_generation_call.in_progress":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Image generation in progress")

	case "response.image_generation_call.generating":
		generating := currentEvent.AsResponseImageGenerationCallGenerating()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Image generation generating: %v", generating)

	case "response.image_generation_call.partial_image":
		partial := currentEvent.AsResponseImageGenerationCallPartialImage()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Image generation partial: index=%d", partial.PartialImageIndex)

	case "response.image_generation_call.completed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Image generation completed")

	case "response.mcp_call.in_progress":
		mcpInProgress := currentEvent.AsResponseMcpCallInProgress()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP call in progress: %v", mcpInProgress)

	case "response.mcp_call.completed":
		mcpCompleted := currentEvent.AsResponseMcpCallCompleted()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP call completed: %v", mcpCompleted)

	case "response.mcp_call.failed":
		mcpFailed := currentEvent.AsResponseMcpCallFailed()
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP call failed: %v", mcpFailed)

	case "response.mcp_list_tools.in_progress":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP list tools in progress")

	case "response.mcp_list_tools.completed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP list tools completed")

	case "response.mcp_list_tools.failed":
		logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] MCP list tools failed")

	case "error", "response.failed":
		logrus.WithContext(r.ctx).Errorf("Responses API error event: %v", currentEvent)
		errMsg := fmt.Sprintf("Responses API error: %v", currentEvent)
		r.emitAnthropic("error", map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": errMsg,
				"type":    "api_error",
			},
		})
		r.convErr = fmt.Errorf("%s", errMsg)
		r.done = true

	default:
		logrus.WithContext(r.ctx).Debugf("Unhandled Responses API event type: %s", currentEvent.Type)
	}
}

// finalize emits the terminal Anthropic events for a completed or incomplete
// Responses API response: it records usage, backfills any non-streamed tool
// calls from the output array, closes open blocks, and sends message_delta +
// message_stop. When stopReasonOverride is empty the stop reason is derived
// from the streamed content (tool_use vs end_turn); otherwise the override is
// used (e.g. max_tokens for an incomplete response).
func (r *responsesToAnthropicConverter) finalize(resp *responses.Response, stopReasonOverride string) {
	r.usage = usage.FromOpenAIResponses(resp.Usage)

	logrus.WithContext(r.ctx).Debugf("[ResponsesAPI] Finalize: input_tokens=%d, output_tokens=%d", r.usage.InputTokens, r.usage.OutputTokens)

	// Handle non-streamed tool calls from the final output array.
	for _, outputItem := range resp.Output {
		if outputItem.Type != "function_call" && outputItem.Type != "custom_tool_call" && outputItem.Type != "mcp_call" {
			continue
		}
		itemID := outputItem.ID
		if _, wasProcessed := r.toolCalls[itemID]; wasProcessed {
			continue
		}

		truncatedID := truncateToolCallID(itemID)
		blockIndex := r.state.nextBlockIndex
		r.state.nextBlockIndex++

		var toolName, arguments string
		switch outputItem.Type {
		case "function_call":
			fn := outputItem.AsFunctionCall()
			toolName = fn.Name
			arguments = fn.Arguments
		case "custom_tool_call":
			cc := outputItem.AsCustomToolCall()
			toolName = cc.Name
			arguments = cc.Input
		case "mcp_call":
			mc := outputItem.AsMcpCall()
			toolName = mc.Name
			arguments = mc.Arguments
		}

		r.lastOutputItemType = "function_call"
		r.emitContentBlockStart(blockIndex, anthropicToolUseBlockStart(truncatedID, toolName))
		if arguments != "" {
			r.emitContentBlockDelta(blockIndex, anthropicInputJSONDelta(arguments))
		}
		r.emitContentBlockStop(blockIndex)
	}

	r.emitStopEvents()

	stopReason := stopReasonOverride
	if stopReason == "" {
		stopReason = anthropicStopReasonEndTurn
		if r.lastOutputItemType == "function_call" {
			stopReason = anthropicStopReasonToolUse
		}
	}
	r.emitMessageDelta(stopReason)
	r.emitMessageStop() // sets r.done = true
}
