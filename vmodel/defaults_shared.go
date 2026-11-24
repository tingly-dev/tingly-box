package vmodel

import "time"

// MockUsage carries deterministic token-usage values that a mock model
// emits over its streaming wire format. All fields are optional; a zero
// value means "do not advertise this dimension". Used so streaming
// converters / observers can be tested for completeness (cache and
// reasoning tokens, not just plain prompt/completion).
type MockUsage struct {
	PromptTokens      int64 `json:"prompt_tokens,omitempty" yaml:"prompt_tokens,omitempty"`             // input_tokens / prompt_tokens
	CompletionTokens  int64 `json:"completion_tokens,omitempty" yaml:"completion_tokens,omitempty"`     // output_tokens / completion_tokens
	CachedInputTokens int64 `json:"cached_input_tokens,omitempty" yaml:"cached_input_tokens,omitempty"` // cache_read_input_tokens / cached_tokens
	// cache_creation_input_tokens / prompt_tokens_details.cache_write_tokens
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty" yaml:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty" yaml:"reasoning_tokens,omitempty"`
}

// SharedMockSpec is the shared configuration for Go-defined and file-loaded
// mocks. Both protocol packages alias MockModelConfig to this type.
//
// Entries returned by SharedDefaultMocks are user-facing demo defaults:
// they are mounted into the production /virtual/v1/* endpoint and visible
// to end users via the virtual provider. Test-only fixtures must NOT be
// added here; tests should build their own GenericRegistry rather than
// pollute the production defaults set.
type SharedMockSpec struct {
	ID           string           `json:"id" yaml:"id"`
	Name         string           `json:"name,omitempty" yaml:"name,omitempty"`
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	Content      string           `json:"content,omitempty" yaml:"content,omitempty"`
	ToolCalls    []ToolCallConfig `json:"tool_calls,omitempty" yaml:"tool_calls,omitempty"`
	Delay        time.Duration    `json:"delay,omitempty" yaml:"delay,omitempty"`
	StreamChunks []string         `json:"stream_chunks,omitempty" yaml:"stream_chunks,omitempty"`
	// Protocol-native completion reasons; empty values keep each protocol's defaults.
	FinishReason  string          `json:"finish_reason,omitempty" yaml:"finish_reason,omitempty"`
	StopReason    string          `json:"stop_reason,omitempty" yaml:"stop_reason,omitempty"`
	Usage         *MockUsage      `json:"usage,omitempty" yaml:"usage,omitempty"`
	Error         *ErrorInjection `json:"error,omitempty" yaml:"error,omitempty"`
	ErrorCategory ErrorCategory   `json:"error_category,omitempty" yaml:"error_category,omitempty"`
	IsRetryable   bool            `json:"is_retryable,omitempty" yaml:"is_retryable,omitempty"`
	Severity      string          `json:"severity,omitempty" yaml:"severity,omitempty"`
}

// SharedDefaultMocks returns the mocks registered by BOTH the Anthropic and
// OpenAI default registries. Per-protocol unique entries (e.g. virtual-claude-3
// for Anthropic, virtual-gpt-4 for OpenAI, the compact transforms) live in
// their respective sub-packages.
func SharedDefaultMocks() []SharedMockSpec {
	return []SharedMockSpec{
		{
			ID:      "echo-model",
			Name:    "Echo Model",
			Content: "Echo: Your message has been received by the virtual model.",
			Delay:   50 * time.Millisecond,
		},
		{
			ID:   "ask-user-question",
			Name: "Ask User Question",
			ToolCalls: []ToolCallConfig{{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Which approach would you prefer?",
					"options": []map[string]string{
						{"label": "Fast Mode", "value": "fast", "description": "Quick results with less accuracy"},
						{"label": "Accurate Mode", "value": "accurate", "description": "Slower but more accurate results"},
					},
				},
			}},
			Delay: 100 * time.Millisecond,
		},
		{
			ID:   "ask-confirmation",
			Name: "Ask Confirmation",
			ToolCalls: []ToolCallConfig{{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Please confirm to proceed:",
					"options": []map[string]string{
						{"label": "Yes", "value": "yes", "description": "Proceed with the action"},
						{"label": "No", "value": "no", "description": "Cancel the action"},
					},
				},
			}},
			Delay: 50 * time.Millisecond,
		},
		{
			ID:   "web-search-example",
			Name: "Web Search Example",
			ToolCalls: []ToolCallConfig{{
				Name:      "web_search",
				Arguments: map[string]interface{}{"query": "latest AI developments"},
			}},
			Delay: 50 * time.Millisecond,
		},
		// Error models - always fail for testing failover and error handling
		{
			ID:      "virtual-fail-429",
			Name:    "Virtual Fail 429",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  429,
				Message: "simulated rate limit",
				Type:    "rate_limit_error",
			},
			ErrorCategory: ErrorCategoryRateLimit,
			IsRetryable:   true,
			Severity:      "medium",
		},
		{
			ID:      "virtual-fail-500",
			Name:    "Virtual Fail 500",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  500,
				Message: "simulated upstream error",
				Type:    "api_error",
			},
			ErrorCategory: ErrorCategoryUpstream,
			IsRetryable:   true,
			Severity:      "high",
		},
		{
			ID:      "virtual-fail-midstream-close",
			Name:    "Virtual Fail Midstream Close",
			Content: "hello world this stream will be truncated",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   1,
				MidStreamMode: MidStreamModeConnectionClose,
			},
			ErrorCategory: ErrorCategoryNetwork,
			IsRetryable:   false,
			Severity:      "high",
		},
		{
			ID:      "virtual-fail-midstream-event",
			Name:    "Virtual Fail Midstream Event",
			Content: "hello world this stream will end with an error",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   1,
				MidStreamMode: MidStreamModeErrorEvent,
				Message:       "simulated mid-stream error",
				Type:          "api_error",
			},
			ErrorCategory: ErrorCategoryUpstream,
			IsRetryable:   false,
			Severity:      "medium",
		},
		{
			// Streams real content then ends the chunked body cleanly with no
			// terminal protocol event (no response.completed / finish chunk) —
			// the shape an upstream reverse proxy or gateway timeout produces.
			// Regression coverage for #1384: the gateway must convert this bare
			// EOF into an explicit error event instead of leaving the client to
			// see "stream closed before response.completed".
			ID:      "virtual-fail-midstream-cleaneof",
			Name:    "Virtual Fail Midstream Clean EOF",
			Content: "hello world this stream ends with no terminal event",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   1,
				MidStreamMode: MidStreamModeCleanEOF,
			},
			ErrorCategory: ErrorCategoryUpstream,
			IsRetryable:   false,
			Severity:      "high",
		},
	}
}

// StreamTestMockSpecs returns deterministic stream-test fixtures (static +
// tool variants) that advertise the full usage shape — prompt, completion,
// cached input, cache-creation input, and reasoning tokens. These are
// opt-in (NOT in SharedDefaultMocks): consumers wire them into their own
// registry via RegisterStreamTestMocks helpers in the per-protocol
// sub-packages.
func StreamTestMockSpecs() []SharedMockSpec {
	usage := &MockUsage{
		PromptTokens:      42,
		CompletionTokens:  17,
		CachedInputTokens: 11,
		CacheWriteTokens:  5,
		ReasoningTokens:   9,
	}
	return []SharedMockSpec{
		{
			ID:      "virtual-stream-test",
			Name:    "Virtual Stream Test",
			Content: "Stream test response for usage coverage.",
			Usage:   usage,
		},
		{
			ID:   "virtual-stream-test-tool",
			Name: "Virtual Stream Test (Tool)",
			ToolCalls: []ToolCallConfig{{
				Name:      "stream_test_tool",
				Arguments: map[string]interface{}{"ok": true},
			}},
			Usage: usage,
		},
	}
}

// ExtendedErrorSpecs returns additional error scenarios for testing.
// These are opt-in - register via RegisterExtendedErrorMocks in per-protocol packages.
func ExtendedErrorSpecs() []SharedMockSpec {
	return []SharedMockSpec{
		{
			ID:      "virtual-fail-auth-401",
			Name:    "Virtual Fail Auth 401",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  401,
				Message: "authentication failed",
				Type:    "authentication_error",
			},
			ErrorCategory: ErrorCategoryAuth,
			IsRetryable:   false,
			Severity:      "high",
		},
		{
			ID:      "virtual-fail-502",
			Name:    "Virtual Fail 502",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  502,
				Message: "bad gateway",
				Type:    "api_error",
			},
			ErrorCategory: ErrorCategoryUpstream,
			IsRetryable:   true,
			Severity:      "high",
		},
		{
			ID:      "virtual-fail-503",
			Name:    "Virtual Fail 503",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  503,
				Message: "service unavailable",
				Type:    "overloaded_error",
			},
			ErrorCategory: ErrorCategoryOverloaded,
			IsRetryable:   true,
			Severity:      "medium",
		},
		{
			ID:      "virtual-fail-400",
			Name:    "Virtual Fail 400",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  400,
				Message: "invalid request",
				Type:    "invalid_request_error",
			},
			ErrorCategory: ErrorCategoryInvalid,
			IsRetryable:   false,
			Severity:      "low",
		},
		{
			ID:      "virtual-fail-timeout",
			Name:    "Virtual Fail Timeout",
			Content: "hello world this stream will timeout",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   2,
				MidStreamMode: MidStreamModeConnectionClose,
				Message:       "timeout",
				Type:          "timeout_error",
			},
			ErrorCategory: ErrorCategoryTimeout,
			IsRetryable:   false,
			Severity:      "high",
		},
	}
}
