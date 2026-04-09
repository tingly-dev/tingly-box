package adapter

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
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

func (a *StreamAccumulator) IngestAnthropicEvent(evt *anthropic.MessageStreamEventUnion) {
	if evt == nil {
		return
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}
	data["type"] = evt.Type
	a.ingestEventMap(data)
}

func (a *StreamAccumulator) IngestAnthropicBetaEvent(evt *anthropic.BetaRawMessageStreamEventUnion) {
	if evt == nil {
		return
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}
	data["type"] = evt.Type
	a.ingestEventMap(data)
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
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	a.ingestEventMap(payload)
}

func (a *StreamAccumulator) Content() guardrailscore.Content {
	text := strings.TrimSpace(a.textBuilder.String())

	content := guardrailscore.Content{Text: text}
	if !a.commandFound {
		return content
	}

	cmd := &guardrailscore.Command{Name: a.commandName}
	args := strings.TrimSpace(a.commandArgs.String())
	if args != "" {
		cmd.Arguments = ParseToolArguments(args)
	}

	content.Command = cmd
	return content
}

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
	index := a.captureIndex(payload)

	switch eventType {
	case "content_block_delta":
		delta, _ := payload["delta"].(map[string]interface{})
		a.ingestDelta(index, delta)
	case "content_block_start":
		block, _ := payload["content_block"].(map[string]interface{})
		a.ingestContentBlock(index, block)
	case "content_block_stop":
		a.ingestContentBlockStop(index)
	case "response.output_text.delta":
		if delta, ok := payload["delta"].(string); ok {
			a.textBuilder.WriteString(delta)
		}
	case "response.output_text.done":
		if text, ok := payload["text"].(string); ok {
			a.textBuilder.WriteString(text)
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.mcp_call_arguments.delta":
		if delta, ok := payload["delta"].(string); ok {
			a.commandArgs.WriteString(delta)
			a.commandFound = true
		}
	case "response.function_call_arguments.done", "response.custom_tool_call_input.done", "response.mcp_call_arguments.done":
		if name, ok := payload["name"].(string); ok && name != "" {
			a.commandName = name
			a.commandFound = true
		}
	case "response.output_item.added":
		item, _ := payload["item"].(map[string]interface{})
		a.ingestOutputItem(item)
	case "response.completed":
		response, _ := payload["response"].(map[string]interface{})
		if output, ok := response["output"].([]interface{}); ok {
			for _, item := range output {
				if itemMap, ok := item.(map[string]interface{}); ok {
					a.ingestOutputItem(itemMap)
				}
			}
		}
	}
}

func (a *StreamAccumulator) ingestDelta(index int, delta map[string]interface{}) {
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

func (a *StreamAccumulator) ingestContentBlock(index int, block map[string]interface{}) {
	if block == nil {
		return
	}
	blockType, _ := block["type"].(string)
	if blockType != "tool_use" && blockType != "function_call" {
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

func (a *StreamAccumulator) ingestContentBlockStop(index int) {
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

func (a *StreamAccumulator) ingestOutputItem(item map[string]interface{}) {
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	if itemType != "function_call" && itemType != "custom_tool_call" && itemType != "mcp_call" {
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

func (a *StreamAccumulator) captureIndex(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	if raw, ok := payload["index"]; ok {
		switch v := raw.(type) {
		case float64:
			a.lastIndex = int(v)
			a.hasIndex = true
			return a.lastIndex
		case int:
			a.lastIndex = v
			a.hasIndex = true
			return a.lastIndex
		case int64:
			a.lastIndex = int(v)
			a.hasIndex = true
			return a.lastIndex
		}
	}
	return 0
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
