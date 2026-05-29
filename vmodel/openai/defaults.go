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

// RegisterStreamTestMocks registers the opt-in stream-test fixtures
// (virtual-stream-test, virtual-stream-test-tool) into r. These advertise
// deterministic, fully-populated usage (prompt / completion / cached input /
// reasoning) so streaming converters can be exercised end-to-end without a
// real upstream. Intentionally separate from RegisterDefaults so production
// registries stay clean.
func RegisterStreamTestMocks(r *Registry) {
	for _, spec := range vmodel.StreamTestMockSpecs() {
		_ = r.Register(NewMockModel(&MockModelConfig{
			ID:       spec.ID,
			Name:     spec.Name,
			Content:  spec.Content,
			ToolCall: spec.ToolCall,
			Delay:    spec.Delay,
			Usage:    spec.Usage,
		}))
	}
}

// RegisterErrorMocks registers the opt-in error-injection fixtures
// (virtual-fail-precontent-{429,500}, virtual-fail-midstream-{close,event})
// into r. These always fail and exist so consumers can simulate a broken
// upstream by model name without standing up an ad-hoc httptest.Server.
// Intentionally separate from RegisterDefaults so production registries
// stay clean.
func RegisterErrorMocks(r *Registry) {
	for _, spec := range vmodel.ErrorMockSpecs() {
		_ = r.Register(NewMockModel(&MockModelConfig{
			ID:      spec.ID,
			Name:    spec.Name,
			Content: spec.Content,
			Error:   spec.Error,
		}))
	}
}
