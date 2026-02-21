package claude

import (
	"encoding/json"
	"time"
)

// MessageType constants for Claude Code stream JSON
const (
	MessageTypeSystem      = "system"
	MessageTypeAssistant   = "assistant"
	MessageTypeUser        = "user"
	MessageTypeToolUse     = "tool_use"
	MessageTypeToolResult  = "tool_result"
	MessageTypeResult      = "result"
	MessageTypeStreamEvent = "stream_event"
)

// Message is the interface for all Claude message types
type Message interface {
	GetType() string
	GetTimestamp() time.Time
	GetRawData() map[string]interface{}
}

// SystemMessage represents system/init messages
type SystemMessage struct {
	Type      string    `json:"type"`
	SubType   string    `json:"subtype,omitempty"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
}

// GetType implements Message
func (m *SystemMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *SystemMessage) GetTimestamp() time.Time {
	return m.Timestamp
}

// GetRawData implements Message
func (m *SystemMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// AssistantMessage represents assistant messages with content blocks
type AssistantMessage struct {
	Type            string      `json:"type"`
	Message         MessageData `json:"message"`
	ParentToolUseID *string     `json:"parent_tool_use_id,omitempty"`
	SessionID       string      `json:"session_id"`
	UUID            string      `json:"uuid"`
	Timestamp       time.Time   `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *AssistantMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *AssistantMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *AssistantMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// MessageData matches Claude's message structure from the API
type MessageData struct {
	Model        string         `json:"model"`
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        UsageInfo      `json:"usage,omitempty"`
}

// UsageInfo contains token usage statistics
type UsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens"`
}

// ContentBlock types
type ContentBlock interface {
	GetContentType() string
}

// TextBlock represents text content
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GetContentType implements ContentBlock
func (b *TextBlock) GetContentType() string {
	return b.Type
}

// ToolUseBlock represents a tool use invocation
type ToolUseBlock struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// GetContentType implements ContentBlock
func (b *ToolUseBlock) GetContentType() string {
	return b.Type
}

// ThinkingBlock represents reasoning/thinking content
type ThinkingBlock struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
}

// GetContentType implements ContentBlock
func (b *ThinkingBlock) GetContentType() string {
	return b.Type
}

// ToolResultContentBlock represents tool result content (within message content array)
type ToolResultContentBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// GetContentType implements ContentBlock
func (b *ToolResultContentBlock) GetContentType() string {
	return b.Type
}

// UnmarshalContentBlock unmarshals a content block from JSON
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	var typeDetect struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeDetect); err != nil {
		return nil, err
	}

	switch typeDetect.Type {
	case "text":
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "tool_use":
		var block ToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "thinking":
		var block ThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "tool_result":
		var block ToolResultContentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	default:
		// Return unknown block type
		var block map[string]interface{}
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &UnknownBlock{Data: block}, nil
	}
}

// UnknownBlock represents an unrecognized content block
type UnknownBlock struct {
	Data map[string]interface{}
}

// GetContentType implements ContentBlock
func (b *UnknownBlock) GetContentType() string {
	if t, ok := b.Data["type"].(string); ok {
		return t
	}
	return "unknown"
}

// UserMessage represents user messages
type UserMessage struct {
	Type            string    `json:"type"`
	Message         string    `json:"message"`
	ParentToolUseID *string   `json:"parent_tool_use_id,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	Timestamp       time.Time `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *UserMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *UserMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *UserMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ToolUseMessage represents a standalone tool use message (from stream)
type ToolUseMessage struct {
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	ToolUseID string                 `json:"tool_use_id"`
	SessionID string                 `json:"session_id,omitempty"`
	Timestamp time.Time              `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *ToolUseMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ToolUseMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ToolUseMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ToolResultMessage represents a tool result message
type ToolResultMessage struct {
	Type      string         `json:"type"`
	Output    string         `json:"output,omitempty"`  // String output
	Content   []ContentBlock `json:"content,omitempty"` // Or structured content
	ToolUseID string         `json:"tool_use_id"`
	IsError   bool           `json:"is_error,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *ToolResultMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ToolResultMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ToolResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ResultMessage represents the final result message
type ResultMessage struct {
	Type              string             `json:"type"`
	SubType           string             `json:"subtype,omitempty"`
	Result            string             `json:"result,omitempty"`
	TotalCostUSD      float64            `json:"total_cost_usd,omitempty"`
	IsError           bool               `json:"is_error,omitempty"`
	DurationMS        int64              `json:"duration_ms,omitempty"`
	DurationAPIMS     int64              `json:"duration_api_ms,omitempty"`
	NumTurns          int                `json:"num_turns,omitempty"`
	Usage             UsageInfo          `json:"usage,omitempty"`
	SessionID         string             `json:"session_id,omitempty"`
	PermissionDenials []PermissionDenial `json:"permission_denials,omitempty"`
	Timestamp         time.Time          `json:"timestamp,omitempty"`
}

// PermissionDenial represents a denied permission request
type PermissionDenial struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

// GetType implements Message
func (m *ResultMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ResultMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// IsSuccess returns true if the result indicates success
func (m *ResultMessage) IsSuccess() bool {
	return m.SubType == "success" || !m.IsError
}

// StreamEventMessage represents real-time streaming delta events
type StreamEventMessage struct {
	Type      string      `json:"type"`
	Event     StreamEvent `json:"event"`
	SessionID string      `json:"session_id,omitempty"`
	Timestamp time.Time   `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *StreamEventMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *StreamEventMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *StreamEventMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// StreamEvent represents a streaming event
type StreamEvent struct {
	Type  string      `json:"type"` // content_block_delta, content_block_start, etc.
	Index int         `json:"index,omitempty"`
	Delta interface{} `json:"delta,omitempty"` // TextDelta, InputJSONDelta, etc.
}

// TextDelta represents incremental text content
type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// InputJSONDelta represents incremental tool input JSON
type InputJSONDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

// MessageDelta represents message-level updates
type MessageDelta struct {
	Type       string     `json:"type"`
	StopReason string     `json:"stop_reason,omitempty"`
	Usage      *UsageInfo `json:"usage,omitempty"`
}
