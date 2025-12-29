package server

import (
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountTokensWithTiktoken(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		messages []anthropic.MessageParam
		system   []anthropic.TextBlockParam
		wantMin  int // Minimum expected tokens (approximate)
	}{
		{
			name:  "simple user message",
			model: "gpt-4o",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello, world!")),
			},
			system:  nil,
			wantMin: 1, // Should at least have some tokens
		},
		{
			name:  "message with system prompt",
			model: "gpt-4o",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("What is the capital of France?")),
			},
			system: []anthropic.TextBlockParam{
				{Text: "You are a helpful assistant."},
			},
			wantMin: 10, // Should count both system and message
		},
		{
			name:  "conversation with multiple messages",
			model: "gpt-4o",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello!")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi there! How can I help?")),
				anthropic.NewUserMessage(anthropic.NewTextBlock("Tell me a joke.")),
			},
			system: []anthropic.TextBlockParam{
				{Text: "You are a funny assistant."},
			},
			wantMin: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := countTokensWithTiktoken(tt.model, tt.messages, tt.system)
			fmt.Printf("t: %s, count: %d\n", tt.name, count)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, tt.wantMin, "token count should be at least %d", tt.wantMin)
			assert.Less(t, count, 10000, "token count seems unreasonably high")
		})
	}
}
