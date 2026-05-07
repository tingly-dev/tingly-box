package adapter

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

const (
	streamEventContentBlockDelta = "content_block_delta"
	streamEventContentBlockStart = "content_block_start"
	streamEventContentBlockStop  = "content_block_stop"

	streamEventResponseOutputTextDelta      = "response.output_text.delta"
	streamEventResponseOutputTextDone       = "response.output_text.done"
	streamEventResponseFunctionArgsDelta    = "response.function_call_arguments.delta"
	streamEventResponseFunctionArgsDone     = "response.function_call_arguments.done"
	streamEventResponseCustomToolInputDelta = "response.custom_tool_call_input.delta"
	streamEventResponseCustomToolInputDone  = "response.custom_tool_call_input.done"
	streamEventResponseMCPArgsDelta         = "response.mcp_call_arguments.delta"
	streamEventResponseMCPArgsDone          = "response.mcp_call_arguments.done"
	streamEventResponseOutputItemAdded      = "response.output_item.added"
	streamEventResponseCompleted            = "response.completed"

	streamToolTypeAnthropicToolUse = "tool_use"
	streamToolTypeFunctionCall     = "function_call"
	streamToolTypeCustomToolCall   = "custom_tool_call"
	streamToolTypeMCPCall          = "mcp_call"
)

type StreamToolUse struct {
	Index int
	ID    string
	Name  string
	Args  string
}

type StreamAccumulator struct {
	textBuilder  strings.Builder
	commandName  string
	commandArgs  strings.Builder
	commandFound bool
	lastIndex    int
	hasIndex     bool
	lastToolID   string
	toolUses     map[int]*streamToolUseState
	completed    []StreamToolUse
}

type streamToolUseState struct {
	index int
	id    string
	name  string
	args  string
}

// Provider entry points.

func (a *StreamAccumulator) IngestAnthropicEvent(evt *anthropic.MessageStreamEventUnion) {
	if evt == nil {
		return
	}
	a.ingestRawJSON(evt.RawJSON())
}

func (a *StreamAccumulator) IngestAnthropicBetaEvent(evt *anthropic.BetaRawMessageStreamEventUnion) {
	if evt == nil {
		return
	}
	a.ingestRawJSON(evt.RawJSON())
}

func (a *StreamAccumulator) IngestOpenAIChatChunk(chunk *openai.ChatCompletionChunk) {
	if chunk == nil || len(chunk.Choices) == 0 {
		return
	}
	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		a.textBuilder.WriteString(choice.Delta.Content)
	}
	if choice.Delta.FunctionCall.Name != "" || choice.Delta.FunctionCall.Arguments != "" {
		if choice.Delta.FunctionCall.Name != "" {
			a.commandName = choice.Delta.FunctionCall.Name
			a.commandFound = true
		}
		if choice.Delta.FunctionCall.Arguments != "" {
			a.commandArgs.WriteString(choice.Delta.FunctionCall.Arguments)
			a.commandFound = true
		}
	}
	for _, toolCall := range choice.Delta.ToolCalls {
		if toolCall.Function.Name != "" {
			a.commandName = toolCall.Function.Name
			a.commandFound = true
		}
		if toolCall.Function.Arguments != "" {
			a.commandArgs.WriteString(toolCall.Function.Arguments)
			a.commandFound = true
		}
	}
}

func (a *StreamAccumulator) IngestOpenAIResponseEvent(evt *responses.ResponseStreamEventUnion) {
	if evt == nil {
		return
	}
	a.ingestRawJSON(evt.RawJSON())
}

func (a *StreamAccumulator) IngestMapEvent(event map[string]interface{}) {
	if event == nil {
		return
	}
	a.ingestEventMap(event)
}

func (a *StreamAccumulator) IngestAnyEvent(event interface{}) {
	if event == nil {
		return
	}
	switch evt := event.(type) {
	case interface{ RawJSON() string }:
		a.ingestRawJSON(evt.RawJSON())
	case map[string]interface{}:
		a.ingestEventMap(evt)
	}
}

// Public state accessors.

func (a *StreamAccumulator) NextBlockIndex() int {
	if a.hasIndex {
		return a.lastIndex + 1
	}
	return 0
}

func (a *StreamAccumulator) LastToolID() string {
	return a.lastToolID
}

func (a *StreamAccumulator) PopCompletedToolUse() (StreamToolUse, bool) {
	if len(a.completed) == 0 {
		return StreamToolUse{}, false
	}
	state := a.completed[0]
	a.completed = a.completed[1:]
	return state, true
}

