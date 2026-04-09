package mutate

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

const (
	anthropicEventTypeContentBlockStart = "content_block_start"
	anthropicEventTypeContentBlockDelta = "content_block_delta"
	anthropicEventTypeContentBlockStop  = "content_block_stop"
	anthropicDeltaTypeTextDelta         = "text_delta"
	anthropicDeltaTypeInputJSONDelta    = "input_json_delta"
)

type AnthropicBufferedEvent struct {
	EventType string
	Payload   map[string]interface{}
}

type anthropicToolUseBufferState struct {
	ByIndex       map[int][]AnthropicBufferedEvent
	ToolIDByIndex map[int]string
}

type anthropicGuardrailsBlockState struct {
	ToolMessages map[string]string
	BlockedIndex map[int]string
}

func RegisterAnthropicGuardrailsBlock(c *gin.Context, toolID string, index int, message string) {
	if toolID == "" || message == "" {
		return
	}
	state := getAnthropicGuardrailsBlockState(c)
	state.ToolMessages[toolID] = message
	state.BlockedIndex[index] = toolID
}

func ShouldRewriteAnthropicEvent(c *gin.Context, eventType string, block interface{}) bool {
	switch eventType {
	case anthropicEventTypeContentBlockStart:
		blockType, _ := extractAnthropicBlockTypeAndID(block)
		if blockType == "tool_use" || blockType == "text" {
			return true
		}
	case anthropicEventTypeContentBlockDelta:
		state := getAnthropicToolUseBufferState(c)
		if len(state.ByIndex) > 0 {
			return true
		}
		if maskState := getAnthropicCredentialMaskState(c); maskState != nil && len(maskState.AliasToReal) > 0 {
			return true
		}
	case anthropicEventTypeContentBlockStop:
		state := getAnthropicToolUseBufferState(c)
		if len(state.ByIndex) > 0 {
			return true
		}
	}
	return false
}

func RestoreCredentialAliasesInAnthropicEventMap(c *gin.Context, eventMap map[string]interface{}) {
	state := getAnthropicCredentialMaskState(c)
	if state == nil || len(state.AliasToReal) == 0 || eventMap == nil {
		return
	}
	eventType, _ := eventMap["type"].(string)
	switch eventType {
	case anthropicEventTypeContentBlockDelta:
		delta, _ := eventMap["delta"].(map[string]interface{})
		deltaType, _ := delta["type"].(string)
		if deltaType == anthropicDeltaTypeTextDelta {
			if text, ok := delta["text"].(string); ok {
				if !guardrailscore.MayContainAliasToken(text) {
					return
				}
				if restored, changed := guardrailscore.RestoreText(text, state); changed {
					delta["text"] = restored
				}
			}
		}
	case anthropicEventTypeContentBlockStart:
		contentBlock, _ := eventMap["content_block"].(map[string]interface{})
		if blockType, _ := contentBlock["type"].(string); blockType == "text" {
			if text, ok := contentBlock["text"].(string); ok {
				if !guardrailscore.MayContainAliasToken(text) {
					return
				}
				if restored, changed := guardrailscore.RestoreText(text, state); changed {
					contentBlock["text"] = restored
				}
			}
		}
	}
}

func HandleAnthropicToolUseBuffer(c *gin.Context, eventType string, index int, block interface{}, eventMap map[string]interface{}) (handled bool, blockMessage string, passthrough []AnthropicBufferedEvent) {
	switch eventType {
	case anthropicEventTypeContentBlockStart:
		blockType, toolID := extractAnthropicBlockTypeAndID(block)
		if blockType != "tool_use" {
			return false, "", nil
		}
		state := getAnthropicToolUseBufferState(c)
		state.ToolIDByIndex[index] = toolID
		state.ByIndex[index] = append(state.ByIndex[index], AnthropicBufferedEvent{EventType: eventType, Payload: eventMap})
		return true, "", nil
	case anthropicEventTypeContentBlockDelta, anthropicEventTypeContentBlockStop:
		state := getAnthropicToolUseBufferState(c)
		if _, ok := state.ByIndex[index]; !ok {
			return false, "", nil
		}
		state.ByIndex[index] = append(state.ByIndex[index], AnthropicBufferedEvent{EventType: eventType, Payload: eventMap})
		if eventType != anthropicEventTypeContentBlockStop {
			return true, "", nil
		}

		toolID := state.ToolIDByIndex[index]
		blockState := getAnthropicGuardrailsBlockState(c)
		if message, ok := blockState.ToolMessages[toolID]; ok {
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true, message, nil
		}

		buffered := state.ByIndex[index]
		if rebuilt, ok := RebuildBufferedAnthropicToolUseEvents(c, buffered); ok {
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true, "", rebuilt
		}
		delete(state.ByIndex, index)
		delete(state.ToolIDByIndex, index)
		return true, "", buffered
	}
	return false, "", nil
}

