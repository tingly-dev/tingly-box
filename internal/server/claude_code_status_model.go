package server

// =============================================
// Claude Code Status Line API Models
// =============================================

// ClaudeCodeStatusInput represents the input from Claude Code status line
// Ref: https://code.claude.com/docs/en/statusline.md
type ClaudeCodeStatusInput struct {
	Model         ClaudeCodeModel         `json:"model"`
	CWD           string                  `json:"cwd"`
	Workspace     ClaudeCodeWorkspace     `json:"workspace"`
	Cost          ClaudeCodeCost          `json:"cost"`
	ContextWindow ClaudeCodeContextWindow `json:"context_window"`
	// Additional fields
	Exceeds200kTokens bool                `json:"exceeds_200k_tokens"`
	SessionID         string              `json:"session_id"`
	TranscriptPath    string              `json:"transcript_path"`
	Version           string              `json:"version"`
	OutputStyle       ClaudeCodeOutputStyle `json:"output_style"`
	Vim               ClaudeCodeVim        `json:"vim"`
	Agent             ClaudeCodeAgent      `json:"agent"`
}

// ClaudeCodeModel represents model information from Claude Code
type ClaudeCodeModel struct {
	ID           string `json:"id" example:"claude-sonnet-4-6"`
	DisplayName  string `json:"display_name" example:"Claude Sonnet 4.6"`
	ProviderName string `json:"provider_name" example:"anthropic"`
}

// ClaudeCodeWorkspace represents workspace information
type ClaudeCodeWorkspace struct {
	CurrentDir string `json:"current_dir" example:"/Users/user/project"`
	ProjectDir string `json:"project_dir" example:"/Users/user/project"`
}

// ClaudeCodeContextWindow represents context window information
type ClaudeCodeContextWindow struct {
	TotalInputTokens     int                      `json:"total_input_tokens" example:"15000"`
	TotalOutputTokens    int                      `json:"total_output_tokens" example:"5000"`
	ContextWindowSize    int                      `json:"context_window_size" example:"200000"`
	UsedPercentage       float64                  `json:"used_percentage" example:"7.5"`
	RemainingPercentage  float64                  `json:"remaining_percentage" example:"92.5"`
	CurrentUsage         ClaudeCodeCurrentUsage   `json:"current_usage"`
}

// ClaudeCodeCurrentUsage represents token counts from the last API call
type ClaudeCodeCurrentUsage struct {
	InputTokens  int `json:"input_tokens" example:"1500"`
	OutputTokens int `json:"output_tokens" example:"500"`
	CacheRead    int `json:"cache_read" example:"10000"`
	CacheWrite   int `json:"cache_write" example:"2000"`
}

// ClaudeCodeCost represents cost information
type ClaudeCodeCost struct {
	TotalCostUSD       float64 `json:"total_cost_usd" example:"0.05"`
	TotalDurationMs    int64   `json:"total_duration_ms" example:"120000"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms" example:"30000"`
	TotalLinesAdded    int     `json:"total_lines_added" example:"150"`
	TotalLinesRemoved  int     `json:"total_lines_removed" example:"50"`
}

// ClaudeCodeOutputStyle represents output style information
type ClaudeCodeOutputStyle struct {
	Name string `json:"name" example:"default"`
}

// ClaudeCodeVim represents vim mode information
type ClaudeCodeVim struct {
	Mode string `json:"mode" example:"NORMAL"`
}

// ClaudeCodeAgent represents agent information
type ClaudeCodeAgent struct {
	Name string `json:"name" example:"claude-opus-4-6"`
}

// ClaudeCodeCombinedStatus represents combined status from Claude Code and Tingly Box
type ClaudeCodeCombinedStatus struct {
	Success bool                       `json:"success"`
	Data    *ClaudeCodeCombinedStatusData `json:"data"`
}

// ClaudeCodeCombinedStatusData represents the combined status data
type ClaudeCodeCombinedStatusData struct {
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
}
