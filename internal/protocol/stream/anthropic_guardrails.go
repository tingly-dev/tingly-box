package stream

import (
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// rewriteAnthropicGuardrailsEvent keeps the main Anthropic passthrough loop
// focused on stream orchestration. Guardrails-specific event rewriting, tool_use
// buffering, and credential alias restoration all live here.
func rewriteAnthropicGuardrailsEvent(c *gin.Context, beta bool, eventType string, index int, block interface{}, evt interface{}) (bool, error) {
	if !shouldRewriteAnthropicEvent(c, eventType, block) {
		return false, nil
	}

	eventMap, err := toEventMap(evt, eventType)
	if err != nil {
		return false, err
	}
	restoreCredentialAliasesInEventMap(c, eventMap)

	if handleToolUseBuffer(c, beta, eventType, index, block, eventMap) {
		return true, nil
	}

	emitAnthropicGuardrailsEvent(c, beta, eventType, eventMap)
	return true, nil
}

func injectAnthropicGuardrailsBlock(c *gin.Context, beta bool) error {
	val, exists := c.Get("guardrails_block_message")
	if !exists {
		return nil
	}
	message, ok := val.(string)
	if !ok || message == "" {
		return nil
	}

	index := 0
	if raw, ok := c.Get("guardrails_block_index"); ok {
		switch v := raw.(type) {
		case int:
			index = v
		case int64:
			index = int(v)
		case float64:
			index = int(v)
		}
	}
	if !canFlushAnthropicGuardrails(c) {
		return errors.New("streaming not supported")
	}

	// injectAnthropicGuardrailsBlock is the higher-level error-path bridge. It rebuilds a
	// synthetic text block from guardrails data already stored on gin.Context and
	// writes it directly to the client when normal tool-use passthrough is no
	// longer driving the stream. In contrast, emitGuardrailsTextBlock is used from
	// the normal passthrough path where the caller already has the block index
	// and message in hand.
	start := map[string]interface{}{
		"type":  eventTypeContentBlockStart,
		"index": index,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	}
	delta := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": message,
		},
	}
	stop := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}

	if beta {
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockStart, start)
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockDelta, delta)
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockStop, stop)
		return nil
	}

	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockStart, start)
	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockDelta, delta)
	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockStop, stop)
	return nil
}

func toEventMap(evt interface{}, eventType string) (map[string]interface{}, error) {
	raw, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if eventType != "" {
		payload["type"] = eventType
	}
	return payload, nil
}

func shouldRewriteAnthropicEvent(c *gin.Context, eventType string, block interface{}) bool {
	switch eventType {
	case eventTypeContentBlockStart:
		blockType, _ := extractBlockTypeAndID(block)
		if blockType == "tool_use" || blockType == "text" {
			return true
		}
	case eventTypeContentBlockDelta:
		state := getToolUseBufferState(c)
		if len(state.ByIndex) > 0 {
			return true
		}
		if maskState := getCredentialMaskState(c); maskState != nil && len(maskState.AliasToReal) > 0 {
			return true
		}
	case eventTypeContentBlockStop:
		state := getToolUseBufferState(c)
		if len(state.ByIndex) > 0 {
			return true
		}
	}
	return false
}

type bufferedEvent struct {
	eventType string
	payload   map[string]interface{}
}

type toolUseBufferState struct {
	ByIndex       map[int][]bufferedEvent
	ToolIDByIndex map[int]string
}

func getToolUseBufferState(c *gin.Context) *toolUseBufferState {
	if existing, ok := c.Get("guardrails_tool_buffer"); ok {
		if state, ok := existing.(*toolUseBufferState); ok {
			return state
		}
	}
	state := &toolUseBufferState{
		ByIndex:       make(map[int][]bufferedEvent),
		ToolIDByIndex: make(map[int]string),
	}
	c.Set("guardrails_tool_buffer", state)
	return state
}

type guardrailsBlockState struct {
	ToolMessages map[string]string
	BlockedIndex map[int]string
}