func RebuildBufferedAnthropicToolUseEvents(c *gin.Context, events []AnthropicBufferedEvent) ([]AnthropicBufferedEvent, bool) {
	state := getAnthropicCredentialMaskState(c)
	if state == nil || len(state.AliasToReal) == 0 || len(events) == 0 {
		return nil, false
	}

	startBlock, _ := events[0].Payload["content_block"].(map[string]interface{})
	if blockType, _ := startBlock["type"].(string); blockType != "tool_use" {
		return nil, false
	}

	rawArgs := ""
	hasDeltaJSON := false
	hasAliasCandidate := false
	if input, ok := startBlock["input"]; ok && input != nil {
		if payload, err := json.Marshal(input); err == nil && guardrailscore.MayContainAliasToken(string(payload)) {
			hasAliasCandidate = true
		}
		if inputMap, ok := input.(map[string]interface{}); ok && len(inputMap) == 0 {
		} else if payload, err := json.Marshal(input); err == nil {
			rawArgs = string(payload)
		}
	}

	for _, buffered := range events {
		if buffered.EventType != anthropicEventTypeContentBlockDelta {
			continue
		}
		delta, _ := buffered.Payload["delta"].(map[string]interface{})
		if deltaType, _ := delta["type"].(string); deltaType == anthropicDeltaTypeInputJSONDelta {
			hasDeltaJSON = true
			if partial, ok := delta["partial_json"].(string); ok {
				if guardrailscore.MayContainAliasToken(partial) {
					hasAliasCandidate = true
				}
				rawArgs += partial
			}
		}
	}
	if !hasAliasCandidate {
		return nil, false
	}
	if !hasDeltaJSON {
		startPayload := cloneAnthropicEventPayload(events[0].Payload)
		stopPayload := cloneAnthropicEventPayload(events[len(events)-1].Payload)
		contentBlock, _ := startPayload["content_block"].(map[string]interface{})
		if input, ok := contentBlock["input"]; ok && input != nil {
			if restored, changed := guardrailscore.RestoreStructuredValue(input, state); changed {
				contentBlock["input"] = restored
				return []AnthropicBufferedEvent{
					{EventType: anthropicEventTypeContentBlockStart, Payload: startPayload},
					{EventType: anthropicEventTypeContentBlockStop, Payload: stopPayload},
				}, true
			}
		}
		return nil, false
	}
	if rawArgs == "" {
		return nil, false
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
		return nil, false
	}

	restoredValue, changed := guardrailscore.RestoreStructuredValue(parsed, state)
	if !changed {
		return nil, false
	}
	restoredJSON, err := json.Marshal(restoredValue)
	if err != nil {
		return nil, false
	}

	startPayload := cloneAnthropicEventPayload(events[0].Payload)
	stopPayload := cloneAnthropicEventPayload(events[len(events)-1].Payload)
	contentBlock, _ := startPayload["content_block"].(map[string]interface{})
	contentBlock["input"] = map[string]interface{}{}
	return []AnthropicBufferedEvent{
		{EventType: anthropicEventTypeContentBlockStart, Payload: startPayload},
		{
			EventType: anthropicEventTypeContentBlockDelta,
			Payload: map[string]interface{}{
				"type":  anthropicEventTypeContentBlockDelta,
				"index": startPayload["index"],
				"delta": map[string]interface{}{
					"type":         anthropicDeltaTypeInputJSONDelta,
					"partial_json": string(restoredJSON),
				},
			},
		},
		{EventType: anthropicEventTypeContentBlockStop, Payload: stopPayload},
	}, true
}

func getAnthropicCredentialMaskState(c *gin.Context) *guardrailscore.CredentialMaskState {
	if existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrailscore.CredentialMaskState); ok {
			return state
		}
	}
	return nil
}

func getAnthropicToolUseBufferState(c *gin.Context) *anthropicToolUseBufferState {
	if existing, ok := c.Get("guardrails_tool_buffer"); ok {
		if state, ok := existing.(*anthropicToolUseBufferState); ok {
			return state
		}
	}
	state := &anthropicToolUseBufferState{
		ByIndex:       make(map[int][]AnthropicBufferedEvent),
		ToolIDByIndex: make(map[int]string),
	}
	c.Set("guardrails_tool_buffer", state)
	return state
}

func getAnthropicGuardrailsBlockState(c *gin.Context) *anthropicGuardrailsBlockState {
	if existing, ok := c.Get("guardrails_block_state"); ok {
		if state, ok := existing.(*anthropicGuardrailsBlockState); ok {
			return state
		}
	}
	state := &anthropicGuardrailsBlockState{
		ToolMessages: make(map[string]string),
		BlockedIndex: make(map[int]string),
	}
	c.Set("guardrails_block_state", state)
	return state
}

func extractAnthropicBlockTypeAndID(block interface{}) (string, string) {
	if block == nil {
		return "", ""
	}
	raw, err := json.Marshal(block)
	if err != nil {
		return "", ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	blockType, _ := payload["type"].(string)
	if id, ok := payload["id"].(string); ok {
		return blockType, id
	}
	return blockType, ""
}

func cloneAnthropicEventPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return payload
	}
	return cloned
}
