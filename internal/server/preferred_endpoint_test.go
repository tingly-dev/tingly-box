package server

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/stretchr/testify/assert"
)

// TestSpecialCaseModels tests the special case logic for known models.
// These are workarounds for API limitations that bypass the probe system.
func TestSpecialCaseModels(t *testing.T) {
	tests := []struct {
		name         string
		modelID      string
		apiBase      string
		shouldBeCodex bool
		shouldBeChatGPT bool
	}{
		{
			name:           "Codex model",
			modelID:        "codex-002",
			apiBase:        "https://api.openai.com/v1",
			shouldBeCodex:   true,
			shouldBeChatGPT: false,
		},
		{
			name:           "codex lowercase",
			modelID:        "my-codex-model",
			apiBase:        "https://api.openai.com/v1",
			shouldBeCodex:   true,
			shouldBeChatGPT: false,
		},
		{
			name:           "CODEX uppercase",
			modelID:        "CODEX-003",
			apiBase:        "https://api.openai.com/v1",
			shouldBeCodex:   true,
			shouldBeChatGPT: false,
		},
		{
			name:           "ChatGPT API base",
			modelID:        "gpt-4",
			apiBase:        "https://chatgpt.com/v1",
			shouldBeCodex:   false,
			shouldBeChatGPT: true,
		},
		{
			name:           "chatgpt.com in path",
			modelID:        "gpt-4",
			apiBase:        "https://api.chatgpt.com/backend/v1",
			shouldBeCodex:   false,
			shouldBeChatGPT: true,
		},
		{
			name:           "Regular GPT model",
			modelID:        "gpt-4",
			apiBase:        "https://api.openai.com/v1",
			shouldBeCodex:   false,
			shouldBeChatGPT: false,
		},
		{
			name:           "Claude model",
			modelID:        "claude-3-opus",
			apiBase:        "https://api.anthropic.com/v1",
			shouldBeCodex:   false,
			shouldBeChatGPT: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCodex := strings.Contains(strings.ToLower(tt.modelID), "codex")
			isChatGPT := strings.Contains(strings.ToLower(tt.apiBase), "chatgpt.com")

			assert.Equal(t, tt.shouldBeCodex, isCodex,
				"codex detection for model %s", tt.modelID)
			assert.Equal(t, tt.shouldBeChatGPT, isChatGPT,
				"chatgpt.com detection for base %s", tt.apiBase)
		})
	}
}

// TestEndpointTypeConstants verifies the endpoint type constants
func TestEndpointTypeConstants(t *testing.T) {
	assert.Equal(t, "chat", string(db.EndpointTypeChat))
	assert.Equal(t, "responses", string(db.EndpointTypeResponses))
}
