package nonstream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
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
	typed := ConvertChatToResponsesWire(resp, "gpt-x", "gpt-x")
	require.NotNil(t, typed.Usage)
	assert.EqualValues(t, 30, typed.Usage.InputTokensDetails.CachedTokens)
	assert.EqualValues(t, 12, typed.Usage.OutputTokensDetails.ReasoningTokens)
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

func TestConvertAnthropicBetaToResponsesWireToolCall(t *testing.T) {
	resp := &anthropic.BetaMessage{
		ID:   "msg_tool",
		Role: "assistant",
		Type: "message",
		Content: []anthropic.BetaContentBlockUnion{
			{Type: "text", Text: "checking"},
			{
				Type: "tool_use", ID: "call_1", Name: "lookup",
				Input: json.RawMessage(`{"query":"typed wire"}`),
			},
		},
		StopReason: "tool_use",
	}

	converted := ConvertAnthropicBetaToResponsesWire(resp, "public-model", "provider-model")
	require.Len(t, converted.Output, 2)
	assert.Equal(t, "message", converted.Output[0].Type)
	item := converted.Output[1]
	assert.Equal(t, "function_call", item.Type)
	assert.Equal(t, "completed", item.Status)
	assert.Equal(t, "call_1", item.CallID)
	require.NotNil(t, item.Arguments)
	assert.Contains(t, *item.Arguments, "typed wire")

	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(encoded), `"arguments":"{\"query\":\"typed wire\"}"`), string(encoded))
	assert.NotContains(t, string(encoded), `"output_index"`)
}

func TestConvertChatToResponsesWirePreservesToolCalls(t *testing.T) {
	resp := &openai.ChatCompletion{
		ID: "chatcmpl_tool",
		Choices: []openai.ChatCompletionChoice{{
			FinishReason: "tool_calls",
			Message: openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
				ID:   "call_chat_1",
				Type: "function",
				Function: openai.ChatCompletionMessageFunctionToolCallFunction{
					Name: "lookup", Arguments: `{"query":"typed wire"}`,
				},
			}}},
		}},
	}

	converted := ConvertChatToResponsesWire(resp, "public-model", "provider-model")
	require.Len(t, converted.Output, 1)
	item := converted.Output[0]
	assert.Equal(t, "function_call", item.Type)
	// The item id is minted in canonical fc_ form; the chat tool_call id is
	// preserved only as the call_id correlation key.
	assert.Regexp(t, `^fc_[0-9a-f]{32}$`, item.ID)
	assert.Equal(t, "call_chat_1", item.CallID)
	assert.Equal(t, "lookup", item.Name)
	require.NotNil(t, item.Arguments)
	assert.JSONEq(t, `{"query":"typed wire"}`, *item.Arguments)
}

func TestResponsesToAnthropicUsesCallIDForToolResultRoundTrip(t *testing.T) {
	resp := &responses.Response{
		ID: "resp_tool",
		Output: []responses.ResponseOutputItemUnion{{
			ID: "fc_item_1", Type: "function_call", CallID: "call_provider_1", Name: "lookup",
			Arguments: responses.ResponseOutputItemUnionArguments{OfString: `{"query":"round trip"}`},
		}},
	}

	beta := HandleResponsesToAnthropicBeta(resp, "public-model")
	require.Len(t, beta.Content, 1)
	assert.Equal(t, "tool_use", beta.Content[0].Type)
	assert.Equal(t, "call_provider_1", beta.Content[0].ID)

	v1 := HandleResponsesToAnthropicV1(resp, "public-model")
	require.Len(t, v1.Content, 1)
	assert.Equal(t, "tool_use", v1.Content[0].Type)
	assert.Equal(t, "call_provider_1", v1.Content[0].ID)
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
	typed := ConvertAnthropicBetaToResponsesWire(resp, "claude-x", "claude-x")
	require.NotNil(t, typed.Usage)
	assert.EqualValues(t, 66, typed.Usage.InputTokens)
	assert.EqualValues(t, 11, typed.Usage.InputTokensDetails.CachedTokens)
	usage, _ := payload["usage"].(map[string]any)
	require.NotNil(t, usage)

	// Total input = 50 uncached + 11 cache-read + 5 cache-creation = 66.
	assert.EqualValues(t, 66, usage["input_tokens"], "input_tokens must be total prompt cost, not uncached only")
	assert.EqualValues(t, 20, usage["output_tokens"])
	assert.EqualValues(t, 86, usage["total_tokens"])

	inDetails, _ := usage["input_tokens_details"].(map[string]any)
	require.NotNil(t, inDetails, "input_tokens_details must carry cached_tokens")
	assert.EqualValues(t, 11, inDetails["cached_tokens"])
	// Anthropic cache_creation is the same billing concept as OpenAI's
	// cache_write_tokens, so it must not be dropped on the way across.
	assert.EqualValues(t, 5, inDetails["cache_write_tokens"])
}

