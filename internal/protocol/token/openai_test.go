package token

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
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
			params := &anthropic.MessageCountTokensParams{
				Model:    anthropic.Model(tt.model),
				Messages: tt.messages,
				System:   anthropic.MessageCountTokensParamsSystemUnion{OfTextBlockArray: tt.system},
			}
			count, err := CountTokensViaTiktoken(params)
			fmt.Printf("t: %s, count: %d\n", tt.name, count)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, tt.wantMin, "token count should be at least %d", tt.wantMin)
			assert.Less(t, count, 10000, "token count seems unreasonably high")
		})
	}
}

// TestEstimateInputTokensApprox verifies the cheap len/4 approximation counts
// the request's billable text without tiktoken, stays in the same ballpark as
// the exact estimator, and scales with content size. It backs the pre-computed
// fallback the streaming passthrough hands to its handler.
func TestEstimateInputTokensApprox(t *testing.T) {
	small := &openai.ChatCompletionNewParams{
		Model: "gpt-4o",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello, world!"),
		},
	}
	big := &openai.ChatCompletionNewParams{
		Model: "gpt-4o",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(strings.Repeat("lorem ipsum dolor ", 5000)),
		},
	}

	approxSmall := EstimateInputTokensSimple(small)
	approxBig := EstimateInputTokensSimple(big)

	assert.Greater(t, approxSmall, 0, "approx must count some tokens")
	assert.Greater(t, approxBig, approxSmall*10, "approx must scale with content size")

	// Same ballpark as the exact tiktoken estimator (within ~3x either way).
	exactBig, err := EstimateInputTokens(big)
	require.NoError(t, err)
	require.Greater(t, exactBig, 0)
	ratio := float64(approxBig) / float64(exactBig)
	assert.Greater(t, ratio, 0.33, "approx should not be wildly below exact")
	assert.Less(t, ratio, 3.0, "approx should not be wildly above exact")
}
