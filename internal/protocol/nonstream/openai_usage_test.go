package nonstream

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildResponsesPayloadFromChat_UsageDetails verifies that cache-read and
// reasoning detail survive the Chat→Responses body conversion instead of being
// dropped to zero.
func TestBuildResponsesPayloadFromChat_UsageDetails(t *testing.T) {
	resp := &openai.ChatCompletion{
		ID: "chatcmpl-1",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop"},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     100,
			CompletionTokens: 40,
			TotalTokens:      140,
			PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
				CachedTokens: 30,
			},
			CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
				ReasoningTokens: 12,
			},
		},
	}

	payload := BuildResponsesPayloadFromChat(resp, "gpt-x", "gpt-x")
	usage, _ := payload["usage"].(map[string]any)
	require.NotNil(t, usage)

	assert.EqualValues(t, 100, usage["input_tokens"])
	assert.EqualValues(t, 40, usage["output_tokens"])

	inDetails, _ := usage["input_tokens_details"].(map[string]any)
	require.NotNil(t, inDetails, "input_tokens_details must carry cached_tokens")
	assert.EqualValues(t, 30, inDetails["cached_tokens"])

	outDetails, _ := usage["output_tokens_details"].(map[string]any)
	require.NotNil(t, outDetails, "output_tokens_details must carry reasoning_tokens")
	assert.EqualValues(t, 12, outDetails["reasoning_tokens"])
}

// TestBuildResponsesPayloadFromAnthropicBeta_UsageDetails verifies that the
// Responses-API input_tokens is the TOTAL prompt cost (uncached + cache-read +
// cache-creation), matching the streaming converter, and that cache-read is
// surfaced as cached_tokens instead of being dropped.
func TestBuildResponsesPayloadFromAnthropicBeta_UsageDetails(t *testing.T) {
	resp := &anthropic.BetaMessage{
		ID:   "msg_1",
		Role: "assistant",
		Type: "message",
		Content: []anthropic.BetaContentBlockUnion{
			{Type: "text", Text: "hi"},
		},
		Usage: anthropic.BetaUsage{
			InputTokens:              50,
			OutputTokens:             20,
			CacheReadInputTokens:     11,
			CacheCreationInputTokens: 5,
		},
		StopReason: "end_turn",
	}

	payload := BuildResponsesPayloadFromAnthropicBeta(resp, "claude-x", "claude-x")
	usage, _ := payload["usage"].(map[string]any)
	require.NotNil(t, usage)

	// Total input = 50 uncached + 11 cache-read + 5 cache-creation = 66.
	assert.EqualValues(t, 66, usage["input_tokens"], "input_tokens must be total prompt cost, not uncached only")
	assert.EqualValues(t, 20, usage["output_tokens"])
	assert.EqualValues(t, 86, usage["total_tokens"])

	inDetails, _ := usage["input_tokens_details"].(map[string]any)
	require.NotNil(t, inDetails, "input_tokens_details must carry cached_tokens")
	assert.EqualValues(t, 11, inDetails["cached_tokens"])
}
