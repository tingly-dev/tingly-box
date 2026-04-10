package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	pv "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestRoundTrip_AnthropicV1_To_OpenAIChat_Text verifies full round-trip:
// client speaks Anthropic V1 → gateway transforms → virtual OpenAI provider →
// response transforms back → client receives Anthropic V1 response.
func TestRoundTrip_AnthropicV1_To_OpenAIChat_Text(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

// TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Text verifies Beta → Responses path.
func TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Text(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pv.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pv.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

// TestRoundTrip_OpenAIChat_To_AnthropicV1_Text verifies OpenAI → Anthropic path.
func TestRoundTrip_OpenAIChat_To_AnthropicV1_Text(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, pv.TextScenario())

	result := env.SendAs(t, protocol.TypeOpenAIChat, pv.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "stop", result.FinishReason)
}

// TestRoundTrip_AnthropicV1_To_Google_Text verifies Anthropic → Google path.
func TestRoundTrip_AnthropicV1_To_Google_Text(t *testing.T) {
	t.Skip("Anthropic→Google target not yet implemented (vendor_adjust transform)")
}

// TestRoundTrip_AnthropicV1_To_OpenAIChat_ToolUse verifies tool call preservation.
func TestRoundTrip_AnthropicV1_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	// Tool call must survive the round-trip in the Anthropic response format
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.NotEmpty(t, result.ToolCalls[0].ID)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

// TestRoundTrip_AnthropicBeta_To_OpenAIChat_ToolUse verifies Beta tool use → OpenAI Chat.
func TestRoundTrip_AnthropicBeta_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pv.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pv.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

// TestRoundTrip_AnthropicV1_To_AnthropicV1_Thinking verifies thinking block passthrough.
func TestRoundTrip_AnthropicV1_To_AnthropicV1_Thinking(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pv.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	// Thinking content must be preserved in Anthropic → Anthropic passthrough
	assert.NotEmpty(t, result.ThinkingContent)
	assert.NotEmpty(t, result.Content)
}

// TestRoundTrip_AnthropicV1_To_OpenAIChat_Thinking verifies thinking is stripped
// when target doesn't support it natively (becomes part of content or dropped).
func TestRoundTrip_AnthropicV1_To_OpenAIChat_Thinking(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	// Response must have text content (thinking may be stripped or embedded)
	assert.NotEmpty(t, result.Content)
}

// TestRoundTrip_AnthropicV1_To_OpenAIChat_MultiTurn verifies conversation history is preserved.
func TestRoundTrip_AnthropicV1_To_OpenAIChat_MultiTurn(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.MultiTurnScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.MultiTurnScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

// TestRoundTrip_AnthropicV1_To_OpenAIChat_Streaming verifies streaming text round-trip.
func TestRoundTrip_AnthropicV1_To_OpenAIChat_Streaming(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.StreamingTextScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	// Response must be SSE format (Anthropic SSE events)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content) // assembled from stream
}

// TestRoundTrip_AnthropicBeta_To_OpenAIChat_StreamingToolUse verifies
// streaming tool call deltas survive the round-trip.
func TestRoundTrip_AnthropicBeta_To_OpenAIChat_StreamingToolUse(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pv.StreamingToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pv.StreamingToolUseScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

// TestRoundTrip_ErrorPassthrough verifies that provider errors are properly
// forwarded to the client in the correct protocol format.
func TestRoundTrip_ErrorPassthrough(t *testing.T) {
	env := pv.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pv.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pv.ErrorScenario(), false)

	// Error must be surfaced to the client
	assert.NotEqual(t, 200, result.HTTPStatus)
}

// TestRoundTrip_AllSources_TextScenario_NonStreaming runs text scenario for all
// source protocols against OpenAI Chat target. Verifies basic connectivity of all paths.
func TestRoundTrip_AllSources_TextScenario_NonStreaming(t *testing.T) {
	sources := []protocol.APIType{
		protocol.TypeAnthropicV1,
		protocol.TypeAnthropicBeta,
		protocol.TypeOpenAIChat,
		protocol.TypeOpenAIResponses,
	}

	for _, src := range sources {
		src := src
		t.Run(string(src), func(t *testing.T) {
			t.Parallel()
			env := pv.NewTestEnv(t)
			defer env.Close()

			env.SetupRoute(src, protocol.TypeOpenAIChat, pv.TextScenario())

			result := env.SendAs(t, src, pv.TextScenario(), false)
			require.Equal(t, 200, result.HTTPStatus)
			assert.NotEmpty(t, result.Content)
		})
	}
}
