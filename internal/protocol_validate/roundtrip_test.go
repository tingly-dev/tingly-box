package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	pt "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

func TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_OpenAIChat_To_AnthropicV1_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeOpenAIChat, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "stop", result.FinishReason)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.NotEmpty(t, result.ToolCalls[0].ID)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestRoundTrip_AnthropicBeta_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pt.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

func TestRoundTrip_AnthropicV1_To_AnthropicV1_Thinking(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pt.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ThinkingContent)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Thinking(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_MultiTurn(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.MultiTurnScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.MultiTurnScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Streaming(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.StreamingTextScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicBeta_To_OpenAIChat_StreamingToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.StreamingToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, pt.StreamingToolUseScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

func TestRoundTrip_ErrorPassthrough(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, pt.ErrorScenario(), false)

	assert.NotEqual(t, 200, result.HTTPStatus)
}

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
			env := pt.NewTestEnv(t)
			defer env.Close()

			env.SetupRoute(src, protocol.TypeOpenAIChat, pt.TextScenario())

			result := env.SendAs(t, src, pt.TextScenario(), false)
			require.Equal(t, 200, result.HTTPStatus)
			assert.NotEmpty(t, result.Content)
		})
	}
}
