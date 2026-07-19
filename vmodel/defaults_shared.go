package vmodel

import "time"

// MockUsage carries deterministic token-usage values that a mock model
// emits over its streaming wire format. All fields are optional; a zero
// value means "do not advertise this dimension". Used so streaming
// converters / observers can be tested for completeness (cache and
// reasoning tokens, not just plain prompt/completion).
type MockUsage struct {
	PromptTokens             int64 // input_tokens / prompt_tokens
	CompletionTokens         int64 // output_tokens / completion_tokens
	CachedInputTokens        int64 // OpenAI prompt_tokens_details.cached_tokens / Anthropic cache_read_input_tokens
	CacheCreationInputTokens int64 // Anthropic cache_creation_input_tokens (no OpenAI analogue)
	ReasoningTokens          int64 // OpenAI completion_tokens_details.reasoning_tokens
}

// SharedMockSpec describes a built-in mock that is identical across
// protocols (anthropic + openai). Each protocol's RegisterDefaults converts
// a SharedMockSpec into its own protocol-specific MockModelConfig.
//
// Entries returned by SharedDefaultMocks are user-facing demo defaults:
// they are mounted into the production /virtual/v1/* endpoint and visible
// to end users via the virtual provider. Test-only fixtures must NOT be
// added here; tests should build their own GenericRegistry rather than
// pollute the production defaults set.
type SharedMockSpec struct {
	ID       string
	Name     string
	Content  string          // static text response (ignored if ToolCall is set)
	ToolCall *ToolCallConfig // if non-nil, this is a tool model
	Delay    time.Duration
	Usage    *MockUsage      // optional explicit usage to advertise in the stream
	Error    *ErrorInjection // optional synthetic failure for error-injection mocks

	// Error metadata (only meaningful when Error != nil)
	ErrorCategory ErrorCategory // Category of error (rate_limit, upstream, etc.)
	IsRetryable   bool          // Whether failover should retry this error
	Severity      string        // "low", "medium", "high" - for filtering/sorting
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
			ToolCall: &ToolCallConfig{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Which approach would you prefer?",
					"options": []map[string]string{
						{"label": "Fast Mode", "value": "fast", "description": "Quick results with less accuracy"},
						{"label": "Accurate Mode", "value": "accurate", "description": "Slower but more accurate results"},
					},
				},
			},
			Delay: 100 * time.Millisecond,
		},
		{
			ID:   "ask-confirmation",
			Name: "Ask Confirmation",
			ToolCall: &ToolCallConfig{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Please confirm to proceed:",
					"options": []map[string]string{
						{"label": "Yes", "value": "yes", "description": "Proceed with the action"},
						{"label": "No", "value": "no", "description": "Cancel the action"},
					},
				},
			},
			Delay: 50 * time.Millisecond,
		},
		{
			ID:   "web-search-example",
			Name: "Web Search Example",
			ToolCall: &ToolCallConfig{
				Name:      "web_search",
				Arguments: map[string]interface{}{"query": "latest AI developments"},
			},
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
		PromptTokens:             42,
		CompletionTokens:         17,
		CachedInputTokens:        11,
		CacheCreationInputTokens: 5,
		ReasoningTokens:          9,
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
			ToolCall: &ToolCallConfig{
				Name:      "stream_test_tool",
				Arguments: map[string]interface{}{"ok": true},
			},
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
			ID:      "virtual-fail-529",
			Name:    "Virtual Fail 529",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  529,
				Message: "simulated overloaded",
				Type:    "overloaded_error",
			},
			ErrorCategory: ErrorCategoryOverloaded,
			IsRetryable:   true,
			Severity:      "medium",
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
