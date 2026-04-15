package server_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server_validate"
)

// TestVirtualClient_BoundToServer verifies the bound-client pattern:
// vs.Client() auto-registers scenarios and sends provider-native requests.

func TestVirtualClient_OpenAIChat_NonStream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendOpenAIChat(t, newTextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
	assert.Equal(t, "stop", result.FinishReason)
	require.NotNil(t, result.Usage)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

func TestVirtualClient_OpenAIChat_Stream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendOpenAIChat(t, newTextScenario(), true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.True(t, result.IsStreaming)
	assert.NotEmpty(t, result.StreamEvents)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
}

func TestVirtualClient_OpenAIResponses_NonStream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	// The Responses endpoint delegates to the OpenAI handler, so use the OpenAI
	// text scenario mock response.
	result := vc.SendOpenAIResponses(t, newTextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.RawBody)
}

func TestVirtualClient_Anthropic_NonStream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendAnthropicV1(t, newTextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
}

func TestVirtualClient_Anthropic_Stream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendAnthropicV1(t, newTextScenario(), true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.True(t, result.IsStreaming)
	assert.NotEmpty(t, result.StreamEvents)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
}

func TestVirtualClient_Google_NonStream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendGoogle(t, newTextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
}

func TestVirtualClient_Google_Stream(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendGoogle(t, newTextScenario(), true)
	require.Equal(t, 200, result.HTTPStatus)
	assert.True(t, result.IsStreaming)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
}

func TestVirtualClient_ToolUse_OpenAI(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendOpenAIChat(t, newToolUseScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestVirtualClient_ToolUse_Anthropic(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendAnthropicV1(t, newToolUseScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestVirtualClient_ToolUse_Google(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendGoogle(t, newToolUseScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

func TestVirtualClient_Error(t *testing.T) {
	vs := server_validate.NewVirtualServer(t)
	vc := vs.Client()

	result := vc.SendOpenAIChat(t, newErrorScenario(), false)
	assert.Equal(t, 429, result.HTTPStatus)
}

func TestVirtualClient_Standalone(t *testing.T) {
	// Standalone: manually register scenario, then use client pointed at same URL.
	vs := server_validate.NewVirtualServer(t)
	s := newTextScenario()
	vs.RegisterScenario(s)

	vc := server_validate.NewVirtualClient(vs.URL())
	result := vc.SendOpenAIChat(t, s, false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "The capital of France is Paris.", result.Content)
}

func TestVirtualClient_WithServer(t *testing.T) {
	// WithServer: create standalone then bind after the fact.
	vs := server_validate.NewVirtualServer(t)
	vc := server_validate.NewVirtualClient(vs.URL()).WithServer(vs)

	result := vc.SendAnthropicV1(t, newTextScenario(), false)
	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.Content)
}