func getGuardrailsBlockState(c *gin.Context) *guardrailsBlockState {
	if existing, ok := c.Get("guardrails_block_state"); ok {
		if state, ok := existing.(*guardrailsBlockState); ok {
			return state
		}
	}
	state := &guardrailsBlockState{
		ToolMessages: make(map[string]string),
		BlockedIndex: make(map[int]string),
	}
	c.Set("guardrails_block_state", state)
	return state
}

// RegisterGuardrailsBlock registers a tool_use block that should be intercepted.
func RegisterGuardrailsBlock(c *gin.Context, toolID string, index int, message string) {
	if toolID == "" || message == "" {
		return
	}
	state := getGuardrailsBlockState(c)
	state.ToolMessages[toolID] = message
	state.BlockedIndex[index] = toolID
}

func extractBlockTypeAndID(block interface{}) (string, string) {
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

func handleToolUseBuffer(c *gin.Context, beta bool, eventType string, index int, block interface{}, eventMap map[string]interface{}) bool {
	switch eventType {
	case eventTypeContentBlockStart:
		blockType, toolID := extractBlockTypeAndID(block)
		if blockType != "tool_use" {
			return false
		}
		state := getToolUseBufferState(c)
		state.ToolIDByIndex[index] = toolID
		state.ByIndex[index] = append(state.ByIndex[index], bufferedEvent{eventType: eventType, payload: eventMap})
		return true
	case eventTypeContentBlockDelta, eventTypeContentBlockStop:
		state := getToolUseBufferState(c)
		if _, ok := state.ByIndex[index]; !ok {
			return false
		}
		state.ByIndex[index] = append(state.ByIndex[index], bufferedEvent{eventType: eventType, payload: eventMap})
		if eventType != eventTypeContentBlockStop {
			return true
		}

		toolID := state.ToolIDByIndex[index]
		blockState := getGuardrailsBlockState(c)
		if message, ok := blockState.ToolMessages[toolID]; ok {
			if canFlushAnthropicGuardrails(c) {
				_ = emitGuardrailsTextBlock(c, beta, index, message)
			} else {
				logrus.Debug("Guardrails tool buffer: streaming not supported")
			}
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}

		if !canFlushAnthropicGuardrails(c) {
			logrus.Debug("Guardrails tool buffer: streaming not supported")
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}
		if rebuilt, ok := rebuildBufferedToolUseEvents(c, state.ByIndex[index]); ok {
			for _, buffered := range rebuilt {
				emitAnthropicGuardrailsEvent(c, beta, buffered.eventType, buffered.payload)
			}
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}
		for _, buffered := range state.ByIndex[index] {
			emitAnthropicGuardrailsEvent(c, beta, buffered.eventType, buffered.payload)
		}
		delete(state.ByIndex, index)
		delete(state.ToolIDByIndex, index)
		return true
	}
	return false
}

func getCredentialMaskState(c *gin.Context) *guardrailscore.CredentialMaskState {
	if existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrailscore.CredentialMaskState); ok {
			return state
		}
	}
	return nil
}