// Generic JSON stream dispatch.

func (a *StreamAccumulator) ingestRawJSON(raw string) {
	if raw == "" {
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	a.ingestEventMap(payload)
}

func (a *StreamAccumulator) ingestEventMap(payload map[string]interface{}) {
	eventType, _ := payload["type"].(string)

	switch eventType {
	case streamEventContentBlockDelta, streamEventContentBlockStart, streamEventContentBlockStop:
		a.ingestAnthropicEventMap(payload)
	case streamEventResponseOutputTextDelta,
		streamEventResponseOutputTextDone,
		streamEventResponseFunctionArgsDelta,
		streamEventResponseCustomToolInputDelta,
		streamEventResponseMCPArgsDelta,
		streamEventResponseFunctionArgsDone,
		streamEventResponseCustomToolInputDone,
		streamEventResponseMCPArgsDone,
		streamEventResponseOutputItemAdded,
		streamEventResponseCompleted:
		a.ingestOpenAIResponsesEventMap(payload)
	}
}

// Provider-specific dispatch.

func (a *StreamAccumulator) ingestAnthropicEventMap(payload map[string]interface{}) {
	eventType, _ := payload["type"].(string)
	index := a.captureAnthropicIndex(payload)

	switch eventType {
	case streamEventContentBlockDelta:
		delta, _ := payload["delta"].(map[string]interface{})
		a.ingestAnthropicDelta(index, delta)
	case streamEventContentBlockStart:
		block, _ := payload["content_block"].(map[string]interface{})
		a.ingestAnthropicContentBlock(index, block)
	case streamEventContentBlockStop:
		a.completeToolUse(index)
	}
}

func (a *StreamAccumulator) ingestOpenAIResponsesEventMap(payload map[string]interface{}) {
	eventType, _ := payload["type"].(string)

	switch eventType {
	case streamEventResponseOutputTextDelta:
		if delta, ok := payload["delta"].(string); ok {
			a.textBuilder.WriteString(delta)
		}
	case streamEventResponseOutputTextDone:
		if text, ok := payload["text"].(string); ok {
			a.textBuilder.WriteString(text)
		}
	case streamEventResponseFunctionArgsDelta, streamEventResponseCustomToolInputDelta, streamEventResponseMCPArgsDelta:
		a.ingestOpenAIResponsesToolArgumentsDelta(payload)
	case streamEventResponseFunctionArgsDone, streamEventResponseCustomToolInputDone, streamEventResponseMCPArgsDone:
		a.ingestOpenAIResponsesToolArgumentsDone(payload)
	case streamEventResponseOutputItemAdded:
		a.ingestOpenAIResponsesOutputItemAdded(payload)
	case streamEventResponseCompleted:
		a.observeOpenAIResponsesCompleted(payload)
	}
}

// Anthropic stream event handling.

func (a *StreamAccumulator) ingestAnthropicDelta(index int, delta map[string]interface{}) {
	if delta == nil {
		return
	}
	deltaType, _ := delta["type"].(string)
	switch deltaType {
	case "text_delta":
		if text, ok := delta["text"].(string); ok {
			a.textBuilder.WriteString(text)
		}
	case "input_json_delta":
		if partial, ok := delta["partial_json"].(string); ok {
			if state := a.getOrCreateToolUse(index); state != nil {
				state.args += partial
			}
		}
	}
}

func (a *StreamAccumulator) ingestAnthropicContentBlock(index int, block map[string]interface{}) {
	if block == nil {
		return
	}
	blockType, _ := block["type"].(string)
	if blockType != streamToolTypeAnthropicToolUse && blockType != streamToolTypeFunctionCall {
		return
	}
	if id, ok := block["id"].(string); ok && id != "" {
		a.lastToolID = id
	}
	if name, ok := block["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
	}
	if input, ok := block["input"].(map[string]interface{}); ok {
		if len(input) > 0 {
			payload, err := json.Marshal(input)
			if err == nil {
				if state := a.getOrCreateToolUse(index); state != nil {
					state.args = string(payload)
				}
			}
		}
	}

	state := a.getOrCreateToolUse(index)
	if state != nil {
		state.id = a.lastToolID
		state.name = a.commandName
	}
}

// OpenAI Responses stream event handling.

func (a *StreamAccumulator) observeOpenAIResponsesToolItem(item map[string]interface{}) {
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	if !isStreamToolItemType(itemType) {
		return
	}
	if id, ok := item["id"].(string); ok && id != "" {
		a.lastToolID = id
	}
	if name, ok := item["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
	}
	if args, ok := item["arguments"].(string); ok && args != "" {
		a.commandArgs.WriteString(args)
		a.commandFound = true
	}
	if input, ok := item["input"].(string); ok && input != "" {
		a.commandArgs.WriteString(input)
		a.commandFound = true
	}
}

func (a *StreamAccumulator) ingestOpenAIResponsesToolArgumentsDelta(payload map[string]interface{}) {
	responseIndex := a.captureOpenAIResponsesOutputIndex(payload)
	if delta, ok := payload["delta"].(string); ok {
		a.commandArgs.WriteString(delta)
		a.commandFound = true
		if state := a.getOrCreateToolUse(responseIndex); state != nil {
			state.args += delta
		}
	}
}

func (a *StreamAccumulator) ingestOpenAIResponsesToolArgumentsDone(payload map[string]interface{}) {
	responseIndex := a.captureOpenAIResponsesOutputIndex(payload)
	state := a.getOrCreateToolUse(responseIndex)
	if state == nil {
		return
	}
	if itemID, ok := payload["item_id"].(string); ok && itemID != "" {
		a.lastToolID = itemID
		state.id = itemID
	}
	if name, ok := payload["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
		state.name = name
	}
	if args, ok := payload["arguments"].(string); ok {
		state.args = args
	}
	a.completeToolUse(responseIndex)
}

func (a *StreamAccumulator) ingestOpenAIResponsesOutputItemAdded(payload map[string]interface{}) {
	responseIndex := a.captureOpenAIResponsesOutputIndex(payload)
	item, _ := payload["item"].(map[string]interface{})
	a.mergeOpenAIResponsesToolItem(responseIndex, item)
	a.observeOpenAIResponsesToolItem(item)
}

func (a *StreamAccumulator) observeOpenAIResponsesCompleted(payload map[string]interface{}) {
	response, _ := payload["response"].(map[string]interface{})
	if output, ok := response["output"].([]interface{}); ok {
		for _, item := range output {
			if itemMap, ok := item.(map[string]interface{}); ok {
				a.observeOpenAIResponsesToolItem(itemMap)
			}
		}
	}
}

// Provider-specific index extraction.

func (a *StreamAccumulator) captureAnthropicIndex(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	if index, ok := numericIndex(payload, "index"); ok {
		a.rememberIndex(index)
		return index
	}
	return 0
}

func (a *StreamAccumulator) captureOpenAIResponsesOutputIndex(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	if index, ok := numericIndex(payload, "output_index"); ok {
		a.rememberIndex(index)
		return index
	}
	return a.captureAnthropicIndex(payload)
}

// Shared tool-use state management.

func (a *StreamAccumulator) mergeOpenAIResponsesToolItem(index int, item map[string]interface{}) {
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	if !isStreamToolItemType(itemType) {
		return
	}
	state := a.getOrCreateToolUse(index)
	if state == nil {
		return
	}
	if id, ok := item["id"].(string); ok && id != "" {
		state.id = id
		a.lastToolID = id
	}
	if name, ok := item["name"].(string); ok && name != "" {
		state.name = name
	}
	if args, ok := item["arguments"].(string); ok && args != "" {
		state.args = args
	}
	if input, ok := item["input"].(string); ok && input != "" {
		state.args = input
	}
}

func (a *StreamAccumulator) completeToolUse(index int) {
	if a.toolUses == nil {
		return
	}
	state, ok := a.toolUses[index]
	if !ok {
		return
	}
	a.completed = append(a.completed, StreamToolUse{
		Index: state.index,
		ID:    state.id,
		Name:  state.name,
		Args:  state.args,
	})
	delete(a.toolUses, index)
}

func (a *StreamAccumulator) getOrCreateToolUse(index int) *streamToolUseState {
	if a.toolUses == nil {
		a.toolUses = make(map[int]*streamToolUseState)
	}
	if existing, ok := a.toolUses[index]; ok {
		return existing
	}
	state := &streamToolUseState{index: index}
	a.toolUses[index] = state
	return state
}

func (a *StreamAccumulator) rememberIndex(index int) {
	a.lastIndex = index
	a.hasIndex = true
}

func numericIndex(payload map[string]interface{}, key string) (int, bool) {
	raw, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	default:
		return 0, false
	}
}

func isStreamToolItemType(itemType string) bool {
	return itemType == streamToolTypeFunctionCall ||
		itemType == streamToolTypeCustomToolCall ||
		itemType == streamToolTypeMCPCall
}
