package claude

import (
	"encoding/json"
	"sync"

	"github.com/tingly-dev/tingly-box/agentboot/events"
)

// MessageAccumulator collects related events into complete messages
type MessageAccumulator struct {
	mu              sync.RWMutex
	messages        []Message
	pendingToolUses map[string]*PendingToolUse
	sessionID       string
}

// PendingToolUse tracks a tool_use waiting for its result
type PendingToolUse struct {
	ToolUseID string
	ToolUse   *ToolUseBlock
	Result    *ToolResultMessage
	Complete  bool
}

// NewMessageAccumulator creates a new message accumulator
func NewMessageAccumulator() *MessageAccumulator {
	return &MessageAccumulator{
		messages:        make([]Message, 0),
		pendingToolUses: make(map[string]*PendingToolUse),
	}
}

// AddEvent adds a parsed event and returns any newly complete messages
// Returns: (newMessages, hasResult, resultSuccess)
func (a *MessageAccumulator) AddEvent(event events.Event) ([]Message, bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var newMessages []Message
	var hasResult bool
	var resultSuccess bool

	switch event.Type {
	case MessageTypeText:
		msg := &ResultMessage{
			Type:      event.Type,
			Result:    getString(event.Data, "text"),
			Timestamp: event.Timestamp,
		}
		a.messages = append(a.messages, msg)
		newMessages = append(newMessages, msg)
		hasResult = true
		resultSuccess = true

	case MessageTypeSystem:
		msg := a.parseSystemMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeAssistant:
		msg := a.parseAssistantMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)

			// Extract tool uses to track for results
			a.trackToolUses(msg)
		}

	case MessageTypeUser:
		msg := a.parseUserMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeToolUse:
		msg := a.parseToolUseMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeToolResult:
		msg := a.parseToolResultMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)

			// Mark pending tool use as complete
			a.completePendingToolUse(msg)
		}

	case MessageTypeResult:
		msg := a.parseResultMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			hasResult = true
			resultSuccess = msg.IsSuccess()
		}

	case MessageTypeStreamEvent:
		msg := a.parseStreamEventMessage(event)
		if msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}
	}

	return newMessages, hasResult, resultSuccess
}

// trackToolUses extracts tool_use blocks from assistant message for tracking
func (a *MessageAccumulator) trackToolUses(msg *AssistantMessage) {
	for _, block := range msg.Message.Content {
		if toolUse, ok := block.(*ToolUseBlock); ok {
			a.pendingToolUses[toolUse.ID] = &PendingToolUse{
				ToolUseID: toolUse.ID,
				ToolUse:   toolUse,
				Complete:  false,
			}
		}
	}
}

// completePendingToolUse marks a tool use as complete when its result arrives
func (a *MessageAccumulator) completePendingToolUse(result *ToolResultMessage) {
	if pending, ok := a.pendingToolUses[result.ToolUseID]; ok {
		pending.Result = result
		pending.Complete = true
	}
}

// GetMessages returns all accumulated messages
func (a *MessageAccumulator) GetMessages() []Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]Message, len(a.messages))
	copy(result, a.messages)
	return result
}

// GetMessagesByType returns messages of a specific type
func (a *MessageAccumulator) GetMessagesByType(msgType string) []Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []Message
	for _, msg := range a.messages {
		if msg.GetType() == msgType {
			result = append(result, msg)
		}
	}
	return result
}

// GetAssistantMessages returns all assistant messages
func (a *MessageAccumulator) GetAssistantMessages() []*AssistantMessage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []*AssistantMessage
	for _, msg := range a.messages {
		if am, ok := msg.(*AssistantMessage); ok {
			result = append(result, am)
		}
	}
	return result
}

// GetToolUses returns all tool uses with their results
func (a *MessageAccumulator) GetToolUses() map[string]*PendingToolUse {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return a copy
	result := make(map[string]*PendingToolUse)
	for k, v := range a.pendingToolUses {
		result[k] = v
	}
	return result
}