// TestBuildResponsesPayloadFromChat_CacheWriteTokens covers the gpt-5.6+ shape
// on the Chat→Responses body conversion: cache_write_tokens has to land in
// input_tokens_details, otherwise a downstream gateway sees the write cost
// folded anonymously into input_tokens and cannot bill it at the 1.25x rate.
func TestBuildResponsesPayloadFromChat_CacheWriteTokens(t *testing.T) {
	resp := &openai.ChatCompletion{
		ID: "chatcmpl-2",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop"},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     1000,
			CompletionTokens: 40,
			TotalTokens:      1040,
			PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
				CachedTokens:     600,
				CacheWriteTokens: 150,
			},
		},
	}

	payload := BuildResponsesPayloadFromChat(resp, "gpt-5.6", "gpt-5.6")
	usage, _ := payload["usage"].(map[string]any)
	require.NotNil(t, usage)
	assert.EqualValues(t, 1000, usage["input_tokens"], "input_tokens stays the prompt TOTAL")

	inDetails, _ := usage["input_tokens_details"].(map[string]any)
	require.NotNil(t, inDetails)
	assert.EqualValues(t, 600, inDetails["cached_tokens"])
	assert.EqualValues(t, 150, inDetails["cache_write_tokens"])
}

// TestBuildAnthropicPayloadFromChat_CacheWriteTokens covers the opposite
// direction: OpenAI cache_write_tokens must come out of input_tokens and land
// in cache_creation_input_tokens, keeping the three Anthropic fields disjoint.
func TestBuildAnthropicPayloadFromChat_CacheWriteTokens(t *testing.T) {
	resp := &openai.ChatCompletion{
		ID: "chatcmpl-3",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop"},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     1000,
			CompletionTokens: 40,
			TotalTokens:      1040,
			PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
				CachedTokens:     600,
				CacheWriteTokens: 150,
			},
		},
	}

	out := HandleOpenAIChatToAnthropic(resp, "claude-x")
	require.NotNil(t, out)
	assert.EqualValues(t, 250, out.Usage.InputTokens, "input_tokens = 1000 - 600 read - 150 written")
	assert.EqualValues(t, 600, out.Usage.CacheReadInputTokens)
	assert.EqualValues(t, 150, out.Usage.CacheCreationInputTokens)
	assert.EqualValues(t, 1000,
		out.Usage.InputTokens+out.Usage.CacheReadInputTokens+out.Usage.CacheCreationInputTokens,
		"the three fields must reconstruct the original prompt total")
}

// TestBuildAnthropicPayloadFromChat_ReasoningTokens verifies OpenAI
// reasoning_tokens survives conversion into an Anthropic-shaped response as
// usage.output_tokens_details.thinking_tokens.
func TestBuildAnthropicPayloadFromChat_ReasoningTokens(t *testing.T) {
	resp := &openai.ChatCompletion{
		ID: "chatcmpl-4",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Role: "assistant", Content: "hi"}, FinishReason: "stop"},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     200,
			CompletionTokens: 80,
			TotalTokens:      280,
			CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
				ReasoningTokens: 30,
			},
		},
	}

	out := HandleOpenAIChatToAnthropic(resp, "claude-x")
	require.NotNil(t, out)
	assert.EqualValues(t, 80, out.Usage.OutputTokens)
	assert.EqualValues(t, 30, out.Usage.OutputTokensDetails.ThinkingTokens)
}
