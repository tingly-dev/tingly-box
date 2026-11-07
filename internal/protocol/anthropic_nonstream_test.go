package protocol

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"
)

// TestAnthropicNonStreamRequest sends a real, non-streaming Anthropic Messages
// request. It is intended for manual protocol experiments and is skipped during
// normal test runs unless credentials and a model are explicitly configured.
//
// Run with:
//
//	ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=... \
//	go test -v ./internal/protocol -run TestAnthropicNonStreamRequest
//
// ANTHROPIC_BASE_URL is optional and may point at an Anthropic-compatible
// gateway. Pass the API root without /v1; a trailing /v1 is normalized below.
func TestAnthropicNonStreamRequest(t *testing.T) {
	apiKey := "tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjQwMzQwNSwiaWF0IjoxNzY2MzE3MDA1fQ.AHtmsHxGGJ0jtzvrTZMHC3kfl3Os94HOhMA-zXFtHXQ"
	model := "cc"
	if apiKey == "" || model == "" {
		t.Skip("ANTHROPIC_API_KEY and ANTHROPIC_MODEL must be set")
	}

	baseURL := "http://localhost:12580/tingly/claude_code:p5"
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 64,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Reply with exactly: pong")),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, message)
	require.NotEmpty(t, message.ID)
	require.Equal(t, "assistant", string(message.Role))
	require.NotEmpty(t, message.Content)

	var text strings.Builder
	for _, block := range message.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	require.NotEmpty(t, text.String(), "expected a text block in the non-stream response")

	t.Logf("response id=%s model=%s stop_reason=%s", message.ID, message.Model, message.StopReason)
	t.Logf("content=%q", text.String())
	t.Logf("usage input=%d output=%d", message.Usage.InputTokens, message.Usage.OutputTokens)
}
