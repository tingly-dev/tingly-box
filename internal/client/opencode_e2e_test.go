package client

import (
	"context"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestE2E_OpenCodeSession drives the real OpenCode Zen upstream through the
// dispatch path a request takes (ClientPool → OpenCodeClient → the vendor
// layer), proving the session header the upstream requires actually reaches
// it (#1713).
//
// Prerequisites:
//   - OPENCODE_API_KEY: a Zen API key
//   - OPENCODE_API_BASE (optional): defaults to the Go subscription base
//   - OPENCODE_MODEL (optional): defaults to a cheap model
//   - OPENCODE_PROXY_URL (optional): provider proxy. The transport pool never
//     inherits HTTP(S)_PROXY from the environment (see NewOpenAIClient), so a
//     sandbox that only reaches the internet through a proxy must configure it
//     here, as a provider would.
//
// Run with: go test -v ./internal/client -run TestE2E_OpenCodeSession
func TestE2E_OpenCodeSession(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set, skipping e2e test")
	}

	apiBase := os.Getenv("OPENCODE_API_BASE")
	if apiBase == "" {
		apiBase = "https://opencode.ai/zen/go/v1"
	}
	model := os.Getenv("OPENCODE_MODEL")
	if model == "" {
		model = "glm-5.3-flash"
	}

	provider := &typ.Provider{
		UUID:     "opencode-e2e-test",
		Name:     "opencode-e2e-test",
		AuthType: ai.AuthTypeAPIKey,
		Token:    apiKey,
		APIBase:  apiBase,
		ProxyURL: os.Getenv("OPENCODE_PROXY_URL"),
		Enabled:  true,
	}

	ctx := typ.WithSessionID(context.Background(), typ.SessionID{
		Source: typ.SessionSourceHeader,
		Value:  "e2e-conversation-a",
	})

	// The pool must hand back the vendor client; without that branch the
	// header is never stamped and the upstream answers 400 MissingSessionID.
	c := NewClientPool().GetOpenAIClient(ctx, provider, model)
	require.NotNil(t, c)
	require.IsType(t, &OpenCodeClient{}, c)

	resp, err := c.ChatCompletionsNew(ctx, openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Int(8),
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("say ok")},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Choices)
	t.Logf("chat: id=%s model=%s tokens=%d", resp.ID, resp.Model, resp.Usage.TotalTokens)

	// #1713 was reported on a streaming request; SSE takes the same transport,
	// but assert it rather than reason about it.
	stream := c.ChatCompletionsNewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Int(8),
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("say ok")},
	})
	defer stream.Close()
	var chunks int
	for stream.Next() {
		chunks++
	}
	require.NoError(t, stream.Err())
	require.Positive(t, chunks, "no SSE chunks arrived")
	t.Logf("stream: %d chunks", chunks)
}

// TestE2E_OpenCodeSessionAnthropicShape covers Zen's Anthropic-style base,
// which rejects a session-less request exactly like the OpenAI-style one.
//
// OPENCODE_ANTHROPIC_MODEL selects the model; not every Zen model answers on
// this shape (glm-5.3-flash 500s upstream regardless of the session header).
func TestE2E_OpenCodeSessionAnthropicShape(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set, skipping e2e test")
	}

	model := os.Getenv("OPENCODE_ANTHROPIC_MODEL")
	if model == "" {
		model = "kimi-k3"
	}

	provider := &typ.Provider{
		UUID:     "opencode-e2e-test-anthropic",
		Name:     "opencode-e2e-test-anthropic",
		AuthType: ai.AuthTypeAPIKey,
		Token:    apiKey,
		APIBase:  "https://opencode.ai/zen/go",
		ProxyURL: os.Getenv("OPENCODE_PROXY_URL"),
		Enabled:  true,
	}

	ctx := typ.WithSessionID(context.Background(), typ.SessionID{
		Source: typ.SessionSourceHeader,
		Value:  "e2e-conversation-b",
	})

	c := NewClientPool().GetAnthropicClient(ctx, provider, model)
	require.NotNil(t, c)

	resp, err := c.MessagesNew(ctx, &anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 8,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("say ok")),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)
	t.Logf("messages: id=%s model=%s tokens=%d", resp.ID, resp.Model, resp.Usage.OutputTokens)
}