// GetSessionID returns the session ID if available
func (a *MessageAccumulator) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// Reset clears the accumulator state
func (a *MessageAccumulator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.messages = make([]Message, 0)
	a.pendingToolUses = make(map[string]*PendingToolUse)
	a.sessionID = ""
}

// parseSystemMessage parses a system message from an event
func (a *MessageAccumulator) parseSystemMessage(event events.Event) *SystemMessage {
	data := event.Data

	sessionID, _ := data["session_id"].(string)
	if sessionID != "" {
		a.sessionID = sessionID
	}

	return &SystemMessage{
		Type:      event.Type,
		SubType:   getString(data, "subtype"),
		SessionID: sessionID,
		Timestamp: event.Timestamp,
	}
}

// parseAssistantMessage parses an assistant message from an event
func (a *MessageAccumulator) parseAssistantMessage(event events.Event) *AssistantMessage {
	data := event.Data

	msgData, ok := data["message"].(map[string]interface{})
	if !ok {
		return nil
	}

	message := MessageData{
		Model:      getString(msgData, "model"),
		ID:         getString(msgData, "id"),
		Type:       getString(msgData, "type"),
		Role:       getString(msgData, "role"),
		StopReason: getString(msgData, "stop_reason"),
	}

	// Parse content blocks
	if contentArr, ok := msgData["content"].([]interface{}); ok {
		for _, item := range contentArr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				blockType := getString(itemMap, "type")
				var block ContentBlock

				switch blockType {
				case "text":
					block = &TextBlock{
						Type: blockType,
						Text: getString(itemMap, "text"),
					}
				case "tool_use":
					block = &ToolUseBlock{
						Type:  blockType,
						ID:    getString(itemMap, "id"),
						Name:  getString(itemMap, "name"),
						Input: getMap(itemMap, "input"),
					}
				case "thinking":
					block = &ThinkingBlock{
						Type:     blockType,
						Thinking: getString(itemMap, "thinking"),
					}
				case "tool_result":
					block = &ToolResultContentBlock{
						Type:      blockType,
						ToolUseID: getString(itemMap, "tool_use_id"),
						Content:   getString(itemMap, "content"),
						IsError:   getBool(itemMap, "is_error"),
					}
				default:
					// Keep as raw map for unknown types
					block = &UnknownBlock{Data: itemMap}
				}

				message.Content = append(message.Content, block)
			}
		}
	}

	// Parse usage if present
	if usageMap, ok := msgData["usage"].(map[string]interface{}); ok {
		message.Usage = UsageInfo{
			InputTokens:              getInt(usageMap, "input_tokens"),
			CacheCreationInputTokens: getInt(usageMap, "cache_creation_input_tokens"),
			CacheReadInputTokens:     getInt(usageMap, "cache_read_input_tokens"),
			OutputTokens:             getInt(usageMap, "output_tokens"),
		}
	}

	sessionID, _ := data["session_id"].(string)
	if sessionID != "" && a.sessionID == "" {
		a.sessionID = sessionID
	}

	return &AssistantMessage{
		Type:            event.Type,
		Message:         message,
		ParentToolUseID: getStringPtr(data, "parent_tool_use_id"),
		SessionID:       sessionID,
		UUID:            getString(data, "uuid"),
		Timestamp:       event.Timestamp,
	}
}

// parseUserMessage parses a user message from an event
func (a *MessageAccumulator) parseUserMessage(event events.Event) *UserMessage {
	data := event.Data

	message, _ := data["message"].(string)

	return &UserMessage{
		Type:            event.Type,
		Message:         message,
		ParentToolUseID: getStringPtr(data, "parent_tool_use_id"),
		SessionID:       getString(data, "session_id"),
		Timestamp:       event.Timestamp,
	}
}

// parseToolUseMessage parses a tool_use message from an event
func (a *MessageAccumulator) parseToolUseMessage(event events.Event) *ToolUseMessage {
	data := event.Data

	return &ToolUseMessage{
		Type:      event.Type,
		Name:      getString(data, "name"),
		Input:     getMap(data, "input"),
		ToolUseID: getString(data, "tool_use_id"),
		SessionID: getString(data, "session_id"),
		Timestamp: event.Timestamp,
	}
}

