package db

import (
	"time"
)

// ProtocolType represents the API protocol/protocol style
type ProtocolType string

const (
	ProtocolOpenAI       ProtocolType = "openai"
	ProtocolAnthropic    ProtocolType = "anthropic"
	ProtocolGoogle       ProtocolType = "google"
	ProtocolOpenAICompat ProtocolType = "openai_compat" // OpenAI-compatible APIs
)

// MemoryRoundRecord stores a single conversation round with user input and result
// Designed to be protocol-agnostic for future extensibility
type MemoryRoundRecord struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Scenario     string `gorm:"column:scenario;index:idx_prompt_rounds_scenario;not null"`
	ProviderUUID string `gorm:"column:provider_uuid;index:idx_prompt_rounds_provider;not null"`
	ProviderName string `gorm:"column:provider_name;not null"`
	Model        string `gorm:"column:model;index:idx_prompt_rounds_model;not null"`

	// Protocol Information
	Protocol  ProtocolType `gorm:"column:protocol;index:idx_prompt_rounds_protocol;not null"` // openai, anthropic, google, etc.
	RequestID string       `gorm:"column:request_id;index:idx_prompt_rounds_request_id"`      // Protocol-specific request ID

	// Cross-Protocol Correlation IDs (extracted from protocol-specific metadata)
	// These are common concepts across different APIs for grouping/tracing
	ProjectID string `gorm:"column:project_id;index:idx_prompt_rounds_project_id"` // Project/workspace ID
	SessionID string `gorm:"column:session_id;index:idx_prompt_rounds_session_id"` // Session/conversation ID

	// Extensible Metadata (protocol-specific additional fields)
	// Examples:
	//   - Anthropic: {"user_id": "xxx"}
	//   - OpenAI:    {"organization_id": "yyy"}
	//   - Custom:    any key-value pairs for additional correlation
	Metadata string `gorm:"column:metadata;type:json"` // JSON string

	// Round Data (protocol-agnostic representation)
	RoundIndex    int    `gorm:"column:round_index;not null"` // 0 for current round, 1+ for historical
	UserInput     string `gorm:"column:user_input;type:text;not null"`
	UserInputHash string `gorm:"column:user_input_hash;index:idx_prompt_rounds_user_input_hash;size:64;"` // SHA256 for deduplication
	RoundResult   string `gorm:"column:round_result;type:text"`
	FullMessages  string `gorm:"column:full_messages;type:text"` // JSON: normalized message array

	// Stats
	InputTokens  int `gorm:"column:input_tokens"`
	OutputTokens int `gorm:"column:output_tokens"`
	TotalTokens  int `gorm:"column:total_tokens"`

	// Timestamps
	CreatedAt time.Time `gorm:"column:created_at;index:idx_prompt_rounds_created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`

	// Flags
	IsStreaming  bool `gorm:"column:is_streaming;type:integer"`
	ToolUseCount int  `gorm:"column:tool_use_count;default:0"` // Number of tool use interactions in this round
}

// TableName specifies the table name for GORM
func (MemoryRoundRecord) TableName() string {
	return "vibe_memory"
}

// MemorySessionRecord represents a unique session with aggregated stats
type MemorySessionRecord struct {
	ID                uint      `json:"id"`
	SessionID         string    `json:"session_id"`
	Scenario          string    `json:"scenario"`
	ProviderName      string    `json:"provider_name"`
	Protocol          string    `json:"protocol"`
	Model             string    `json:"model"`
	AccountID         string    `json:"account_id"`   // User ID from metadata
	AccountName       string    `json:"account_name"` // Display name for account
	Title             string    `json:"title"`        // First user input as session title
	CreatedAt         time.Time `json:"created_at"`
	TotalRounds       int       `json:"total_rounds"`
	TotalTokens       int       `json:"total_tokens"`
	TotalInputTokens  int       `json:"total_input_tokens"`
	TotalOutputTokens int       `json:"total_output_tokens"`
}

// PromptMetadata represents protocol-specific metadata in JSON format
type PromptMetadata struct {
	// Anthropic-specific fields (not in top-level fields)
	AnthropicUserID string `json:"anthropic_user_id,omitempty"`

	// OpenAI-specific fields
	OpenAIOrganizationID string `json:"openai_organization_id,omitempty"`
	OpenAIUser           string `json:"openai_user,omitempty"`

	// Additional custom fields for extensibility
	Custom map[string]string `json:"custom,omitempty"`
}

// SetMetadata sets the metadata as JSON string
func (r *MemoryRoundRecord) SetMetadata(metadata interface{}) error {
	if metadata == nil {
		r.Metadata = ""
		return nil
	}
	// Use json.Marshal to convert to JSON string
	// This will be imported in the file that uses this method
	return nil // Placeholder - actual implementation in prompt_store.go
}

// RoundData represents extracted round information for storage
type RoundData struct {
	RoundIndex    int                      // 0 for current, 1+ for historical
	UserInput     string                   // Extracted user input text
	UserInputHash string                   // SHA256 hash of user input for deduplication
	RoundResult   string                   // Assistant's response
	FullMessages  []map[string]interface{} // Normalized message array
	InputTokens   int                      // Input token count
	OutputTokens  int                      // Output token count
	ToolUseCount  int                      // Number of tool use interactions
}
