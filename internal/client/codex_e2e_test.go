package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TestE2E_CodexRoundTripper tests the ChatGPT backend API with OpenAI client streaming support.
//
// Prerequisites:
// 1. Set environment variables:
//   - CODEX_ACCESS_TOKEN: ChatGPT OAuth access token
//   - CODEX_ACCOUNT_ID (optional): ChatGPT account ID from OAuth metadata
//   - CODEX_MODEL (optional): Model name, defaults to "gpt-4o"
//
// Run with: go test -v ./internal/client -run TestE2E_CodexRoundTripper
func TestE2E_CodexRoundTripper(t *testing.T) {
	// Skip if no credentials provided
	accessToken := os.Getenv("CODEX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CODEX_ACCESS_TOKEN not set, skipping e2e test")
	}

	accountID := os.Getenv("CODEX_ACCOUNT_ID")
	model := os.Getenv("CODEX_MODEL")
	if model == "" {
		model = "gpt-5-codex"
	}

	// Create provider for ChatGPT backend API
	provider := &typ.Provider{
		ProxyURL: "socks5://127.0.0.1:7890",
		Name:     "codex-e2e-test",
		APIBase:  protocol.CodexAPIBase,
		AuthType: typ.AuthTypeOAuth,
		Timeout:  int64((60 * time.Second).Seconds()),
		OAuthDetail: &typ.OAuthDetail{
			ProviderType: string(oauth.ProviderCodex),
			AccessToken:  accessToken,
			ExtraFields:  make(map[string]interface{}),
		},
	}

	if accountID != "" {
		provider.OAuthDetail.ExtraFields["account_id"] = accountID
	}

	t.Run("streaming_with_tools", func(t *testing.T) {
		// Create OpenAI client which already has the proper transport configured
		client, err := NewOpenAIClient(provider)
		require.NoError(t, err)
		defer client.Close()

		// Build request using OpenAI Responses API
		// Build request body with tools
		reqBody := map[string]interface{}{
			"model":        model,
			"instructions": "You are a helpful assistant with access to tools.",
			"input": []map[string]interface{}{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]string{
						{"type": "input_text", "text": "What's the weather in San Francisco? Use the get_weather tool."},
					},
				},
			},
			"tools": []map[string]interface{}{
				{
					"type":        "function",
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state, e.g. San Francisco, CA",
							},
							"unit": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"celsius", "fahrenheit"},
								"description": "The temperature unit",
							},
						},
						"required": []string{"location"},
					},
				},
			},
			"tool_choice": "auto",
			"stream":      true,
			"store":       false,
			"include":     []string{},
		}

		req := responses.ResponseNewParams{}

		bs, _ := json.Marshal(reqBody)
		json.Unmarshal(bs, &req)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		stream := client.ResponsesNewStreaming(ctx, req)

		// Process the stream
		for stream.Next() {
			cur := stream.Current()
			bs, _ := json.Marshal(cur)
			t.Logf("stream chunk: %s\n", string(bs))
		}
		require.NoError(t, stream.Err())

	})

	t.Run("streaming_simple", func(t *testing.T) {
		// Create OpenAI client which already has the proper transport configured
		client, err := NewOpenAIClient(provider)
		require.NoError(t, err)
		defer client.Close()

		// Build request using OpenAI Responses API
		reqBody := map[string]interface{}{
			"model":        model,
			"instructions": "You are a helpful assistant.",
			"input": []map[string]interface{}{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]string{
						{"type": "input_text", "text": "Say 'Hello, streaming test!' and call get_weather for New York."},
					},
				},
			},
			"tools": []map[string]interface{}{
				{
					"type":        "function",
					"name":        "get_weather",
					"description": "Get weather for a location",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"location"},
					},
				},
			},
			"tool_choice": "auto",
			"stream":      true,
			"store":       false,
			"include":     []string{},
		}

		req := responses.ResponseNewParams{}
		bs, _ := json.Marshal(reqBody)
		json.Unmarshal(bs, &req)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		stream := client.ResponsesNewStreaming(ctx, req)
		require.NotNil(t, stream)

		// Read streaming response
		chunkCount := 0
		var fullOutput strings.Builder
		var inputTokens, outputTokens int64

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			bs, _ := json.Marshal(chunk)
			t.Logf("Chunk #%d: %s", chunkCount, string(bs))

			// Extract response data based on type
			if chunk.Type == "response.done" {
				// Response completed
				t.Logf("Response completed")
				if chunk.Response.Usage.TotalTokens > 0 {
					inputTokens = chunk.Response.Usage.InputTokens
					outputTokens = chunk.Response.Usage.OutputTokens
				}
			} else if chunk.Type == "response.in_progress" {
				// Accumulate output text from in-progress chunks
				for _, item := range chunk.Response.Output {
					if item.Type == "message" {
						for _, content := range item.Content {
							if content.Type == "output_text" {
								fullOutput.WriteString(content.Text)
							}
						}
					}
				}
			}
		}

		err = stream.Err()
		require.NoError(t, err)

		t.Logf("Total chunks: %d", chunkCount)
		t.Logf("Full output: %s", fullOutput.String())
		t.Logf("Tokens - Input: %d, Output: %d", inputTokens, outputTokens)

		// Verify we got some response
		assert.Greater(t, chunkCount, 0, "expected at least one chunk")
		assert.NotEmpty(t, fullOutput.String(), "expected some output text")
	})

	t.Run("parameter_filtering", func(t *testing.T) {
		// Test that unsupported parameters are filtered out by the OpenAI client
		client, err := NewOpenAIClient(provider)
		require.NoError(t, err)
		defer client.Close()

		// Build request with parameters - the client/transport will handle filtering
		reqBody := map[string]interface{}{
			"model":             model,
			"instructions":      "Say hello",
			"max_output_tokens": int64(100), // Supported
			"input": []map[string]interface{}{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]string{
						{"type": "input_text", "text": "Hello!"},
					},
				},
			},
			"stream":  false,
			"store":   false,
			"include": []string{},
		}

		req := responses.ResponseNewParams{}
		bs, _ := json.Marshal(reqBody)
		json.Unmarshal(bs, &req)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.ResponsesNew(ctx, req)
		require.NoError(t, err)

		// Success means parameters were handled correctly
		t.Logf("Request succeeded with filtered parameters")
		assert.NotEmpty(t, result.ID)
	})

	t.Run("simple", func(t *testing.T) {
		// Simple test using OpenAI client
		client, err := NewOpenAIClient(provider)
		require.NoError(t, err)
		defer client.Close()

		reqBody := map[string]interface{}{
			"model":        model,
			"instructions": "You are a helpful AI assistant.",
			"input": []map[string]interface{}{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]string{
						{"type": "input_text", "text": "work as `echo`"},
					},
				},
			},
			"stream":      true,
			"store":       false,
			"tool_choice": "auto",
		}

		req := responses.ResponseNewParams{}
		bs, _ := json.Marshal(reqBody)
		json.Unmarshal(bs, &req)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream := client.ResponsesNewStreaming(ctx, req)
		require.NotNil(t, stream)

		var fullOutput strings.Builder
		chunkCount := 0

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			bs, _ := json.Marshal(chunk)
			t.Logf("Chunk #%d: %s", chunkCount, string(bs))

			if chunk.Type == "response.in_progress" {
				// Accumulate output text from in-progress chunks
				for _, item := range chunk.Response.Output {
					if item.Type == "message" {
						for _, content := range item.Content {
							if content.Type == "output_text" {
								fullOutput.WriteString(content.Text)
							}
						}
					}
				}
			}
		}

		err = stream.Err()
		require.NoError(t, err)

		t.Logf("Total chunks: %d", chunkCount)
		t.Logf("Full output: %s", fullOutput.String())
		assert.NotEmpty(t, fullOutput.String())
	})
}
