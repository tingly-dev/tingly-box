package agentboot

import (
	"encoding/json"
	"time"
)

// EventType constants for unified agent events
// All agents should map their internal events to these standard types
const (
	// EventTypeInit indicates agent initialization
	EventTypeInit = "init"
	// EventTypeSystem indicates system-level messages
	EventTypeSystem = "system"
	// EventTypeAssistant indicates assistant/agent response messages
	EventTypeAssistant = "assistant"
	// EventTypeUser indicates user messages (echoed back)
	EventTypeUser = "user"
	// EventTypeToolUse indicates a tool is being invoked
	EventTypeToolUse = "tool_use"
	// EventTypeToolResult indicates the result of a tool invocation
	EventTypeToolResult = "tool_result"
	// EventTypePermissionRequest indicates a permission request is pending
	EventTypePermissionRequest = "permission_request"
	// EventTypePermissionResult indicates the result of a permission request
	EventTypePermissionResult = "permission_result"
	// EventTypeResult indicates the final result of execution
	EventTypeResult = "result"
	// EventTypeError indicates an error occurred
	EventTypeError = "error"
	// EventTypeStreamDelta indicates incremental streaming content
	EventTypeStreamDelta = "stream_delta"
)

// AgentMessage is the unified interface for all agent messages
// All agent implementations should convert their messages to this interface
type AgentMessage interface {
	// GetType returns the message type (one of EventType constants)
	GetType() string
	// GetTimestamp returns when the message was created
	GetTimestamp() time.Time
	// GetSessionID returns the session ID if available
	GetSessionID() string
	// GetAgentType returns the source agent type
	GetAgentType() AgentType
	// GetRawData returns the raw message data as a map
	GetRawData() map[string]interface{}
	// ToEvent converts the message to an Event
	ToEvent() Event
}

