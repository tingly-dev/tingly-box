package statusline

// =============================================
// Claude Code Status Line API Models
// =============================================

// StatusInput represents the input from Claude Code status line
// Ref: https://code.claude.com/docs/en/statusline.md
type StatusInput struct {
	Model         Model         `json:"model"`
	CWD           string        `json:"cwd"`
	Workspace     Workspace     `json:"workspace"`
	Cost          Cost          `json:"cost"`
	ContextWindow ContextWindow `json:"context_window"`
	// Additional fields
	Exceeds200kTokens bool        `json:"exceeds_200k_tokens"`
	SessionID         string      `json:"session_id"`
	TranscriptPath    string      `json:"transcript_path"`
	Version           string      `json:"version"`
	OutputStyle       OutputStyle `json:"output_style"`
	Vim               Vim         `json:"vim"`
	Agent             Agent       `json:"agent"`
}

// Model represents model information from Claude Code
type Model struct {
	ID           string `json:"id" example:"claude-sonnet-4-6"`
	DisplayName  string `json:"display_name" example:"Claude Sonnet 4.6"`
	ProviderName string `json:"provider_name" example:"anthropic"`
}

// Workspace represents workspace information
type Workspace struct {
	CurrentDir string `json:"current_dir" example:"/Users/user/project"`
	ProjectDir string `json:"project_dir" example:"/Users/user/project"`
}

// ContextWindow represents context window information
type ContextWindow struct {
	TotalInputTokens    int          `json:"total_input_tokens" example:"15000"`
	TotalOutputTokens   int          `json:"total_output_tokens" example:"5000"`
	ContextWindowSize   int          `json:"context_window_size" example:"200000"`
	UsedPercentage      float64      `json:"used_percentage" example:"7.5"`
	RemainingPercentage float64      `json:"remaining_percentage" example:"92.5"`
	CurrentUsage        CurrentUsage `json:"current_usage"`
}

// CurrentUsage represents token counts from the last API call
type CurrentUsage struct {
	InputTokens  int `json:"input_tokens" example:"1500"`
	OutputTokens int `json:"output_tokens" example:"500"`
	CacheRead    int `json:"cache_read" example:"10000"`
	CacheWrite   int `json:"cache_write" example:"2000"`
}

// Cost represents cost information
type Cost struct {
	TotalCostUSD       float64 `json:"total_cost_usd" example:"0.05"`
	TotalDurationMs    int64   `json:"total_duration_ms" example:"120000"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms" example:"30000"`
	TotalLinesAdded    int     `json:"total_lines_added" example:"150"`
	TotalLinesRemoved  int     `json:"total_lines_removed" example:"50"`
}

// OutputStyle represents output style information
type OutputStyle struct {
	Name string `json:"name" example:"default"`
}

// Vim represents vim mode information
type Vim struct {
	Mode string `json:"mode" example:"NORMAL"`
}

// Agent represents agent information
type Agent struct {
	Name string `json:"name" example:"claude-opus-4-6"`
}

// CombinedStatus represents combined status from Claude Code and Tingly Box
type CombinedStatus struct {
	Success bool                `json:"success"`
	Data    *CombinedStatusData `json:"data"`
}

// CombinedStatusData represents the combined status data
type CombinedStatusData struct {
	// Claude Code info
	CCModel             string  `json:"cc_model" example:"Claude Sonnet 4.6"`
	CCUsedPct           int     `json:"cc_used_pct" example:"7"`
	CCUsedTokens        int     `json:"cc_used_tokens" example:"15000"`
	CCMaxTokens         int     `json:"cc_max_tokens" example:"200000"`
	CCCost              float64 `json:"cc_cost" example:"0.05"`
	CCDurationMs        int64   `json:"cc_duration_ms" example:"120000"`
	CCAPIDurationMs     int64   `json:"cc_api_duration_ms" example:"30000"`
	CCLinesAdded        int     `json:"cc_lines_added" example:"150"`
	CCLinesRemoved      int     `json:"cc_lines_removed" example:"50"`
	CCSessionID         string  `json:"cc_session_id" example:"session-123"`
	CCExceeds200kTokens bool    `json:"cc_exceeds_200k_tokens"`
	// Tingly Box model mapping info
	TBProviderName string `json:"tb_provider_name,omitempty" example:"openai"`
	TBProviderUUID string `json:"tb_provider_uuid,omitempty" example:"uuid-1234"`
	TBModel        string `json:"tb_model,omitempty" example:"gpt-4"`
	TBRequestModel string `json:"tb_request_model,omitempty" example:"gpt-4"`
	TBScenario     string `json:"tb_scenario,omitempty" example:"openai"`
	// Provider quota information
	TBQuotaAvailable bool   `json:"tb_quota_available" example:"true"`                           // Whether quota is available
	TBQuotaUsed      int    `json:"tb_quota_used,omitempty" example:"40000"`                     // Quota used
	TBQuotaLimit     int    `json:"tb_quota_limit,omitempty" example:"100000"`                   // Quota limit
	TBQuotaPercent   int    `json:"tb_quota_percent,omitempty" example:"40"`                     // Quota percentage
	TBQuotaWindow    string `json:"tb_quota_window,omitempty" example:"daily"`                   // Window type
	TBQuotaUnit      string `json:"tb_quota_unit,omitempty" example:"tokens"`                    // Unit
	TBQuotaResetsAt  string `json:"tb_quota_resets_at,omitempty" example:"2026-04-03T00:00:00Z"` // ISO 8601 reset time
}
