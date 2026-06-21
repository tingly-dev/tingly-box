package vmodelclient_test

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

func newVirtualProvider() *typ.Provider {
	return &typ.Provider{Name: "test-vmodel", AuthType: typ.AuthTypeVirtual}
}

// ── OpenAI client ─────────────────────────────────────────────────────────────

func TestOpenAIClient_ChatCompletionsNew(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	req := openai.ChatCompletionNewParams{
		Model:    "virtual-gpt-4",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello!")},
	}
	resp, err := c.ChatCompletionsNew(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Choices, 1)
	assert.NotEmpty(t, resp.Choices[0].Message.Content, "should have text content")
	assert.Greater(t, resp.Usage.PromptTokens, int64(0))
	assert.Greater(t, resp.Usage.CompletionTokens, int64(0))
}

func TestOpenAIClient_ChatCompletionsNew_MissingModel(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	_, err := c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "does-not-exist",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hi")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vmodel not found")
}

func TestOpenAIClient_ChatCompletionsNewStreaming(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	req := openai.ChatCompletionNewParams{
		Model:    "virtual-gpt-4",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello!")},
	}
	stream := c.ChatCompletionsNewStreaming(context.Background(), req)

	var contentChunks int
	var finishReason string
	var trailingUsagePromptTokens int64

	for stream.Next() {
		chunk := stream.Current()
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				contentChunks++
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
		if len(chunk.Choices) == 0 && chunk.Usage.PromptTokens > 0 {
			trailingUsagePromptTokens = chunk.Usage.PromptTokens
		}
	}
	require.NoError(t, stream.Err())

	assert.Greater(t, contentChunks, 0, "should have received content delta chunks")
	assert.Equal(t, "stop", finishReason, "finish_reason should be stop")
	assert.Greater(t, trailingUsagePromptTokens, int64(0), "trailing usage chunk should have prompt tokens")
}

func TestOpenAIClient_ChatCompletionsNewStreaming_ExplicitUsage(t *testing.T) {
	svc := virtualserver.NewService()
	openaivm.RegisterStreamTestMocks(svc.GetOpenAIRegistry())
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	req := openai.ChatCompletionNewParams{
		Model:    "virtual-stream-test",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello!")},
	}
	stream := c.ChatCompletionsNewStreaming(context.Background(), req)

	var trailingChunk *openai.ChatCompletionChunk
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 && chunk.Usage.PromptTokens > 0 {
			cp := chunk
			trailingChunk = &cp
		}
	}
	require.NoError(t, stream.Err())
	require.NotNil(t, trailingChunk, "must have trailing usage chunk")

	// MockUsage from StreamTestMockSpecs: prompt=42 completion=17 cached=11 reasoning=9
	assert.EqualValues(t, 42, trailingChunk.Usage.PromptTokens)
	assert.EqualValues(t, 17, trailingChunk.Usage.CompletionTokens)
	assert.EqualValues(t, 11, trailingChunk.Usage.PromptTokensDetails.CachedTokens)
	assert.EqualValues(t, 9, trailingChunk.Usage.CompletionTokensDetails.ReasoningTokens)
}

func TestOpenAIClient_ChatCompletionsNewStreaming_MissingModel(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	stream := c.ChatCompletionsNewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:    "no-such-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hi")},
	})
	// Stream immediately returns false; error is available via Err().
	for stream.Next() {
	}
	assert.Error(t, stream.Err())
}

func TestOpenAIClient_ListModels(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())

	ids, err := c.ListModels(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	assert.Contains(t, ids, "virtual-gpt-4")
}

func TestOpenAIClient_UnsupportedMethods(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewOpenAIClient(svc.GetOpenAIRegistry(), newVirtualProvider())
	ctx := context.Background()

	_, err := c.ImagesGenerate(ctx, openai.ImageGenerateParams{})
	assert.Error(t, err)

	_, err = c.ResponsesNew(ctx, responses.ResponseNewParams{})
	assert.Error(t, err)

	_, err = c.EmbeddingsNew(ctx, openai.EmbeddingNewParams{})
	assert.Error(t, err)

	assert.Nil(t, c.Client(), "Client() should return nil for vmodel")
}

// ── Anthropic client ──────────────────────────────────────────────────────────

func TestAnthropicClient_BetaMessagesNew(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())

	req := &anthropic.BetaMessageNewParams{
		Model:     "virtual-claude-3",
		MaxTokens: 256,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Hello!"))},
	}
	resp, err := c.BetaMessagesNew(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Content)

	var text string
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			text = blk.Text
		}
	}
	assert.NotEmpty(t, text, "should have text content")
	assert.NotEmpty(t, resp.StopReason)
	assert.Greater(t, resp.Usage.InputTokens, int64(0))
	assert.Greater(t, resp.Usage.OutputTokens, int64(0))
}

func TestAnthropicClient_BetaMessagesNew_MissingModel(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())

	_, err := c.BetaMessagesNew(context.Background(), &anthropic.BetaMessageNewParams{
		Model:     "does-not-exist",
		MaxTokens: 256,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Hi"))},
	})
	require.Error(t, err)
}

func TestAnthropicClient_BetaMessagesNewStreaming(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())

	req := &anthropic.BetaMessageNewParams{
		Model:     "virtual-claude-3",
		MaxTokens: 256,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Hello!"))},
	}
	stream := c.BetaMessagesNewStreaming(context.Background(), req)

	// Use the SDK accumulator — it validates wire protocol compliance.
	var msg anthropic.BetaMessage
	for stream.Next() {
		ev := stream.Current()
		require.NoError(t, msg.Accumulate(ev))
	}
	require.NoError(t, stream.Err())

	require.NotEmpty(t, msg.Content, "accumulated message should have content")
	assert.NotEmpty(t, msg.StopReason)
	assert.Greater(t, msg.Usage.OutputTokens, int64(0))
}

func TestAnthropicClient_BetaMessagesNewStreaming_MissingModel(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())

	stream := c.BetaMessagesNewStreaming(context.Background(), &anthropic.BetaMessageNewParams{
		Model:     "no-such-model",
		MaxTokens: 256,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Hi"))},
	})
	for stream.Next() {
	}
	assert.Error(t, stream.Err())
}

func TestAnthropicClient_ListModels(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())

	ids, err := c.ListModels(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	assert.Contains(t, ids, "virtual-claude-3")
}

func TestAnthropicClient_UnsupportedMethods(t *testing.T) {
	svc := virtualserver.NewService()
	c := vmodelclient.NewAnthropicClient(svc.GetAnthropicRegistry(), newVirtualProvider())
	ctx := context.Background()

	_, err := c.MessagesNew(ctx, &anthropic.MessageNewParams{})
	assert.Error(t, err)

	_, err = c.MessagesCountTokens(ctx, &anthropic.MessageCountTokensParams{})
	assert.Error(t, err)

	_, err = c.BetaMessagesCountTokens(ctx, &anthropic.BetaMessageCountTokensParams{})
	assert.Error(t, err)

	assert.Nil(t, c.Client(), "Client() should return nil for vmodel")
}
