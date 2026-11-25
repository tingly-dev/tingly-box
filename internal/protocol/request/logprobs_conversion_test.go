package request

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two OpenAI protocols spell a logprobs request differently: Chat has a
// boolean logprobs plus top_logprobs, Responses has an include entry plus
// top_logprobs. Losing the translation silently drops the probability data a
// fingerprint collector needs, so pin both directions.

func TestConvertChatToOpenAIResponses_CarriesLogprobs(t *testing.T) {
	params := &openai.ChatCompletionNewParams{
		Model:       "m",
		Messages:    []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
		Logprobs:    openai.Opt(true),
		TopLogprobs: openai.Opt(int64(4)),
	}
	out := ConvertChatToOpenAIResponses(params, 0)
	assert.False(t, param.IsOmitted(out.TopLogprobs))
	assert.Equal(t, int64(4), out.TopLogprobs.Value)
	assert.Contains(t, out.Include, responses.ResponseIncludableMessageOutputTextLogprobs)
}

func TestConvertChatToOpenAIResponses_NoLogprobsLeavesIncludeAlone(t *testing.T) {
	params := &openai.ChatCompletionNewParams{
		Model:    "m",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
		Logprobs: openai.Opt(false),
	}
	out := ConvertChatToOpenAIResponses(params, 0)
	assert.NotContains(t, out.Include, responses.ResponseIncludableMessageOutputTextLogprobs)
	assert.True(t, param.IsOmitted(out.TopLogprobs))
}

func TestConvertOpenAIResponsesToChat_IncludeBecomesLogprobs(t *testing.T) {
	params := &responses.ResponseNewParams{
		Model:   "m",
		Include: []responses.ResponseIncludable{responses.ResponseIncludableMessageOutputTextLogprobs},
	}
	out := ConvertOpenAIResponsesToChat(params, 0)
	require.False(t, param.IsOmitted(out.Logprobs))
	assert.True(t, out.Logprobs.Value)
	assert.True(t, param.IsOmitted(out.TopLogprobs), "include alone asks only for the chosen token")
}

func TestConvertOpenAIResponsesToChat_TopLogprobsImpliesLogprobs(t *testing.T) {
	params := &responses.ResponseNewParams{
		Model:       "m",
		TopLogprobs: openai.Opt(int64(5)),
	}
	out := ConvertOpenAIResponsesToChat(params, 0)
	require.False(t, param.IsOmitted(out.Logprobs))
	assert.True(t, out.Logprobs.Value, "Chat rejects top_logprobs unless logprobs is true")
	require.False(t, param.IsOmitted(out.TopLogprobs))
	assert.Equal(t, int64(5), out.TopLogprobs.Value)
}
