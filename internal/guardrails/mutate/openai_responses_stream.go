package mutate

import (
	"encoding/json"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const (
	openAIResponsesEventOutputItemAdded        = "response.output_item.added"
	openAIResponsesEventFunctionArgsDelta      = "response.function_call_arguments.delta"
	openAIResponsesEventFunctionArgsDone       = "response.function_call_arguments.done"
	openAIResponsesEventOutputItemDone         = "response.output_item.done"
	openAIResponsesEventOutputTextDelta        = "response.output_text.delta"
	openAIResponsesEventOutputTextDone         = "response.output_text.done"
	openAIResponsesEventCompleted              = "response.completed"
	openAIResponsesItemTypeFunctionCall        = "function_call"
	openAIResponsesItemTypeMessage             = "message"
	openAIResponsesContentTypeOutputText       = "output_text"
	openAIResponsesStatusInProgress            = "in_progress"
	openAIResponsesStatusCompleted             = "completed"
	openAIResponsesGuardrailsReplacementSuffix = "_guardrails"
)

type OpenAIResponsesBufferedEvent = protocol.GuardrailsBufferedEvent

// RewriteOpenAIResponsesFunctionCallEvent decides whether an OpenAI Responses
// stream event should be buffered, replaced, or flushed. It only rewrites
// function_call-related events and response.completed after a previous block.
func RewriteOpenAIResponsesFunctionCallEvent(
	credentialMask *guardrailscore.CredentialMaskState,
	streamState *protocol.GuardrailsStreamState,
	event map[string]interface{},
) (bool, []OpenAIResponsesBufferedEvent, error) {
	if streamState == nil || event == nil {
		return false, nil, nil
	}
	ensureOpenAIResponsesStreamState(streamState)

	eventType, _ := event["type"].(string)
	switch eventType {
	case openAIResponsesEventOutputItemAdded:
		item, _ := event["item"].(map[string]interface{})
		if itemType, _ := item["type"].(string); itemType != openAIResponsesItemTypeFunctionCall {
			return false, nil, nil
		}
		itemID := stringFromOpenAIEventMap(item, "id")
		if itemID == "" {
			return false, nil, nil
		}
		outputIndex := intFromOpenAIEventMap(event, "output_index")
		streamState.OpenAIResponsesOutputIDs[outputIndex] = itemID
		streamState.OpenAIResponsesToolEvents[itemID] = append(
			streamState.OpenAIResponsesToolEvents[itemID],
			OpenAIResponsesBufferedEvent{EventType: eventType, Payload: cloneOpenAIEventMap(event)},
		)
		return true, nil, nil
	case openAIResponsesEventFunctionArgsDelta, openAIResponsesEventFunctionArgsDone:
		itemID := stringFromOpenAIEventMap(event, "item_id")
		if itemID == "" {
			itemID = streamState.OpenAIResponsesOutputIDs[intFromOpenAIEventMap(event, "output_index")]
		}
		if itemID == "" {
			return false, nil, nil
		}
		if _, ok := streamState.OpenAIResponsesToolEvents[itemID]; !ok {
			return false, nil, nil
		}
		streamState.OpenAIResponsesToolEvents[itemID] = append(
			streamState.OpenAIResponsesToolEvents[itemID],
			OpenAIResponsesBufferedEvent{EventType: eventType, Payload: cloneOpenAIEventMap(event)},
		)
		return true, nil, nil
	case openAIResponsesEventOutputItemDone:
		item, _ := event["item"].(map[string]interface{})
		itemID := stringFromOpenAIEventMap(item, "id")
		if itemID == "" {
			itemID = streamState.OpenAIResponsesOutputIDs[intFromOpenAIEventMap(event, "output_index")]
		}
		if itemID == "" {
			return false, nil, nil
		}
		buffered, ok := streamState.OpenAIResponsesToolEvents[itemID]
		if !ok {
			return false, nil, nil
		}
		buffered = append(buffered, OpenAIResponsesBufferedEvent{EventType: eventType, Payload: cloneOpenAIEventMap(event)})
		delete(streamState.OpenAIResponsesToolEvents, itemID)

		outputIndex := intFromOpenAIEventMap(event, "output_index")
		if blockMessage := consumeOpenAIResponsesBlockMessage(streamState, itemID, outputIndex); blockMessage != "" {
			blocked := protocol.GuardrailsOpenAIResponsesBlockedItem{
				ItemID:      itemID,
				TextItemID:  itemID + openAIResponsesGuardrailsReplacementSuffix,
				OutputIndex: outputIndex,
				Message:     blockMessage,
			}
			streamState.OpenAIResponsesBlocked = append(streamState.OpenAIResponsesBlocked, blocked)
			return true, syntheticOpenAIResponsesBlockEvents(event, blocked), nil
		}

		rebuilt, ok := RebuildBufferedOpenAIResponsesFunctionCallEvents(credentialMask, buffered)
		if ok {
			return true, rebuilt, nil
		}
		return true, buffered, nil
	case openAIResponsesEventCompleted:
		if len(streamState.OpenAIResponsesBlocked) == 0 {
			return false, nil, nil
		}
		return true, []OpenAIResponsesBufferedEvent{
			{
				EventType: eventType,
				Payload:   rewriteOpenAIResponsesCompletedEvent(event, streamState.OpenAIResponsesBlocked),
			},
		}, nil
	default:
		return false, nil, nil
	}
}

func RebuildBufferedOpenAIResponsesFunctionCallEvents(state *guardrailscore.CredentialMaskState, events []OpenAIResponsesBufferedEvent) ([]OpenAIResponsesBufferedEvent, bool) {
	if state == nil || len(state.AliasToReal) == 0 || len(events) == 0 {
		return nil, false
	}

	changed := false
	rebuilt := make([]OpenAIResponsesBufferedEvent, 0, len(events))
	for _, event := range events {
		payload := cloneOpenAIEventMap(event.Payload)
		switch event.EventType {
		case openAIResponsesEventFunctionArgsDelta:
			if delta, ok := payload["delta"].(string); ok && guardrailscore.MayContainAliasToken(delta) {
				if restored, ok := guardrailscore.RestoreText(delta, state); ok {
					payload["delta"] = restored
					changed = true
				}
			}
		case openAIResponsesEventFunctionArgsDone:
			if args, ok := payload["arguments"].(string); ok && guardrailscore.MayContainAliasToken(args) {
				if restored, ok := guardrailscore.RestoreText(args, state); ok {
					payload["arguments"] = restored
					changed = true
				}
			}
		case openAIResponsesEventOutputItemAdded, openAIResponsesEventOutputItemDone:
			item, _ := payload["item"].(map[string]interface{})
			if item != nil {
				if args, ok := item["arguments"].(string); ok && guardrailscore.MayContainAliasToken(args) {
					if restored, ok := guardrailscore.RestoreText(args, state); ok {
						item["arguments"] = restored
						changed = true
					}
				}
			}
		}
		rebuilt = append(rebuilt, OpenAIResponsesBufferedEvent{EventType: event.EventType, Payload: payload})
	}
	if !changed {
		return nil, false
	}
	return rebuilt, true
}

func ensureOpenAIResponsesStreamState(state *protocol.GuardrailsStreamState) {
	if state.OpenAIResponsesToolEvents == nil {
		state.OpenAIResponsesToolEvents = make(map[string][]protocol.GuardrailsBufferedEvent)
	}
	if state.OpenAIResponsesOutputIDs == nil {
		state.OpenAIResponsesOutputIDs = make(map[int]string)
	}
}

func consumeOpenAIResponsesBlockMessage(state *protocol.GuardrailsStreamState, itemID string, outputIndex int) string {
	if state == nil {
		return ""
	}
	if message, ok := state.PendingBlockMessages[itemID]; ok {
		delete(state.PendingBlockMessages, itemID)
		delete(state.PendingBlockedIndex, outputIndex)
		return message
	}
	if blockedID, ok := state.PendingBlockedIndex[outputIndex]; ok {
		if message, ok := state.PendingBlockMessages[blockedID]; ok {
			delete(state.PendingBlockMessages, blockedID)
			delete(state.PendingBlockedIndex, outputIndex)
			return message
		}
	}
	return ""
}

func syntheticOpenAIResponsesBlockEvents(source map[string]interface{}, blocked protocol.GuardrailsOpenAIResponsesBlockedItem) []OpenAIResponsesBufferedEvent {
	seq := source["sequence_number"]
	return []OpenAIResponsesBufferedEvent{
		{
			EventType: openAIResponsesEventOutputItemAdded,
			Payload: map[string]interface{}{
				"type":            openAIResponsesEventOutputItemAdded,
				"sequence_number": seq,
				"output_index":    blocked.OutputIndex,
				"item":            openAIResponsesBlockMessageItem(blocked, openAIResponsesStatusInProgress, ""),
			},
		},
		{
			EventType: openAIResponsesEventOutputTextDelta,
			Payload: map[string]interface{}{
				"type":            openAIResponsesEventOutputTextDelta,
				"sequence_number": seq,
				"item_id":         blocked.TextItemID,
				"output_index":    blocked.OutputIndex,
				"content_index":   0,
				"delta":           blocked.Message,
				"logprobs":        []interface{}{},
			},
		},
		{
			EventType: openAIResponsesEventOutputTextDone,
			Payload: map[string]interface{}{
				"type":            openAIResponsesEventOutputTextDone,
				"sequence_number": seq,
				"item_id":         blocked.TextItemID,
				"output_index":    blocked.OutputIndex,
				"content_index":   0,
				"text":            blocked.Message,
				"logprobs":        []interface{}{},
			},
		},
		{
			EventType: openAIResponsesEventOutputItemDone,
			Payload: map[string]interface{}{
				"type":            openAIResponsesEventOutputItemDone,
				"sequence_number": seq,
				"output_index":    blocked.OutputIndex,
				"item":            openAIResponsesBlockMessageItem(blocked, openAIResponsesStatusCompleted, blocked.Message),
			},
		},
	}
}

func rewriteOpenAIResponsesCompletedEvent(event map[string]interface{}, blockedItems []protocol.GuardrailsOpenAIResponsesBlockedItem) map[string]interface{} {
	next := cloneOpenAIEventMap(event)
	response, _ := next["response"].(map[string]interface{})
	if response == nil {
		return next
	}
	blockedIDs := make(map[string]struct{}, len(blockedItems))
	for _, blocked := range blockedItems {
		blockedIDs[blocked.ItemID] = struct{}{}
	}
	output, _ := response["output"].([]interface{})
	rewritten := make([]interface{}, 0, len(output)+len(blockedItems))
	for _, item := range output {
		itemMap, _ := item.(map[string]interface{})
		if itemMap != nil {
			if itemType, _ := itemMap["type"].(string); itemType == openAIResponsesItemTypeFunctionCall {
				if _, ok := blockedIDs[stringFromOpenAIEventMap(itemMap, "id")]; ok {
					continue
				}
			}
		}
		rewritten = append(rewritten, item)
	}
	for _, blocked := range blockedItems {
		rewritten = append(rewritten, openAIResponsesBlockMessageItem(blocked, openAIResponsesStatusCompleted, blocked.Message))
	}
	response["output"] = rewritten
	return next
}

func openAIResponsesBlockMessageItem(blocked protocol.GuardrailsOpenAIResponsesBlockedItem, status string, text string) map[string]interface{} {
	return map[string]interface{}{
		"id":     blocked.TextItemID,
		"type":   openAIResponsesItemTypeMessage,
		"role":   "assistant",
		"status": status,
		"content": []map[string]interface{}{
			{
				"type":        openAIResponsesContentTypeOutputText,
				"text":        text,
				"annotations": []interface{}{},
			},
		},
	}
}

func cloneOpenAIEventMap(event map[string]interface{}) map[string]interface{} {
	if event == nil {
		return nil
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return event
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return event
	}
	return out
}

func stringFromOpenAIEventMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	value, _ := m[key].(string)
	return value
}

func intFromOpenAIEventMap(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	switch value := m[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
