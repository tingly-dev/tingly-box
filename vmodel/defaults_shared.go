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

// ErrorMockSpecs returns opt-in fixtures that always fail. They exist so a
// consumer (priority-routing failover tests, gateway integration tests, demos)
// can register a "broken upstream" mock by name instead of standing up an
// ad-hoc httptest.Server. Each spec covers one of the two failover-relevant
// stages:
//
//   - virtual-fail-precontent-429 / -500: pre-content failures the failover
//     orchestrator MUST retry (gate stays buffered, retryable status).
//   - virtual-fail-midstream-close / -event: mid-stream failures the
//     orchestrator MUST NOT retry (gate committed, bytes on the wire).
//
// Like StreamTestMockSpecs, these are intentionally NOT in SharedDefaultMocks
// so production registries stay clean — register via RegisterErrorMocks.
func ErrorMockSpecs() []SharedMockSpec {
	return []SharedMockSpec{
		{
			ID:      "virtual-fail-precontent-429",
			Name:    "Virtual Fail (Pre-Content 429)",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  429,
				Message: "simulated rate limit",
				Type:    "rate_limit_error",
			},
		},
		{
			ID:      "virtual-fail-precontent-500",
			Name:    "Virtual Fail (Pre-Content 500)",
			Content: "unreachable",
			Error: &ErrorInjection{
				Stage:   ErrorStagePreContent,
				Status:  500,
				Message: "simulated upstream error",
				Type:    "api_error",
			},
		},
		{
			ID:      "virtual-fail-midstream-close",
			Name:    "Virtual Fail (Mid-Stream Connection Close)",
			Content: "hello world this stream will be truncated",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   1,
				MidStreamMode: MidStreamModeConnectionClose,
			},
		},
		{
			ID:      "virtual-fail-midstream-event",
			Name:    "Virtual Fail (Mid-Stream Error Event)",
			Content: "hello world this stream will end with an error",
			Error: &ErrorInjection{
				Stage:         ErrorStageMidStream,
				AfterEvents:   1,
				MidStreamMode: MidStreamModeErrorEvent,
				Message:       "simulated mid-stream error",
				Type:          "api_error",
			},
		},
	}
}
