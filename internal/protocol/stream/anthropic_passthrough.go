package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ===================================================================
// Anthropic Handle Functions
// ===================================================================

// HandleAnthropicV1Stream handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1Stream(hc *protocol.HandleContext, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	flusher, ok := hc.GinContext.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported")
	}

	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if streamResp.Err() != nil {
				return false, streamResp.Err(), nil
			}
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)
			// Only set model for message_start events, as other events don't have a message field
			if evt.Type == "message_start" {
				evt.Message.Model = anthropic.Model(hc.ResponseModel)
			}

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}
			if evt.Usage.CacheReadInputTokens > 0 {
				cacheTokens = int(evt.Usage.CacheReadInputTokens)
				hasUsage = true
			}

			eventMap, err := toEventMap(evt, evt.Type)
			if err != nil {
				return err
			}
			restoreCredentialAliasesInEventMap(hc.GinContext, eventMap)

			if handleToolUseBuffer(hc.GinContext, false, evt.Type, int(evt.Index), evt.ContentBlock, eventMap) {
				return nil
			}

			sendAnthropicStreamEvent(hc.GinContext, evt.Type, eventMap, flusher)
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 stream canceled by client")
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	if err := injectGuardrailsBlock(hc.GinContext, false); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// HandleAnthropicV1BetaStream handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaStream(hc *protocol.HandleContext, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	flusher, ok := hc.GinContext.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroTokenUsage(), errors.New("streaming not supported")
	}

	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if streamResp.Err() != nil {
				return false, streamResp.Err(), nil
			}
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.BetaRawMessageStreamEventUnion)
			// Only set model for message_start events, as other events don't have a message field
			if evt.Type == "message_start" {
				evt.Message.Model = anthropic.Model(hc.ResponseModel)
			}

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}
			if evt.Usage.CacheReadInputTokens > 0 {
				cacheTokens = int(evt.Usage.CacheReadInputTokens)
				hasUsage = true
			}

			// Send SSE event
			eventMap, err := toEventMap(evt, evt.Type)
			if err != nil {
				return err
			}
			restoreCredentialAliasesInEventMap(hc.GinContext, eventMap)

			if handleToolUseBuffer(hc.GinContext, true, evt.Type, int(evt.Index), evt.ContentBlock, eventMap) {
				return nil
			}

			sendAnthropicBetaStreamEvent(hc.GinContext, evt.Type, eventMap, flusher)
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 beta stream canceled by client")
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	if err := injectGuardrailsBlock(hc.GinContext, true); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
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
			flusher, ok := c.Writer.(http.Flusher)
			if ok {
				_ = emitGuardrailsTextBlock(c, beta, index, message, flusher)
			} else {
				logrus.Debug("Guardrails tool buffer: streaming not supported")
			}
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			logrus.Debug("Guardrails tool buffer: streaming not supported")
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}
		if rebuilt, ok := rebuildBufferedToolUseEvents(c, state.ByIndex[index]); ok {
			for _, buffered := range rebuilt {
				if beta {
					sendAnthropicBetaStreamEvent(c, buffered.eventType, buffered.payload, flusher)
				} else {
					sendAnthropicStreamEvent(c, buffered.eventType, buffered.payload, flusher)
				}
			}
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}
		for _, buffered := range state.ByIndex[index] {
			if beta {
				sendAnthropicBetaStreamEvent(c, buffered.eventType, buffered.payload, flusher)
			} else {
				sendAnthropicStreamEvent(c, buffered.eventType, buffered.payload, flusher)
			}
		}
		delete(state.ByIndex, index)
		delete(state.ToolIDByIndex, index)
		return true
	}
	return false
}

func getCredentialMaskState(c *gin.Context) *guardrails.CredentialMaskState {
	if existing, ok := c.Get(guardrails.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrails.CredentialMaskState); ok {
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
				if !guardrails.MayContainAliasToken(text) {
					return
				}
				if restored, changed := guardrails.RestoreText(text, state); changed {
					delta["text"] = restored
				}
			}
		}
	case eventTypeContentBlockStart:
		contentBlock, _ := eventMap["content_block"].(map[string]interface{})
		if blockType, _ := contentBlock["type"].(string); blockType == "text" {
			if text, ok := contentBlock["text"].(string); ok {
				if !guardrails.MayContainAliasToken(text) {
					return
				}
				if restored, changed := guardrails.RestoreText(text, state); changed {
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
		if payload, err := json.Marshal(input); err == nil && guardrails.MayContainAliasToken(string(payload)) {
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
				if guardrails.MayContainAliasToken(partial) {
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
			if restored, changed := guardrails.RestoreStructuredValue(input, state); changed {
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

	restoredValue, changed := guardrails.RestoreStructuredValue(parsed, state)
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

func emitGuardrailsTextBlock(c *gin.Context, beta bool, index int, message string, flusher http.Flusher) error {
	if message == "" {
		return nil
	}

	// emitGuardrailsTextBlock is the low-level helper used while we are already
	// inside the Anthropic passthrough streaming path. At this point we have the
	// current block index, a live flusher, and we only need to splice a synthetic
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
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStart, start, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
		return nil
	}

	sendAnthropicStreamEvent(c, eventTypeContentBlockStart, start, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
	return nil
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================
