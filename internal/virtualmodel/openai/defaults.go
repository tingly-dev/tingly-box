package openai

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// RegisterDefaults registers the default OpenAI-protocol virtual models
// into r: the static "virtual-gpt-4" / "echo-model" mocks plus the tool-call
// mocks ("ask-user-question", "ask-confirmation", "web-search-example").
//
// Compact-style transform models are Anthropic-only and live in the anthropic
// sub-package; they are intentionally not registered here.
func RegisterDefaults(r *Registry) {
	staticModels := []MockModelConfig{
		{
			ID:      "virtual-gpt-4",
			Name:    "Virtual GPT-4",
			Content: "Hello! This is a response from the virtual GPT-4 model. I'm here to help you test your application without making actual API calls.",
			Delay:   100 * time.Millisecond,
		},
		{
			ID:      "echo-model",
			Name:    "Echo Model",
			Content: "Echo: Your message has been received by the virtual model.",
			Delay:   50 * time.Millisecond,
		},
	}
	for i := range staticModels {
		_ = r.Register(NewMockModel(&staticModels[i]))
	}

	toolModels := []MockModelConfig{
		{
			ID:   "ask-user-question",
			Name: "Ask User Question",
			ToolCall: &virtualmodel.ToolCallConfig{
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
			ToolCall: &virtualmodel.ToolCallConfig{
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
			ToolCall: &virtualmodel.ToolCallConfig{
				Name:      "web_search",
				Arguments: map[string]interface{}{"query": "latest AI developments"},
			},
			Delay: 50 * time.Millisecond,
		},
	}
	for i := range toolModels {
		_ = r.Register(NewMockModel(&toolModels[i]))
	}
}
