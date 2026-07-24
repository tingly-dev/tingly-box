package claude

import (
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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

	// Subagent task fields (populated for task lifecycle subtypes).
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	TaskType    string `json:"task_type,omitempty"`
	ToolUseID   string `json:"tool_use_id,omitempty"`

	// Retry / rate-limit fields (populated when SubType is api_retry/rate_limit).
	// The CLI's exact field spelling has shifted across versions, so the typed
	// fields below are best-effort; the retry* accessors fall back to Raw.
	Attempt    int    `json:"attempt,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
	DelayMS    int64  `json:"delay_ms,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`

	// Raw preserves the full decoded payload. The typed fields above only
	// capture the names known at compile time; Raw keeps everything else so
	// forward-compatible fields (notably the retry metadata, whose names vary
	// by CLI version) survive for formatting and logging. Populated by
	// parseSystemMessage; nil for messages built in code.
	Raw map[string]interface{} `json:"-"`
}

// GetType implements Message
func (m *SystemMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *SystemMessage) GetTimestamp() time.Time {
	return m.Timestamp
}

// GetRawData implements Message. When the message was decoded from the wire it
// returns the full original payload (so unknown fields are not lost); otherwise
// it falls back to marshalling the typed struct.
func (m *SystemMessage) GetRawData() map[string]interface{} {
	if m.Raw != nil {
		return m.Raw
	}
	return marshalToMap(m)
}

// retryAttempt returns the retry attempt number for an api_retry/rate_limit
// notice, checking the typed field first and then Raw under the spellings the
// CLI has used (snake_case and camelCase).
func (m *SystemMessage) retryAttempt() int {
	if m.Attempt > 0 {
		return m.Attempt
	}
	return intFromMap(m.Raw, "attempt", "retry", "retries", "retryCount", "retry_count")
}

// retryDelayMS returns the delay before the next attempt, in milliseconds.
func (m *SystemMessage) retryDelayMS() int64 {
	if m.DelayMS > 0 {
		return m.DelayMS
	}
	return int64(intFromMap(m.Raw, "delay_ms", "delayMs", "delayMS", "retry_after_ms", "retryAfterMs"))
}

// retryReason returns a human-readable reason for the retry, if the CLI
// included one.
func (m *SystemMessage) retryReason() string {
	if m.Error != "" {
		return m.Error
	}
	if m.Message != "" {
		return m.Message
	}
	return stringFromMap(m.Raw, "error", "message", "reason", "errorType", "error_type")
}

// AssistantMessage represents assistant messages with content blocks
type AssistantMessage struct {
	Type            string            `json:"type"`
	Message         anthropic.Message `json:"message"`
	ParentToolUseID *string           `json:"parent_tool_use_id,omitempty"`
	SessionID       string            `json:"session_id"`
	UUID            string            `json:"uuid"`
	Timestamp       time.Time         `json:"timestamp,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// IsError returns true if the assistant message contains an error
func (m *AssistantMessage) IsError() bool {
	return m.Error != ""
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
	return marshalToMap(m)
}

// UsageInfo contains token usage statistics
type UsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens"`
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
	return marshalToMap(m)
}

// ToolCallInfo represents a tool call
type ToolCallInfo struct {
	CallID    string      `json:"call_id"`
	ToolName  string      `json:"tool_name"`
	Input     interface{} `json:"input"`
	Result    interface{} `json:"result"`
	Completed bool        `json:"completed"`
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
	return marshalToMap(m)
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
	return marshalToMap(m)
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
	return marshalToMap(m)
}

// IsSuccess returns true if the result indicates success
func (m *ResultMessage) IsSuccess() bool {
	return m.SubType == ResultSubtypeSuccess || !m.IsError
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
	return marshalToMap(m)
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
