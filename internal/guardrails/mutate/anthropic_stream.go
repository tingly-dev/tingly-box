package mutate

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const (
	anthropicEventTypeContentBlockStart = "content_block_start"
	anthropicEventTypeContentBlockDelta = "content_block_delta"
	anthropicEventTypeContentBlockStop  = "content_block_stop"
	anthropicDeltaTypeInputJSONDelta    = "input_json_delta"
)

type AnthropicBufferedEvent = protocol.GuardrailsBufferedEvent

type AnthropicToolUseDecisionKind string

const (
	AnthropicToolUseDecisionNone        AnthropicToolUseDecisionKind = "none"
	AnthropicToolUseDecisionBuffer      AnthropicToolUseDecisionKind = "buffer"
	AnthropicToolUseDecisionBlock       AnthropicToolUseDecisionKind = "block"
	AnthropicToolUseDecisionPassthrough AnthropicToolUseDecisionKind = "passthrough"
)

type AnthropicToolUseDecision struct {
	Kind         AnthropicToolUseDecisionKind
	BlockMessage string
	Passthrough  []AnthropicBufferedEvent
}

func RegisterAnthropicGuardrailsBlock(state *protocol.GuardrailsStreamState, toolID string, index int, message string) {
	if state == nil || toolID == "" || message == "" {
		return
	}
	state.PendingBlockMessages[toolID] = message
	state.PendingBlockedIndex[index] = toolID
}

// RewriteAnthropicToolUseEvent decides whether an Anthropic stream event should
// be buffered, replaced, or flushed, and returns the exact events the caller
// should emit. It only rewrites tool_use-related events.
func RewriteAnthropicToolUseEvent(
	credentialMask *guardrailscore.CredentialMaskState,
	streamState *protocol.GuardrailsStreamState,
	event interface{},
) (bool, []AnthropicBufferedEvent, error) {
	var (
		eventType string
		index     int
		block     interface{}
		rawJSON   string
	)

	switch evt := event.(type) {
	case *anthropic.MessageStreamEventUnion:
		if evt == nil {
			return false, nil, nil
		}
		eventType = evt.Type
		index = int(evt.Index)
		block = evt.ContentBlock
		rawJSON = evt.RawJSON()
	case *anthropic.BetaRawMessageStreamEventUnion:
		if evt == nil {
			return false, nil, nil
		}
		eventType = evt.Type
		index = int(evt.Index)
		block = evt.ContentBlock
		rawJSON = evt.RawJSON()
	default:
		return false, nil, nil
	}

	if !ShouldRewriteAnthropicEvent(streamState, eventType, block) {
		return false, nil, nil
	}

	var eventMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &eventMap); err != nil {
		return false, nil, err
	}
	if eventType != "" {
		eventMap["type"] = eventType
	}

	decision := HandleAnthropicToolUseBuffer(credentialMask, streamState, eventType, index, block, eventMap)
	switch decision.Kind {
	case AnthropicToolUseDecisionBuffer:
		return true, nil, nil
	case AnthropicToolUseDecisionBlock:
		if decision.BlockMessage == "" {
			return true, nil, nil
		}
		return true, []AnthropicBufferedEvent{
			{
				EventType: anthropicEventTypeContentBlockStart,
				Payload: map[string]interface{}{
					"type":  anthropicEventTypeContentBlockStart,
					"index": index,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				},
			},
			{
				EventType: anthropicEventTypeContentBlockDelta,
				Payload: map[string]interface{}{
					"type":  anthropicEventTypeContentBlockDelta,
					"index": index,
					"delta": map[string]interface{}{
						"type": "text_delta",
						"text": decision.BlockMessage,
					},
				},
			},
			{
				EventType: anthropicEventTypeContentBlockStop,
				Payload: map[string]interface{}{
					"type":  anthropicEventTypeContentBlockStop,
					"index": index,
				},
			},
		}, nil
	case AnthropicToolUseDecisionPassthrough:
		return true, decision.Passthrough, nil
	default:
		return false, nil, nil
	}
}

