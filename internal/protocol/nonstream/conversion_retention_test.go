package nonstream

import (
	"encoding/json"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"testing"
)

func TestAnthropicConversionRetainsToolPrecisionAndThinking(t *testing.T) {
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{"id":"m1","type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"visible thought","signature":"opaque"},{"type":"tool_use","id":"t1","name":"lookup","input":{"id":9007199254740993}}]}`), &msg))
	chat := ConvertAnthropicBetaToOpenAIChat(&msg, "model")
	require.Equal(t, `{"id":9007199254740993}`, chat.Choices[0].Message.ToolCalls[0].Function.Arguments)
	require.Equal(t, "visible thought", chat.Choices[0].Message.ReasoningContent)
	resp := ConvertAnthropicBetaToResponsesWire(&msg, "model", "model", nil)
	require.Equal(t, `{"id":9007199254740993}`, *resp.Output[0].Arguments)
}

func TestResponsesToChatRetainsVisibleReasoningSummary(t *testing.T) {
	var resp responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{"id":"r1","status":"completed","output":[{"type":"reasoning","id":"rs1","summary":[{"type":"summary_text","text":"visible summary"}]}]}`), &resp))
	chat := ConvertResponsesToOpenAIChat(&resp, "model")
	require.Equal(t, "visible summary", chat.Choices[0].Message.ReasoningContent)
}

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

func TestResponsesChatNonstreamIncompleteToolCallKeepsTruncation(t *testing.T) {
	var resp responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","id":"fc1","call_id":"call1","name":"lookup","arguments":"{\"id\":"}]}`), &resp))
	require.Equal(t, "length", ConvertResponsesToOpenAIChat(&resp, "model").Choices[0].FinishReason)
}
