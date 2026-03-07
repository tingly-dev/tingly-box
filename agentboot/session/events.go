package session

import "time"

// SessionEvent represents a single event in the session
type SessionEvent struct {
	Type      string    `json:"type"`
	Subtype   string    `json:"subtype,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Data      EventData `json:"data"`
}

// EventData is a union type for different event types
// Implementations should provide EventType() method for type discrimination
type EventData interface {
	EventType() string
}

// SystemInitEvent represents session initialization
type SystemInitEvent struct {
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
}

func (SystemInitEvent) EventType() string { return "system_init" }

// UserMessageEvent represents a user message
type UserMessageEvent struct {
	ParentUUID *string `json:"parentUuid"`
	IsSidechain bool    `json:"isSidechain"`
	UserType   string  `json:"userType"`
	CWD        string  `json:"cwd"`
	SessionID  string  `json:"sessionId"`
	Version    string  `json:"version"`
	GitBranch  string  `json:"gitBranch"`
	Type       string  `json:"type"`
	Message    struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
}

func (UserMessageEvent) EventType() string { return "user" }

// AssistantMessageEvent represents an assistant message
type AssistantMessageEvent struct {
	Message AssistantMessage `json:"message"`
}

func (AssistantMessageEvent) EventType() string { return "assistant" }

// AssistantMessage represents the full assistant message structure
type AssistantMessage struct {
	Model        string         `json:"model"`
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	StopReason   *string        `json:"stop_reason,omitempty"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        *UsageInfo     `json:"usage,omitempty"`
}

// ContentBlock represents a content block in the assistant message
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Input    string `json:"input,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// UsageInfo represents token usage information
type UsageInfo struct {
	InputTokens            int `json:"input_tokens"`
	OutputTokens           int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens   int `json:"cache_read_input_tokens,omitempty"`
}

// ToolUseEvent represents a tool use event
type ToolUseEvent struct {
	Name      string `json:"name"`
	Input     string `json:"input"`
	ToolUseID string `json:"tool_use_id"`
}

func (ToolUseEvent) EventType() string { return "tool_use" }

// ToolResultEvent represents a tool result event
type ToolResultEvent struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	Output    string `json:"output,omitempty"`
}

func (ToolResultEvent) EventType() string { return "tool_result" }

// ResultEvent represents the final result event
type ResultEvent struct {
	Subtype       string    `json:"subtype"`
	Result        string    `json:"result"`
	TotalCostUSD  float64   `json:"total_cost_usd"`
	DurationMS    int       `json:"duration_ms"`
	DurationAPIMS int       `json:"duration_api_ms,omitempty"`
	NumTurns      int       `json:"num_turns"`
	Usage         UsageInfo `json:"usage"`
	Error         string    `json:"error,omitempty"`
}

func (ResultEvent) EventType() string { return "result" }

// FileHistorySnapshotEvent represents file history snapshot
type FileHistorySnapshotEvent struct {
	MessageID  string `json:"messageId"`
	Snapshot   struct {
		MessageID          string `json:"messageId"`
		TrackedFileBackups map[string]interface{} `json:"trackedFileBackups"`
		Timestamp          string `json:"timestamp"`
	} `json:"snapshot"`
	IsSnapshotUpdate bool `json:"isSnapshotUpdate"`
}

func (FileHistorySnapshotEvent) EventType() string { return "file-history-snapshot" }

// SystemEvent represents generic system events (duration, etc.)
type SystemEvent struct {
	Subtype    string `json:"subtype"`
	DurationMS int    `json:"durationMs,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	IsMeta     bool   `json:"isMeta,omitempty"`
}

func (SystemEvent) EventType() string { return "system" }
