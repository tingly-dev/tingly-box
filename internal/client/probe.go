package client

import (
	"context"
)

// ProbeResult represents the result of a probe operation
type ProbeResult struct {
	Success          bool
	Message          string
	Content          string
	LatencyMs        int64
	ModelsCount      int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ErrorMessage     string
}

// ProbeUsage represents token usage from a probe operation
type ProbeUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Prober defines the interface for client probe capabilities
type Prober interface {
	// ProbeChatEndpoint tests the chat/messages endpoint with a minimal request
	// Returns a ProbeResult with success status, latency, and any response content
	ProbeChatEndpoint(ctx context.Context, model string) ProbeResult

	// ProbeModelsEndpoint tests the models list endpoint
	// Returns a ProbeResult with the number of available models
	ProbeModelsEndpoint(ctx context.Context) ProbeResult

	// ProbeOptionsEndpoint tests basic connectivity with an OPTIONS request
	// Returns a ProbeResult indicating if the endpoint is reachable
	ProbeOptionsEndpoint(ctx context.Context) ProbeResult
}
