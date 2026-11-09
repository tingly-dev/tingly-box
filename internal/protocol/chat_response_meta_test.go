package protocol

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatServiceTierFromAnthropic(t *testing.T) {
	ptr := func(s string) *string { return &s }
	assert.Equal(t, ptr("default"), ChatServiceTierFromAnthropic(anthropic.UsageServiceTierStandard))
	assert.Equal(t, ptr("priority"), ChatServiceTierFromAnthropic(anthropic.BetaUsageServiceTierPriority))
	assert.Nil(t, ChatServiceTierFromAnthropic(anthropic.UsageServiceTierBatch))
	assert.Nil(t, ChatServiceTierFromAnthropic(anthropic.UsageServiceTier("")))
}

func TestChatServiceTierFromResponses(t *testing.T) {
	assert.Nil(t, ChatServiceTierFromResponses(""))
	got := ChatServiceTierFromResponses(responses.ResponseServiceTierFlex)
	require.NotNil(t, got)
	assert.Equal(t, "flex", *got)
}

func TestChatModerationFromResponses(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		var rs responses.Response
		require.NoError(t, json.Unmarshal([]byte(`{"id":"r","moderation":null}`), &rs))
		assert.Nil(t, ChatModerationFromResponses(rs.Moderation))
	})

	t.Run("result is wrapped, error passes through", func(t *testing.T) {
		var rs responses.Response
		require.NoError(t, json.Unmarshal([]byte(`{"id":"r","moderation":{
			"input":{"type":"moderation_result","model":"omni-moderation-latest","flagged":false,
				"categories":{"violence":false},"category_scores":{"violence":0.01},"category_applied_input_types":{"violence":["text"]}},
			"output":{"type":"error","code":"timeout","message":"moderation timed out"}}}`), &rs))

		raw := ChatModerationFromResponses(rs.Moderation)
		require.NotNil(t, raw)
		var got map[string]map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))

		assert.Equal(t, "moderation_results", got["input"]["type"])
		assert.Equal(t, "omni-moderation-latest", got["input"]["model"])
		results := got["input"]["results"].([]any)
		require.Len(t, results, 1)
		assert.Equal(t, "moderation_result", results[0].(map[string]any)["type"])
		assert.Equal(t, false, results[0].(map[string]any)["flagged"])

		assert.Equal(t, map[string]any{"type": "error", "code": "timeout", "message": "moderation timed out"}, got["output"])
	})
}
