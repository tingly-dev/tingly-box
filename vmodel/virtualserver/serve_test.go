package virtualserver

import (
	"context"
	"net/http"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicopt "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
)

// These tests drive Serve through the OFFICIAL SDKs over the in-memory
// listener: the exact path a vmodel provider takes in the gateway. They pin
// down what the old in-process clients could not reproduce — real status codes
// on injected errors and real SSE framing.

func newServed(t *testing.T) (*Server, *http.Client) {
	t.Helper()
	srv := Serve(NewService())
	t.Cleanup(func() { _ = srv.Close() })
	return srv, &http.Client{Transport: vmodelclient.NewTransport(srv.DialContext)}
}

// openAISDK returns an OpenAI SDK client pointed at a fresh Server.
func openAISDK(t *testing.T) openai.Client {
	t.Helper()
	_, hc := newServed(t)
	return openai.NewClient(
		openaiopt.WithBaseURL("http://vmodel.internal/openai/v1"),
		openaiopt.WithAPIKey("EMPTY"),
		openaiopt.WithHTTPClient(hc),
		openaiopt.WithMaxRetries(0),
	)
}

// anthropicSDK returns an Anthropic SDK client pointed at a fresh Server.
func anthropicSDK(t *testing.T) anthropicsdk.Client {
	t.Helper()
	_, hc := newServed(t)
	return anthropicsdk.NewClient(
		anthropicopt.WithBaseURL("http://vmodel.internal/anthropic"),
		anthropicopt.WithAPIKey("EMPTY"),
		anthropicopt.WithHTTPClient(hc),
		anthropicopt.WithMaxRetries(0),
	)
}

func TestServe_OpenAI_NonStreaming(t *testing.T) {
	client := openAISDK(t)
	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model:    "echo-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("ping")},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Contains(t, resp.Choices[0].Message.Content, "Echo")
	assert.Greater(t, resp.Usage.TotalTokens, int64(0))
}

func TestServe_OpenAI_Streaming(t *testing.T) {
	client := openAISDK(t)
	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:    "echo-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("stream me")},
	})
	var text string
	chunks := 0
	for stream.Next() {
		chunks++
		for _, ch := range stream.Current().Choices {
			text += ch.Delta.Content
		}
	}
	require.NoError(t, stream.Err())
	assert.Greater(t, chunks, 1, "must arrive as multiple SSE frames")
	assert.Contains(t, text, "Echo")
}

func TestServe_OpenAI_InjectedStatusIsPreserved(t *testing.T) {
	client := openAISDK(t)
	for _, tc := range []struct {
		model  string
		status int
	}{{"virtual-fail-429", 429}, {"virtual-fail-500", 500}} {
		_, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
			Model:    tc.model,
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("x")},
		})
		var apiErr *openai.Error
		require.ErrorAs(t, err, &apiErr, tc.model)
		assert.Equal(t, tc.status, apiErr.StatusCode, tc.model)
	}
}

func TestServe_OpenAI_ModelsOnlyListOpenAIRegistry(t *testing.T) {
	client := openAISDK(t)
	page, err := client.Models.List(context.Background())
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, m := range page.Data {
		ids[m.ID] = true
	}
	assert.True(t, ids["virtual-gpt-4"])
	assert.False(t, ids["virtual-claude-3"], "anthropic-only model must not appear on the openai root")
}

func TestServe_Anthropic_NonStreaming(t *testing.T) {
	client := anthropicSDK(t)
	msg, err := client.Messages.New(context.Background(), anthropicsdk.MessageNewParams{
		Model:     "echo-model",
		MaxTokens: 64,
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hello"))},
	})
	require.NoError(t, err)
	require.NotEmpty(t, msg.Content)
	assert.Contains(t, msg.Content[0].Text, "Echo")
}

func TestServe_Anthropic_Streaming(t *testing.T) {
	client := anthropicSDK(t)
	stream := client.Messages.NewStreaming(context.Background(), anthropicsdk.MessageNewParams{
		Model:     "echo-model",
		MaxTokens: 64,
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("stream"))},
	})
	msg := anthropicsdk.Message{}
	for stream.Next() {
		require.NoError(t, msg.Accumulate(stream.Current()))
	}
	require.NoError(t, stream.Err())
	require.NotEmpty(t, msg.Content)
	assert.Contains(t, msg.Content[0].Text, "Echo")
}

func TestServe_Anthropic_InjectedStatusIsPreserved(t *testing.T) {
	client := anthropicSDK(t)
	_, err := client.Messages.New(context.Background(), anthropicsdk.MessageNewParams{
		Model:     "virtual-fail-429",
		MaxTokens: 8,
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("x"))},
	})
	var apiErr *anthropicsdk.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 429, apiErr.StatusCode)
}

func TestServe_CloseUnblocksDial(t *testing.T) {
	srv := Serve(NewService())
	require.NoError(t, srv.Close())
	_, err := srv.DialContext(context.Background(), "tcp", "vmodel.internal:80")
	require.Error(t, err)
}

func TestMemListener_DialHonoursContext(t *testing.T) {
	ln := newMemListener() // nobody accepting
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ln.Dial(ctx, "", "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestServe_UnsimulatedEndpointsReturn501(t *testing.T) {
	oc := openAISDK(t)
	_, err := oc.Embeddings.New(context.Background(), openai.EmbeddingNewParams{
		Model: "echo-model",
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String("x")},
	})
	var oaiErr *openai.Error
	require.ErrorAs(t, err, &oaiErr)
	assert.Equal(t, http.StatusNotImplemented, oaiErr.StatusCode)
	assert.Contains(t, err.Error(), "not supported by vmodel")

	ac := anthropicSDK(t)
	_, err = ac.Messages.CountTokens(context.Background(), anthropicsdk.MessageCountTokensParams{
		Model:    "echo-model",
		Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("x"))},
	})
	var anErr *anthropicsdk.Error
	require.ErrorAs(t, err, &anErr)
	assert.Equal(t, http.StatusNotImplemented, anErr.StatusCode)
}
