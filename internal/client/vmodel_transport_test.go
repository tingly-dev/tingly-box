package client

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

func withVModelServer(t *testing.T) *virtualserver.Server {
	t.Helper()
	srv := virtualserver.Serve(virtualserver.NewService())
	SetVModelDialer(srv.DialContext)
	t.Cleanup(func() {
		SetVModelDialer(nil)
		_ = srv.Close()
	})
	return srv
}

func vmodelProvider(style protocol.APIStyle) *typ.Provider {
	return &typ.Provider{
		UUID:     "vm-" + string(style),
		Name:     "vm-" + string(style),
		APIBase:  ai.VModelAPIBase(ai.APIStyle(style)),
		APIStyle: style,
		AuthType: typ.AuthTypeVirtual,
		Enabled:  true,
	}
}

func TestProviderBaseURL_RewritesVModelScheme(t *testing.T) {
	assert.Equal(t, "http://vmodel.internal/openai/v1", providerBaseURL(vmodelProvider(protocol.APIStyleOpenAI)))
	real := &typ.Provider{APIBase: "https://api.openai.com/v1"}
	assert.Equal(t, "https://api.openai.com/v1", providerBaseURL(real))
}

// The generic constructors must serve vmodel providers with no special path:
// same SDK, same transport chain, only the dialer differs.
func TestNewOpenAIClient_VModelProvider_RoundTrips(t *testing.T) {
	withVModelServer(t)
	c, err := NewOpenAIClient(vmodelProvider(protocol.APIStyleOpenAI), "echo-model", typ.SessionID{})
	require.NoError(t, err)
	require.NotNil(t, c.Client(), "vmodel providers expose the real SDK client")

	resp, err := c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "echo-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("via pool")},
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Choices[0].Message.Content, "Echo")

	_, err = c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "virtual-fail-429",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("x")},
	})
	var apiErr *openai.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 429, apiErr.StatusCode, "injected status must survive the transport unchanged")
}

func TestNewAnthropicClient_VModelProvider_RoundTrips(t *testing.T) {
	withVModelServer(t)
	c, err := NewAnthropicClient(vmodelProvider(protocol.APIStyleAnthropic), "echo-model", typ.SessionID{})
	require.NoError(t, err)
	require.NotNil(t, c.Client())

	msg, err := c.Client().Messages.New(context.Background(), anthropicsdk.MessageNewParams{
		Model:     "echo-model",
		MaxTokens: 32,
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))},
	})
	require.NoError(t, err)
	assert.Contains(t, msg.Content[0].Text, "Echo")
}

func TestVModelProvider_WithoutDialer_FailsClearly(t *testing.T) {
	SetVModelDialer(nil)
	c, err := NewOpenAIClient(vmodelProvider(protocol.APIStyleOpenAI), "echo-model", typ.SessionID{})
	require.NoError(t, err)
	_, err = c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "echo-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("x")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "virtual-model server is not running")
}
