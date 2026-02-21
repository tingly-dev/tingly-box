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
		var msg ResultMessage
		if err := unmarshalEvent(event, &msg); err == nil {
			msg.Type = event.Type
			msg.Timestamp = event.Timestamp
			a.messages = append(a.messages, &msg)
			newMessages = append(newMessages, &msg)
			hasResult = true
			resultSuccess = true
		}

	case MessageTypeSystem:
		if msg := a.parseSystemMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeAssistant:
		if msg := a.parseAssistantMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeUser:
		if msg := a.parseUserMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeToolUse:
		if msg := a.parseToolUseMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case MessageTypeToolResult:
		if msg := a.parseToolResultMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			a.completePendingToolUse(msg)
		}

	case MessageTypeResult:
		if msg := a.parseResultMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			hasResult = true
			resultSuccess = msg.IsSuccess()
		}

	case MessageTypeStreamEvent:
		if msg := a.parseStreamEventMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}
	}

	return newMessages, hasResult, resultSuccess
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

// unmarshalEvent unmarshals event raw JSON into a target struct
func unmarshalEvent(event events.Event, target interface{}) error {
	return json.Unmarshal([]byte(event.Raw), target)
}

// parseSystemMessage parses a system message from an event
func (a *MessageAccumulator) parseSystemMessage(event events.Event) *SystemMessage {
	var msg SystemMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	if msg.SessionID != "" {
		a.sessionID = msg.SessionID
	}
	return &msg
}

// parseAssistantMessage parses an assistant message from an event
func (a *MessageAccumulator) parseAssistantMessage(event events.Event) *AssistantMessage {
	// Unmarshal raw JSON into the struct
	var msg AssistantMessage
	if err := json.Unmarshal([]byte(event.Raw), &msg); err != nil {
		return nil
	}

	// Handle session_id
	if msg.SessionID != "" && a.sessionID == "" {
		a.sessionID = msg.SessionID
	}

	// Set timestamp from event if not in JSON
	if msg.Timestamp.IsZero() {
		msg.Timestamp = event.Timestamp
	}

	return &msg
}

// parseUserMessage parses a user message from an event
func (a *MessageAccumulator) parseUserMessage(event events.Event) *UserMessage {
	var msg UserMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	if msg.SessionID != "" && a.sessionID == "" {
		a.sessionID = msg.SessionID
	}
	return &msg
}

// parseToolUseMessage parses a tool_use message from an event
func (a *MessageAccumulator) parseToolUseMessage(event events.Event) *ToolUseMessage {
	var msg ToolUseMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseToolResultMessage parses a tool_result message from an event
func (a *MessageAccumulator) parseToolResultMessage(event events.Event) *ToolResultMessage {
	var msg ToolResultMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseResultMessage parses a result message from an event
func (a *MessageAccumulator) parseResultMessage(event events.Event) *ResultMessage {
	var msg ResultMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseStreamEventMessage parses a stream_event message from an event
func (a *MessageAccumulator) parseStreamEventMessage(event events.Event) *StreamEventMessage {
	var msg StreamEventMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}