// BaseMessage provides common fields for message implementations
type BaseMessage struct {
	Type      string    `json:"type"`
	AgentType AgentType `json:"agent_type"`
	SessionID string    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// GetType returns the message type
func (m *BaseMessage) GetType() string {
	return m.Type
}

// GetTimestamp returns the timestamp
func (m *BaseMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetSessionID returns the session ID
func (m *BaseMessage) GetSessionID() string {
	return m.SessionID
}

// GetAgentType returns the agent type
func (m *BaseMessage) GetAgentType() AgentType {
	return m.AgentType
}

// ToEvent converts BaseMessage to Event - should be overridden by embedders
func (m *BaseMessage) ToEvent() Event {
	return Event{
		Type:      m.Type,
		Timestamp: m.Timestamp,
		Data: map[string]interface{}{
			"agent_type": string(m.AgentType),
			"session_id": m.SessionID,
		},
	}
}

// GetRawData returns the raw data - should be overridden by embedders
func (m *BaseMessage) GetRawData() map[string]interface{} {
	return map[string]interface{}{
		"type":       m.Type,
		"agent_type": string(m.AgentType),
		"session_id": m.SessionID,
		"timestamp":  m.Timestamp,
	}
}

// InitMessage represents agent initialization
type InitMessage struct {
	BaseMessage
	MaxIterations int `json:"max_iterations,omitempty"`
}

// ToEvent converts InitMessage to Event
func (m *InitMessage) ToEvent() Event {
	return Event{
		Type: m.Type,
		Data: map[string]interface{}{
			"agent_type":     string(m.AgentType),
			"session_id":     m.SessionID,
			"max_iterations": m.MaxIterations,
		},
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *InitMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// AssistantMessage represents an assistant response
type AssistantMessage struct {
	BaseMessage
	Text    string                 `json:"text,omitempty"`
	Content []ContentBlock         `json:"content,omitempty"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// ContentBlock represents a block of content
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// For tool_use
	ToolID   string                 `json:"tool_id,omitempty"`
	ToolName string                 `json:"tool_name,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
}

// ToEvent converts AssistantMessage to Event
func (m *AssistantMessage) ToEvent() Event {
	data := map[string]interface{}{
		"agent_type": string(m.AgentType),
		"session_id": m.SessionID,
	}
	if m.Text != "" {
		data["message"] = m.Text
	}
	if len(m.Content) > 0 {
		data["content"] = m.Content
	}
	for k, v := range m.Extra {
		data[k] = v
	}
	return Event{
		Type:      m.Type,
		Data:      data,
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *AssistantMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// GetText returns the text content
func (m *AssistantMessage) GetText() string {
	if m.Text != "" {
		return m.Text
	}
	// Concatenate text from content blocks
	var text string
	for _, block := range m.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}

// PermissionRequestMessage represents a permission request
type PermissionRequestMessage struct {
	BaseMessage
	RequestID string                 `json:"request_id"`
	ToolName  string                 `json:"tool_name"`
	Input     map[string]interface{} `json:"input"`
	Reason    string                 `json:"reason,omitempty"`
	Step      int                    `json:"step,omitempty"`
	Total     int                    `json:"total,omitempty"`
}

// ToEvent converts PermissionRequestMessage to Event
func (m *PermissionRequestMessage) ToEvent() Event {
	return Event{
		Type: m.Type,
		Data: map[string]interface{}{
			"agent_type": string(m.AgentType),
			"session_id": m.SessionID,
			"request_id": m.RequestID,
			"tool_name":  m.ToolName,
			"input":      m.Input,
			"reason":     m.Reason,
			"step":       m.Step,
			"total":      m.Total,
		},
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *PermissionRequestMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// PermissionResultMessage represents the result of a permission request
type PermissionResultMessage struct {
	BaseMessage
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
	Reason    string `json:"reason,omitempty"`
	Remember  bool   `json:"remember,omitempty"`
}

// ToEvent converts PermissionResultMessage to Event
func (m *PermissionResultMessage) ToEvent() Event {
	return Event{
		Type: m.Type,
		Data: map[string]interface{}{
			"agent_type": string(m.AgentType),
			"session_id": m.SessionID,
			"request_id": m.RequestID,
			"approved":   m.Approved,
			"reason":     m.Reason,
			"remember":   m.Remember,
		},
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *PermissionResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ResultMessage represents the final result
type ResultMessage struct {
	BaseMessage
	Status    string  `json:"status"` // "success", "error", "cancelled", "permission_denied"
	Message   string  `json:"message,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	Duration  int64   `json:"duration_ms,omitempty"`
	Steps     int     `json:"steps_completed,omitempty"`
	IsError   bool    `json:"is_error,omitempty"`
	ErrorMsg  string  `json:"error,omitempty"`
}

// ToEvent converts ResultMessage to Event
func (m *ResultMessage) ToEvent() Event {
	return Event{
		Type: m.Type,
		Data: map[string]interface{}{
			"agent_type":     string(m.AgentType),
			"session_id":     m.SessionID,
			"status":         m.Status,
			"message":        m.Message,
			"cost_usd":       m.CostUSD,
			"duration_ms":    m.Duration,
			"steps_completed": m.Steps,
			"is_error":       m.IsError,
			"error":          m.ErrorMsg,
		},
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *ResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// IsSuccess returns true if result is successful
func (m *ResultMessage) IsSuccess() bool {
	return m.Status == "success" && !m.IsError
}

// StreamDeltaMessage represents incremental streaming content
type StreamDeltaMessage struct {
	BaseMessage
	Delta string `json:"delta"`
}

// ToEvent converts StreamDeltaMessage to Event
func (m *StreamDeltaMessage) ToEvent() Event {
	return Event{
		Type: m.Type,
		Data: map[string]interface{}{
			"agent_type": string(m.AgentType),
			"session_id": m.SessionID,
			"delta":      m.Delta,
		},
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *StreamDeltaMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// NewInitMessage creates a new init message
func NewInitMessage(agentType AgentType, sessionID string, maxIterations int) *InitMessage {
	return &InitMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypeInit,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		MaxIterations: maxIterations,
	}
}

// NewAssistantMessage creates a new assistant message
func NewAssistantMessage(agentType AgentType, sessionID, text string) *AssistantMessage {
	return &AssistantMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypeAssistant,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		Text: text,
	}
}

// NewPermissionRequestMessage creates a new permission request message
func NewPermissionRequestMessage(agentType AgentType, sessionID, requestID, toolName string, input map[string]interface{}, reason string) *PermissionRequestMessage {
	return &PermissionRequestMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypePermissionRequest,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		RequestID: requestID,
		ToolName:  toolName,
		Input:     input,
		Reason:    reason,
	}
}

// NewPermissionResultMessage creates a new permission result message
func NewPermissionResultMessage(agentType AgentType, sessionID, requestID string, approved bool, reason string) *PermissionResultMessage {
	return &PermissionResultMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypePermissionResult,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		RequestID: requestID,
		Approved:  approved,
		Reason:    reason,
	}
}

// NewResultMessage creates a new result message
func NewResultMessage(agentType AgentType, sessionID, status, message string) *ResultMessage {
	return &ResultMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypeResult,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		Status:  status,
		Message: message,
	}
}

// NewStreamDeltaMessage creates a new stream delta message
func NewStreamDeltaMessage(agentType AgentType, sessionID, delta string) *StreamDeltaMessage {
	return &StreamDeltaMessage{
		BaseMessage: BaseMessage{
			Type:      EventTypeStreamDelta,
			AgentType: agentType,
			SessionID: sessionID,
			Timestamp: time.Now(),
		},
		Delta: delta,
	}
}

// MessageFromEvent converts an Event to an AgentMessage if possible
func MessageFromEvent(event Event, agentType AgentType) AgentMessage {
	base := BaseMessage{
		Type:      event.Type,
		AgentType: agentType,
		Timestamp: event.Timestamp,
	}
	if sid, ok := event.Data["session_id"].(string); ok {
		base.SessionID = sid
	}

	switch event.Type {
	case EventTypeInit:
		msg := &InitMessage{BaseMessage: base}
		if mi, ok := event.Data["max_iterations"].(int); ok {
			msg.MaxIterations = mi
		} else if mi, ok := event.Data["max_iterations"].(float64); ok {
			msg.MaxIterations = int(mi)
		}
		return msg
	case EventTypeAssistant:
		msg := &AssistantMessage{BaseMessage: base}
		if text, ok := event.Data["message"].(string); ok {
			msg.Text = text
		} else if text, ok := event.Data["text"].(string); ok {
			msg.Text = text
		}
		return msg
	case EventTypePermissionRequest:
		msg := &PermissionRequestMessage{BaseMessage: base}
		if rid, ok := event.Data["request_id"].(string); ok {
			msg.RequestID = rid
		}
		if tn, ok := event.Data["tool_name"].(string); ok {
			msg.ToolName = tn
		}
		if input, ok := event.Data["input"].(map[string]interface{}); ok {
			msg.Input = input
		}
		if reason, ok := event.Data["reason"].(string); ok {
			msg.Reason = reason
		}
		if step, ok := event.Data["step"].(int); ok {
			msg.Step = step
		} else if step, ok := event.Data["step"].(float64); ok {
			msg.Step = int(step)
		}
		if total, ok := event.Data["total"].(int); ok {
			msg.Total = total
		} else if total, ok := event.Data["total"].(float64); ok {
			msg.Total = int(total)
		}
		return msg
	case EventTypePermissionResult:
		msg := &PermissionResultMessage{BaseMessage: base}
		if rid, ok := event.Data["request_id"].(string); ok {
			msg.RequestID = rid
		}
		if approved, ok := event.Data["approved"].(bool); ok {
			msg.Approved = approved
		}
		if reason, ok := event.Data["reason"].(string); ok {
			msg.Reason = reason
		}
		return msg
	case EventTypeResult:
		msg := &ResultMessage{BaseMessage: base}
		if status, ok := event.Data["status"].(string); ok {
			msg.Status = status
		}
		if message, ok := event.Data["message"].(string); ok {
			msg.Message = message
		}
		if cost, ok := event.Data["total_cost_usd"].(float64); ok {
			msg.CostUSD = cost
		} else if cost, ok := event.Data["cost_usd"].(float64); ok {
			msg.CostUSD = cost
		}
		if steps, ok := event.Data["steps_completed"].(int); ok {
			msg.Steps = steps
		} else if steps, ok := event.Data["steps_completed"].(float64); ok {
			msg.Steps = int(steps)
		}
		return msg
	case EventTypeStreamDelta:
		msg := &StreamDeltaMessage{BaseMessage: base}
		if delta, ok := event.Data["delta"].(string); ok {
			msg.Delta = delta
		}
		return msg
	default:
		// Return a generic message wrapper
		return &genericMessage{BaseMessage: base, data: event.Data}
	}
}

// genericMessage is a fallback for unknown message types
type genericMessage struct {
	BaseMessage
	data map[string]interface{}
}

// ToEvent converts to Event
func (m *genericMessage) ToEvent() Event {
	return Event{
		Type:      m.Type,
		Data:      m.data,
		Timestamp: m.Timestamp,
	}
}

// GetRawData returns raw data
func (m *genericMessage) GetRawData() map[string]interface{} {
	result := map[string]interface{}{
		"type":       m.Type,
		"agent_type": string(m.AgentType),
		"timestamp":  m.Timestamp,
	}
	for k, v := range m.data {
		result[k] = v
	}
	return result
}
