package nonstream

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestGoogleNonstreamRetainsUsageDetails(t *testing.T) {
	resp := &genai.GenerateContentResponse{UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 100, CachedContentTokenCount: 40, CandidatesTokenCount: 10, ThoughtsTokenCount: 5, TotalTokenCount: 115}}
	chat := HandleGoogleToOpenAI(resp, "model")["usage"].(map[string]interface{})
	require.EqualValues(t, 100, chat["prompt_tokens"])
	require.EqualValues(t, 15, chat["completion_tokens"])
	require.EqualValues(t, 115, chat["total_tokens"])
	require.EqualValues(t, 40, chat["prompt_tokens_details"].(map[string]interface{})["cached_tokens"])
	require.EqualValues(t, 5, chat["completion_tokens_details"].(map[string]interface{})["reasoning_tokens"])
	anthropic := HandleGoogleToAnthropicBeta(resp, "model")
	require.EqualValues(t, 60, anthropic.Usage.InputTokens)
	require.EqualValues(t, 40, anthropic.Usage.CacheReadInputTokens)
	require.EqualValues(t, 15, anthropic.Usage.OutputTokens)
}
