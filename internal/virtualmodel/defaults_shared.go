package virtualmodel

import "time"

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
