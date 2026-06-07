package probe

// ProbeResult is the canonical SDK-level probe result, shared by the E2E and
// lightweight probe strategies. It doubles as the JSON payload returned by the
// probe HTTP endpoints (exposed under the E2EData alias).
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

	// Routing trace — populated for TB-loopback probes (provider and rule targets).
	// Empty for direct probes and provider_config probes.
	SelectedProvider     string `json:"selected_provider,omitempty"`
	SelectedProviderUUID string `json:"selected_provider_uuid,omitempty"`
	SelectedModel        string `json:"selected_model,omitempty"`
	RoutingSource        string `json:"routing_source,omitempty"`
	MatchedSmartRule     *int   `json:"matched_smart_rule,omitempty"` // nil = none, ≥0 = index

	// Execution-level facts — the real upstream endpoint TB used, the matched
	// rule, and the flags it applied. Populated for TB-loopback probes.
	UpstreamAPI     string `json:"upstream_api,omitempty"`
	UpstreamURL     string `json:"upstream_url,omitempty"`
	MatchedRule     string `json:"matched_rule,omitempty"`
	MatchedRuleDesc string `json:"matched_rule_desc,omitempty"`
	AppliedFlags    string `json:"applied_flags,omitempty"`
}

// ProbeToolCall represents a tool call in a probe response.
type ProbeToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// toProbeResult builds a ProbeResult carrying the raw (JSON-marshaled)
// upstream response for a successful probe.
func toProbeResult(content string, latencyMs int64, requestURL string, isStreaming bool) *ProbeResult {
	return &ProbeResult{
		Content:    content,
		LatencyMs:  latencyMs,
		RequestURL: requestURL,
		Stream:     isStreaming,
	}
}
