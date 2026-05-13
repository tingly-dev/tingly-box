package client

import (
	"context"
)

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

// ProbeMode defines the test mode for probeStream
type ProbeMode string

const (
	ProbeModeSimple    ProbeMode = "simple"
	ProbeModeStreaming ProbeMode = "streaming"
	ProbeModeTool      ProbeMode = "tool"
)

// Prober defines the interface for client probe capabilities
type Prober interface {
	// Probe tests the chat/messages endpoint with a minimal request
	// Returns a ProbeResult with success status, latency, and any response content
	Probe(ctx context.Context, model string) ProbeResult

	// ProbeStream performs a streaming probe with configurable test mode
	// Returns ProbeResult with streaming content, tool calls, usage, and latency
	ProbeStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error)
}

// ToProbeResult creates a ProbeResult with basic fields
func ToProbeResult(content string, latencyMs int64, requestURL string, isStreaming bool) *ProbeResult {
	return &ProbeResult{
		Content:    content,
		LatencyMs:  latencyMs,
		RequestURL: requestURL,
		Stream:     isStreaming,
	}
}
