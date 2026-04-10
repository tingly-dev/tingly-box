package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pv "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestVirtualServer_Lifecycle ensures the virtual server can be started,
// registered, and cleanly shut down within a test.
func TestVirtualServer_Lifecycle(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	require.NotNil(t, vs)
	defer vs.Close()

	url := vs.URL()
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "http://")
}

// TestVirtualServer_OpenAI_ChatCompletions verifies the virtual server returns
// a valid OpenAI chat completions response for a registered scenario.
func TestVirtualServer_OpenAI_ChatCompletions(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	result := vs.SendOpenAIChat(t, pv.TextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "stop", result.FinishReason)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

// TestVirtualServer_OpenAI_ChatCompletions_Streaming verifies SSE streaming works.
func TestVirtualServer_OpenAI_ChatCompletions_Streaming(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	result := vs.SendOpenAIChat(t, pv.TextScenario(), true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content) // assembled from stream
}

// TestVirtualServer_Anthropic_Messages verifies Anthropic /v1/messages format.
func TestVirtualServer_Anthropic_Messages(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	result := vs.SendAnthropicV1(t, pv.TextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
}

// TestVirtualServer_Anthropic_Messages_Streaming verifies Anthropic SSE streaming.
func TestVirtualServer_Anthropic_Messages_Streaming(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	result := vs.SendAnthropicV1(t, pv.TextScenario(), true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}

// TestVirtualServer_Google_GenerateContent verifies Google format responses.
func TestVirtualServer_Google_GenerateContent(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	result := vs.SendGoogle(t, pv.TextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.Content)
}

// TestVirtualServer_ToolUse_OpenAI verifies tool call responses for OpenAI format.
func TestVirtualServer_ToolUse_OpenAI(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.ToolUseScenario())

	result := vs.SendOpenAIChat(t, pv.ToolUseScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.NotEmpty(t, result.ToolCalls[0].ID)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

// TestVirtualServer_ToolUse_Anthropic verifies tool use for Anthropic format.
func TestVirtualServer_ToolUse_Anthropic(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.ToolUseScenario())

	result := vs.SendAnthropicV1(t, pv.ToolUseScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

// TestVirtualServer_Thinking_Anthropic verifies thinking blocks in Anthropic format.
func TestVirtualServer_Thinking_Anthropic(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.ThinkingScenario())

	result := vs.SendAnthropicV1(t, pv.ThinkingScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ThinkingContent)
	assert.NotEmpty(t, result.Content)
}

// TestVirtualServer_ErrorResponse verifies error responses are served correctly.
func TestVirtualServer_ErrorResponse(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.ErrorScenario())

	result := vs.SendOpenAIChat(t, pv.ErrorScenario(), false)
	assert.Equal(t, 429, result.HTTPStatus)
}

// TestVirtualServer_CallCount verifies call tracking works.
func TestVirtualServer_CallCount(t *testing.T) {
	vs := pv.NewVirtualServer(t)
	defer vs.Close()

	vs.RegisterScenario(pv.TextScenario())

	vs.SendOpenAIChat(t, pv.TextScenario(), false)
	vs.SendOpenAIChat(t, pv.TextScenario(), false)

	assert.Equal(t, 2, vs.CallCount())
}
