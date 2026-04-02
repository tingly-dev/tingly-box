package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Test helpers
func newBaseContext() *TransformContext {
	return &TransformContext{
		Extra:         make(map[string]interface{}),
		ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: false},
	}
}

func newOpenAIRequest(model string, maxTokens int64) *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: param.Opt[int64]{Value: maxTokens},
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("test")},
	}
}

func newAnthropicV1Request(model string, maxTokens int64) *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("test"))},
	}
}

func newAnthropicBetaRequest(model string, maxTokens int64) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}
}

func TestBaseTransform_Name(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)
	assert.Equal(t, "base_convert", bt.Name())
}

func TestBaseTransform_ConvertAnthropicV1ToOpenAIChat(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)

	ctx := newBaseContext()
	ctx.Request = newAnthropicV1Request("claude-3-5-sonnet-20241022", 1024)

	err := bt.Apply(ctx)
	require.NoError(t, err)

	openaiReq, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	require.True(t, ok)
	assert.Equal(t, "claude-3-5-sonnet-20241022", string(openaiReq.Model))
	assert.Equal(t, int64(1024), openaiReq.MaxTokens.Value)

	config := ctx.Config.OpenAIConfig
	require.NotNil(t, config)
	assert.False(t, config.HasThinking)
	assert.Equal(t, "low", string(config.ReasoningEffort))
}

func TestBaseTransform_ConvertAnthropicBetaToOpenAIChat(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)

	ctx := newBaseContext()
	ctx.Request = newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 2048)
	ctx.IsStreaming = true

	err := bt.Apply(ctx)
	require.NoError(t, err)

	openaiReq, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	require.True(t, ok)
	assert.True(t, openaiReq.StreamOptions.IncludeUsage.Value)
}

func TestBaseTransform_AlreadyOpenAIChat(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)

	ctx := newBaseContext()
	ctx.Request = newOpenAIRequest("gpt-4", 1024)

	err := bt.Apply(ctx)
	require.NoError(t, err)
	assert.Same(t, ctx.Request, ctx.Request)

	config := ctx.Config.OpenAIConfig
	require.NotNil(t, config)
	assert.False(t, config.HasThinking)
	assert.Equal(t, "none", string(config.ReasoningEffort))
}

func TestBaseTransform_DisableStreamUsage(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)

	ctx := newBaseContext()
	ctx.Request = newAnthropicV1Request("claude-3-5-sonnet-20241022", 1024)
	ctx.IsStreaming = true
	ctx.ScenarioFlags.DisableStreamUsage = true

	err := bt.Apply(ctx)
	require.NoError(t, err)

	openaiReq, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	require.True(t, ok)
	assert.False(t, openaiReq.StreamOptions.IncludeUsage.Value)
}

func TestBaseTransform_UnsupportedRequestType(t *testing.T) {
	bt := NewBaseTransform(protocol.TypeOpenAIChat)

	ctx := newBaseContext()
	ctx.Request = "invalid type"

	err := bt.Apply(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported request type")
}

func TestBaseTransform_UnsupportedTargetType(t *testing.T) {
	bt := NewBaseTransform("unknown")

	ctx := newBaseContext()
	ctx.Request = &anthropic.MessageNewParams{}

	err := bt.Apply(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target API style")
}
