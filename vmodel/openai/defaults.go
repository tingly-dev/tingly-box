package openai

import (
	"time"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// RegisterDefaults registers the default OpenAI-protocol virtual models into r:
// the OpenAI-only "virtual-gpt-4" mock plus the shared mocks ("echo-model",
// "ask-user-question", "ask-confirmation", "web-search-example").
//
// Compact-style transform models are Anthropic-only and live in the anthropic
// sub-package; they are intentionally not registered here.
func RegisterDefaults(r *Registry) {
	_ = r.Register(NewMockModel(&MockModelConfig{
		ID:      "virtual-gpt-4",
		Name:    "Virtual GPT-4",
		Content: "Hello! This is a response from the virtual GPT-4 model. I'm here to help you test your application without making actual API calls.",
		Delay:   100 * time.Millisecond,
	}))

	for _, spec := range vmodel.SharedDefaultMocks() {
		_ = r.Register(NewMockModel(&MockModelConfig{
			ID:       spec.ID,
			Name:     spec.Name,
			Content:  spec.Content,
			ToolCall: spec.ToolCall,
			Delay:    spec.Delay,
		}))
	}
}