func restoreCredentialAliasesInEventMap(c *gin.Context, eventMap map[string]interface{}) {
	state := getCredentialMaskState(c)
	if state == nil || len(state.AliasToReal) == 0 || eventMap == nil {
		return
	}
	eventType, _ := eventMap["type"].(string)
	switch eventType {
	case eventTypeContentBlockDelta:
		delta, _ := eventMap["delta"].(map[string]interface{})
		deltaType, _ := delta["type"].(string)
		if deltaType == deltaTypeTextDelta {
			if text, ok := delta["text"].(string); ok {
				if !guardrailscore.MayContainAliasToken(text) {
					return
				}
				if restored, changed := guardrailscore.RestoreText(text, state); changed {
					delta["text"] = restored
				}
			}
		}
	case eventTypeContentBlockStart:
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

func rebuildBufferedToolUseEvents(c *gin.Context, events []bufferedEvent) ([]bufferedEvent, bool) {
	state := getCredentialMaskState(c)
	if state == nil || len(state.AliasToReal) == 0 || len(events) == 0 {
		return nil, false
	}

	startBlock, _ := events[0].payload["content_block"].(map[string]interface{})
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
		// Anthropic often starts tool_use input with an empty object and streams the
		// real JSON through input_json_delta chunks. Skip the empty "{}" seed here so
		// we do not rebuild an invalid payload like `{}{"command":"..."}`.
		if inputMap, ok := input.(map[string]interface{}); ok && len(inputMap) == 0 {
			// no-op
		} else if payload, err := json.Marshal(input); err == nil {
			rawArgs = string(payload)
		}
	}

	for _, buffered := range events {
		if buffered.eventType != eventTypeContentBlockDelta {
			continue
		}
		delta, _ := buffered.payload["delta"].(map[string]interface{})
		if deltaType, _ := delta["type"].(string); deltaType == deltaTypeInputJSONDelta {
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
		startPayload := cloneEventPayload(events[0].payload)
		stopPayload := cloneEventPayload(events[len(events)-1].payload)
		contentBlock, _ := startPayload["content_block"].(map[string]interface{})
		if input, ok := contentBlock["input"]; ok && input != nil {
			if restored, changed := guardrailscore.RestoreStructuredValue(input, state); changed {
				contentBlock["input"] = restored
				return []bufferedEvent{
					{eventType: eventTypeContentBlockStart, payload: startPayload},
					{eventType: eventTypeContentBlockStop, payload: stopPayload},
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

	startPayload := cloneEventPayload(events[0].payload)
	stopPayload := cloneEventPayload(events[len(events)-1].payload)
	contentBlock, _ := startPayload["content_block"].(map[string]interface{})

	// Rebuild the buffered tool_use in the same shape Anthropic streams it:
	// keep the empty input object on the start event, then emit one restored
	// input_json_delta chunk. Claude Code is stricter about this structure than
	// about how many delta chunks it receives.
	contentBlock["input"] = map[string]interface{}{}
	return []bufferedEvent{
		{eventType: eventTypeContentBlockStart, payload: startPayload},
		{
			eventType: eventTypeContentBlockDelta,
			payload: map[string]interface{}{
				"type":  eventTypeContentBlockDelta,
				"index": startPayload["index"],
				"delta": map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": string(restoredJSON),
				},
			},
		},
		{eventType: eventTypeContentBlockStop, payload: stopPayload},
	}, true
}

func cloneEventPayload(payload map[string]interface{}) map[string]interface{} {
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

func emitGuardrailsTextBlock(c *gin.Context, beta bool, index int, message string) error {
	if message == "" {
		return nil
	}

	// emitGuardrailsTextBlock is the low-level helper used while we are already
	// inside the Anthropic passthrough streaming path. At this point we have the
	// current block index and we only need to splice a synthetic
	// text block into the stream in place of the intercepted tool block.
	start := map[string]interface{}{
		"type":  eventTypeContentBlockStart,
		"index": index,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	}
	delta := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": message,
		},
	}
	stop := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}

	if beta {
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockStart, start)
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockDelta, delta)
		emitAnthropicGuardrailsEvent(c, true, eventTypeContentBlockStop, stop)
		return nil
	}

	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockStart, start)
	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockDelta, delta)
	emitAnthropicGuardrailsEvent(c, false, eventTypeContentBlockStop, stop)
	return nil
}

func canFlushAnthropicGuardrails(c *gin.Context) bool {
	_, ok := c.Writer.(interface{ Flush() })
	return ok
}

func emitAnthropicGuardrailsEvent(c *gin.Context, beta bool, eventType string, eventData map[string]interface{}) {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		logrus.Errorf("Failed to marshal Anthropic guardrails event: %v", err)
		return
	}

	c.SSEvent(eventType, string(eventJSON))
	c.Writer.Flush()

	if beta {
		if recorder, exists := c.Get("stream_event_recorder"); exists {
			if r, ok := recorder.(StreamEventRecorder); ok {
				r.RecordRawMapEvent(eventType, eventData)
			}
		}
	}
}