// parseToolResultMessage parses a tool_result message from an event
func (a *MessageAccumulator) parseToolResultMessage(event events.Event) *ToolResultMessage {
	data := event.Data

	msg := &ToolResultMessage{
		Type:      event.Type,
		ToolUseID: getString(data, "tool_use_id"),
		IsError:   getBool(data, "is_error"),
		SessionID: getString(data, "session_id"),
		Timestamp: event.Timestamp,
	}

	// Output can be either a string or content blocks
	if output, ok := data["output"].(string); ok {
		msg.Output = output
	}

	// Check for structured content
	if contentArr, ok := data["content"].([]interface{}); ok {
		// Parse content blocks
		contentBytes, _ := json.Marshal(contentArr)
		var blocks []json.RawMessage
		_ = json.Unmarshal(contentBytes, &blocks)

		for _, blockBytes := range blocks {
			if block, err := UnmarshalContentBlock(blockBytes); err == nil {
				msg.Content = append(msg.Content, block)
			}
		}
	}

	return msg
}

// parseResultMessage parses a result message from an event
func (a *MessageAccumulator) parseResultMessage(event events.Event) *ResultMessage {
	data := event.Data

	msg := &ResultMessage{
		Type:          event.Type,
		SubType:       getString(data, "subtype"),
		Result:        getString(data, "result"),
		TotalCostUSD:  getFloat(data, "total_cost_usd"),
		IsError:       getBool(data, "is_error"),
		DurationMS:    getInt64(data, "duration_ms"),
		DurationAPIMS: getInt64(data, "duration_api_ms"),
		NumTurns:      getInt(data, "num_turns"),
		SessionID:     getString(data, "session_id"),
		Timestamp:     event.Timestamp,
	}

	// Parse usage if present
	if usageMap, ok := data["usage"].(map[string]interface{}); ok {
		msg.Usage = UsageInfo{
			InputTokens:              getInt(usageMap, "input_tokens"),
			CacheCreationInputTokens: getInt(usageMap, "cache_creation_input_tokens"),
			CacheReadInputTokens:     getInt(usageMap, "cache_read_input_tokens"),
			OutputTokens:             getInt(usageMap, "output_tokens"),
		}
	}

	// Parse permission denials if present
	if denialsArr, ok := data["permission_denials"].([]interface{}); ok {
		for _, d := range denialsArr {
			if denialMap, ok := d.(map[string]interface{}); ok {
				msg.PermissionDenials = append(msg.PermissionDenials, PermissionDenial{
					RequestID: getString(denialMap, "request_id"),
					Reason:    getString(denialMap, "reason"),
				})
			}
		}
	}

	return msg
}

// parseStreamEventMessage parses a stream_event message from an event
func (a *MessageAccumulator) parseStreamEventMessage(event events.Event) *StreamEventMessage {
	data := event.Data

	msg := &StreamEventMessage{
		Type:      event.Type,
		SessionID: getString(data, "session_id"),
		Timestamp: event.Timestamp,
	}

	// Parse event data
	if eventMap, ok := data["event"].(map[string]interface{}); ok {
		event := StreamEvent{
			Type:  getString(eventMap, "type"),
			Index: getInt(eventMap, "index"),
		}

		// Parse delta based on type
		if deltaMap, ok := eventMap["delta"].(map[string]interface{}); ok {
			deltaType := getString(deltaMap, "type")

			switch deltaType {
			case "text_delta":
				event.Delta = &TextDelta{
					Type: deltaType,
					Text: getString(deltaMap, "text"),
				}
			case "input_json_delta":
				event.Delta = &InputJSONDelta{
					Type:        deltaType,
					PartialJSON: getString(deltaMap, "partial_json"),
				}
			default:
				// Keep as raw map for unknown delta types
				event.Delta = deltaMap
			}
		}

		msg.Event = event
	}

	return msg
}
