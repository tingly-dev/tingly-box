package claude

import (
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/protocol"
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
func (a *MessageAccumulator) AddEvent(event protocol.Event) ([]Message, bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var newMessages []Message
	var hasResult bool
	var resultSuccess bool

	switch event.Type {
	case SDKTextMessage:
		var msg ResultMessage
		if err := unmarshalEvent(event, &msg); err == nil {
			msg.Type = event.Type
			msg.Timestamp = event.Timestamp
			a.messages = append(a.messages, &msg)
			newMessages = append(newMessages, &msg)
			hasResult = true
			resultSuccess = true
		}

	case SDKSystemMessage:
		if msg := a.parseSystemMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case SDKAssistantMessage:
		if msg := a.parseAssistantMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case SDKUserMessage:
		for _, msg := range a.parseUserMessages(event) {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			if tr, ok := msg.(*ToolResultMessage); ok {
				a.completePendingToolUse(tr)
			}
		}

	case SDKToolUseMessage:
		if msg := a.parseToolUseMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case SDKToolResultMessage:
		if msg := a.parseToolResultMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			a.completePendingToolUse(msg)
		}

	case SDKResultMessage:
		if msg := a.parseResultMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
			hasResult = true
			resultSuccess = msg.IsSuccess()
		}

	case SDKStreamEventMessage:
		if msg := a.parseStreamEventMessage(event); msg != nil {
			a.messages = append(a.messages, msg)
			newMessages = append(newMessages, msg)
		}

	case SDKRateLimitEvent,
		SDKHookStartedMessage, SDKHookProgressMessage, SDKHookResponseMessage,
		SDKPostTurnSummaryMessage,
		SDKPartialAssistantMessage, SDKCompactBoundaryMessage,
		SDKStatusMessage, SDKLocalCommandOutputMessage,
		SDKToolProgressMessage, SDKAuthStatusMessage,
		SDKFilesPersistedMessage, SDKToolUseSummaryMessage,
		SDKPromptSuggestionMessage, SystemSubtypeTaskProgress,
		SDKUserMessageReplayMessage:
		// Infrastructure / housekeeping events emitted by the CLI that carry no
		// user-facing content. Consume them here so they don't fall through to
		// the default and produce spurious log noise. Their raw JSON is still
		// logged at the runner level for diagnostics.
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
	maps.Copy(result, a.pendingToolUses)
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
func unmarshalEvent(event protocol.Event, target interface{}) error {
	return json.Unmarshal([]byte(event.Raw), target)
}

// parseSystemMessage parses a system message from an event
func (a *MessageAccumulator) parseSystemMessage(event protocol.Event) *SystemMessage {
	var msg SystemMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	// Preserve the full payload so forward-compatible fields (notably the
	// api_retry/rate_limit metadata, whose names vary by CLI version) survive
	// for formatting and logging instead of being stripped to the typed fields.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(event.Raw), &raw); err == nil {
		msg.Raw = raw
	}
	if msg.SessionID != "" {
		a.sessionID = msg.SessionID
	}
	return &msg
}

// parseAssistantMessage parses an assistant message from an event
func (a *MessageAccumulator) parseAssistantMessage(event protocol.Event) *AssistantMessage {
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

// parseUserMessages parses a user event. The CLI emits two shapes: the
// plain {"message": "<text>"} and the SDK {"message": {"role": "user",
// "content": ...}} whose content blocks carry the tool_result answers to
// earlier tool_use calls. Tool results are returned as ToolResultMessages so
// consumers see them exactly as they would a standalone tool_result event.
func (a *MessageAccumulator) parseUserMessages(event protocol.Event) []Message {
	var raw struct {
		Type            string          `json:"type"`
		Message         json.RawMessage `json:"message"`
		ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"`
		SessionID       string          `json:"session_id,omitempty"`
		Timestamp       time.Time       `json:"timestamp,omitempty"`
	}
	if err := unmarshalEvent(event, &raw); err != nil {
		return nil
	}
	if raw.SessionID != "" && a.sessionID == "" {
		a.sessionID = raw.SessionID
	}
	base := &UserMessage{Type: raw.Type, ParentToolUseID: raw.ParentToolUseID, SessionID: raw.SessionID, Timestamp: raw.Timestamp}

	var text string
	if json.Unmarshal(raw.Message, &text) == nil {
		base.Message = text
		return []Message{base}
	}
	var obj struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw.Message, &obj) != nil || len(obj.Content) == 0 {
		return []Message{base}
	}
	if json.Unmarshal(obj.Content, &text) == nil {
		base.Message = text
		return []Message{base}
	}
	var blocks []json.RawMessage
	if json.Unmarshal(obj.Content, &blocks) != nil {
		return []Message{base}
	}
	var results []Message
	var texts []string
	for _, b := range blocks {
		var probe struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		}
		if json.Unmarshal(b, &probe) != nil {
			continue
		}
		switch probe.Type {
		case ContentBlockTypeText:
			texts = append(texts, probe.Text)
		case ContentBlockTypeToolResult:
			tr := &ToolResultMessage{Type: SDKToolResultMessage, ToolUseID: probe.ToolUseID, IsError: probe.IsError,
				SessionID: raw.SessionID, Timestamp: raw.Timestamp}
			var out string
			if json.Unmarshal(probe.Content, &out) == nil {
				tr.Output = out
			} else {
				var parts []json.RawMessage
				if json.Unmarshal(probe.Content, &parts) == nil {
					for _, part := range parts {
						if cb, err := UnmarshalContentBlock(part); err == nil {
							tr.Content = append(tr.Content, cb)
						}
					}
				}
			}
			results = append(results, tr)
		}
	}
	if len(texts) == 0 {
		return results
	}
	base.Message = strings.Join(texts, "\n")
	return append([]Message{base}, results...)
}

// parseToolUseMessage parses a tool_use message from an event
func (a *MessageAccumulator) parseToolUseMessage(event protocol.Event) *ToolUseMessage {
	var msg ToolUseMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseToolResultMessage parses a tool_result message from an event
func (a *MessageAccumulator) parseToolResultMessage(event protocol.Event) *ToolResultMessage {
	var msg ToolResultMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseResultMessage parses a result message from an event
func (a *MessageAccumulator) parseResultMessage(event protocol.Event) *ResultMessage {
	var msg ResultMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}

// parseStreamEventMessage parses a stream_event message from an event
func (a *MessageAccumulator) parseStreamEventMessage(event protocol.Event) *StreamEventMessage {
	var msg StreamEventMessage
	if err := unmarshalEvent(event, &msg); err != nil {
		return nil
	}
	return &msg
}
