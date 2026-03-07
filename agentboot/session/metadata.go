package session

import "time"

// SessionStatus represents session state
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusComplete SessionStatus = "complete"
	SessionStatusError    SessionStatus = "error"
)

// SessionMetadata holds parsed session information
type SessionMetadata struct {
	SessionID   string        `json:"session_id"`
	ProjectPath string        `json:"project_path"`
	Status      SessionStatus `json:"status"`

	// Time information
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`

	// Session summary
	FirstMessage   string `json:"first_message,omitempty"`   // User's first prompt
	LastUserMessage string `json:"last_user_message,omitempty"` // Last user message (for context)
	LastAssistantMessage string `json:"last_assistant_message,omitempty"` // Last assistant response
	LastResult     string `json:"last_result,omitempty"`     // Final result/error message
	NumTurns       int    `json:"num_turns,omitempty"`       // Number of exchanges

	// Usage metrics
	InputTokens     int     `json:"input_tokens,omitempty"`
	OutputTokens    int     `json:"output_tokens,omitempty"`
	CacheReadTokens int     `json:"cache_read_tokens,omitempty"`
	TotalCostUSD    float64 `json:"total_cost_usd,omitempty"`

	// Error (if failed)
	Error string `json:"error,omitempty"`
}
