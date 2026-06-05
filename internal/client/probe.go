package client

// ProbeResult represents the result of a probe operation (for both simple and streaming)
type ProbeResult struct {
	// Basic fields
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	Content      string `json:"content,omitempty"`
	LatencyMs    int64  `json:"latency_ms"`
	ModelsCount  int    `json:"models_count,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Streaming mode indicator
	Stream bool `json:"stream,omitempty"`

	// Token usage
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`

	// Tool calls (for tool mode)
	ToolCalls []ProbeToolCall `json:"tool_calls,omitempty"`

	// Request URL (for debugging)
	RequestURL string `json:"request_url,omitempty"`
}

// ProbeToolCall represents a tool call in probe response
type ProbeToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ProbeUsage represents token usage from a probe operation
type ProbeUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ProbeMode defines the test mode for a probe request.
type ProbeMode string

const (
	ProbeModeSimple    ProbeMode = "simple"
	ProbeModeStreaming ProbeMode = "streaming"
	ProbeModeTool      ProbeMode = "tool"
)

// ToProbeResult creates a ProbeResult with basic fields
func ToProbeResult(content string, latencyMs int64, requestURL string, isStreaming bool) *ProbeResult {
	return &ProbeResult{
		Content:    content,
		LatencyMs:  latencyMs,
		RequestURL: requestURL,
		Stream:     isStreaming,
	}
}
