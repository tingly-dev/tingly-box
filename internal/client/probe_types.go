package client

// ProbeStreamResult represents the result of a probeStream operation
// This is a client-side type that mirrors server.ProbeV2Data to avoid import cycles
type ProbeStreamResult struct {
	Content    string                `json:"content,omitempty"`
	Usage      *ProbeStreamUsage     `json:"usage,omitempty"`
	ToolCalls  []ProbeStreamToolCall `json:"tool_calls,omitempty"`
	LatencyMs  int64                 `json:"latency_ms"`
	RequestURL string                `json:"request_url,omitempty"`
}

// ProbeStreamUsage represents token usage from a probeStream operation
type ProbeStreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ProbeStreamToolCall represents a tool call in probe response
type ProbeStreamToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToProbeStreamResult creates a ProbeStreamResult with basic fields
func ToProbeStreamResult(content string, latencyMs int64, requestURL string) *ProbeStreamResult {
	return &ProbeStreamResult{
		Content:    content,
		LatencyMs:  latencyMs,
		RequestURL: requestURL,
	}
}