// ShouldRewriteAnthropicEvent is the fast-path gate for stream rewriting. It
// only turns on rewrite handling when a tool_use block starts, or when a
// previously buffered tool_use block is still in flight.
func ShouldRewriteAnthropicEvent(state *protocol.GuardrailsStreamState, eventType string, block interface{}) bool {
	switch eventType {
	case anthropicEventTypeContentBlockStart:
		blockType, _ := extractAnthropicBlockTypeAndID(block)
		if blockType == "tool_use" {
			return true
		}
	case anthropicEventTypeContentBlockDelta:
		if state != nil {
			hasBuffered := len(state.AnthropicToolEvents) > 0
			if hasBuffered {
				return true
			}
		}
	case anthropicEventTypeContentBlockStop:
		if state != nil {
			hasBuffered := len(state.AnthropicToolEvents) > 0
			if hasBuffered {
				return true
			}
		}
	}
	return false
}

// HandleAnthropicToolUseBuffer owns the protocol-level buffering for one
// Anthropic tool_use content block. By the time a block_stop arrives, it can
// choose between:
// 1. keep buffering
// 2. emit a synthetic block message
// 3. flush the original or rebuilt tool_use events
func HandleAnthropicToolUseBuffer(credentialMask *guardrailscore.CredentialMaskState, streamState *protocol.GuardrailsStreamState, eventType string, index int, block interface{}, eventMap map[string]interface{}) AnthropicToolUseDecision {
	if streamState == nil {
		return AnthropicToolUseDecision{}
	}

	switch eventType {
	case anthropicEventTypeContentBlockStart:
		blockType, toolID := extractAnthropicBlockTypeAndID(block)
		if blockType != "tool_use" {
			return AnthropicToolUseDecision{}
		}
		streamState.AnthropicToolIDs[index] = toolID
		streamState.AnthropicToolEvents[index] = append(streamState.AnthropicToolEvents[index], AnthropicBufferedEvent{EventType: eventType, Payload: eventMap})
		return AnthropicToolUseDecision{Kind: AnthropicToolUseDecisionBuffer}
	case anthropicEventTypeContentBlockDelta, anthropicEventTypeContentBlockStop:
		if _, ok := streamState.AnthropicToolEvents[index]; !ok {
			return AnthropicToolUseDecision{}
		}
		streamState.AnthropicToolEvents[index] = append(streamState.AnthropicToolEvents[index], AnthropicBufferedEvent{EventType: eventType, Payload: eventMap})
		if eventType != anthropicEventTypeContentBlockStop {
			return AnthropicToolUseDecision{Kind: AnthropicToolUseDecisionBuffer}
		}

		toolID := streamState.AnthropicToolIDs[index]
		if message, ok := streamState.PendingBlockMessages[toolID]; ok {
			delete(streamState.PendingBlockMessages, toolID)
			delete(streamState.PendingBlockedIndex, index)
			delete(streamState.AnthropicToolEvents, index)
			delete(streamState.AnthropicToolIDs, index)
			return AnthropicToolUseDecision{
				Kind:         AnthropicToolUseDecisionBlock,
				BlockMessage: message,
			}
		}
		if blockedToolID, ok := streamState.PendingBlockedIndex[index]; ok && blockedToolID != "" {
			if message, ok := streamState.PendingBlockMessages[blockedToolID]; ok {
				delete(streamState.PendingBlockMessages, blockedToolID)
				delete(streamState.PendingBlockedIndex, index)
				delete(streamState.AnthropicToolEvents, index)
				delete(streamState.AnthropicToolIDs, index)
				return AnthropicToolUseDecision{
					Kind:         AnthropicToolUseDecisionBlock,
					BlockMessage: message,
				}
			}
		}

		buffered := streamState.AnthropicToolEvents[index]
		if rebuilt, ok := RebuildBufferedAnthropicToolUseEvents(credentialMask, buffered); ok {
			delete(streamState.AnthropicToolEvents, index)
			delete(streamState.AnthropicToolIDs, index)
			return AnthropicToolUseDecision{
				Kind:        AnthropicToolUseDecisionPassthrough,
				Passthrough: rebuilt,
			}
		}
		delete(streamState.AnthropicToolEvents, index)
		delete(streamState.AnthropicToolIDs, index)
		return AnthropicToolUseDecision{
			Kind:        AnthropicToolUseDecisionPassthrough,
			Passthrough: buffered,
		}
	}
	return AnthropicToolUseDecision{}
}

func RebuildBufferedAnthropicToolUseEvents(state *guardrailscore.CredentialMaskState, events []AnthropicBufferedEvent) ([]AnthropicBufferedEvent, bool) {
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
