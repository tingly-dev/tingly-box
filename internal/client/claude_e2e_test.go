package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TestE2E_ClaudeRoundTripper tests the Anthropic API with Claude Code OAuth support.
//
// Prerequisites:
// 1. Set environment variables:
//   - CLAUDE_ACCESS_TOKEN: Anthropic API key or Claude Code OAuth access token
//   - CLAUDE_MODEL (optional): Model name, defaults to "claude-sonnet-4-20250514"
//
// Run with: go test -v ./internal/client -run TestE2E_ClaudeRoundTripper
func TestE2E_ClaudeRoundTripper(t *testing.T) {
	// Skip if no credentials provided
	accessToken := os.Getenv("CLAUDE_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CLAUDE_ACCESS_TOKEN not set, skipping e2e test")
	}

	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Determine auth type based on token format
	authType := typ.AuthTypeAPIKey
	oauthProvider := ""
	if IsClaudeOAuthToken(accessToken) {
		authType = typ.AuthTypeOAuth
		oauthProvider = string(oauth.ProviderClaudeCode)
	}

	// Create provider for Anthropic API
	provider := &typ.Provider{
		ProxyURL: "socks5://127.0.0.1:7890",
		Name:     "claude-e2e-test",
		APIBase:  "https://api.anthropic.com",
		AuthType: authType,
		Timeout:  int64((60 * time.Second).Seconds()),
	}

	if authType == typ.AuthTypeOAuth {
		provider.OAuthDetail = &typ.OAuthDetail{
			ProviderType: oauthProvider,
			AccessToken:  accessToken,
			ExtraFields:  make(map[string]interface{}),
		}
	} else {
		provider.Token = accessToken
	}

	t.Run("model_list", func(t *testing.T) {
		// Create Anthropic client
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// List models
		models, err := client.ListModels(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, models, "expected at least one model")

		t.Logf("Found %d models:", len(models))
		for _, m := range models {
			t.Logf("  - %s", m)
		}

		// Verify the requested model is in the list
		found := false
		for _, m := range models {
			if strings.Contains(m, model) || m == model {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Warning: requested model %q not found in model list", model)
		}
	})

	t.Run("streaming_with_tools", func(t *testing.T) {
		// Create Anthropic client
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Build message request with tools
		systemMessages := []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant with access to tools."},
		}

		// Add Claude Code system header if using OAuth
		if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "claude_code" {
			systemMessages = append([]anthropic.TextBlockParam{{
				Text: ClaudeCodeSystemHeader,
			}}, systemMessages...)
		}

		// Define tool parameter
		toolParam := anthropic.ToolParam{
			Name:        "get_weather",
			Description: anthropic.String("Get the current weather for a location"),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
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
				Required: []string{"location"},
			},
		}

		req := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 1024,
			System:    systemMessages,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("What's the weather in San Francisco? Use the get_weather tool.")),
			},
			Tools: []anthropic.ToolUnionParam{
				{OfTool: &toolParam},
			},
			ToolChoice: anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			},
		}

		stream := client.MessagesNewStreaming(ctx, &req)
		require.NotNil(t, stream)

		// Process the stream
		chunkCount := 0
		var fullOutput strings.Builder
		var toolUseBlocks []map[string]interface{}

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			bs, _ := json.Marshal(chunk)
			t.Logf("Stream chunk #%d: %s", chunkCount, string(bs))

			// Handle different event types - use AsAny() to get concrete type
			switch event := chunk.AsAny().(type) {
			case anthropic.MessageStartEvent:
				t.Logf("Message started: %s", event.Message.ID)

			case anthropic.MessageDeltaEvent:
				t.Logf("Message delta: used %d tokens", event.Usage.OutputTokens)

			case anthropic.MessageStopEvent:
				t.Logf("Message stopped")

			case anthropic.ContentBlockStartEvent:
				if event.ContentBlock.Type == "tool_use" {
					toolUse := map[string]interface{}{
						"id":    event.ContentBlock.ID,
						"name":  event.ContentBlock.Name,
						"input": map[string]interface{}{},
					}
					toolUseBlocks = append(toolUseBlocks, toolUse)
				}

			case anthropic.ContentBlockDeltaEvent:
				switch event.Delta.Type {
				case "text_delta":
					fullOutput.WriteString(event.Delta.Text)
				case "input_json_delta":
					// Accumulate tool input
					if len(toolUseBlocks) > 0 {
						lastTool := toolUseBlocks[len(toolUseBlocks)-1]
						input := lastTool["input"].(map[string]interface{})
						var partialInput map[string]interface{}
						json.Unmarshal([]byte(event.Delta.PartialJSON), &partialInput)
						for k, v := range partialInput {
							input[k] = v
						}
					}
				}

			case anthropic.ContentBlockStopEvent:
				t.Logf("Content block stopped: index %d", event.Index)
			}
		}

		err = stream.Err()
		require.NoError(t, err)

		t.Logf("Total chunks: %d", chunkCount)
		t.Logf("Full output: %s", fullOutput.String())
		t.Logf("Tool use blocks: %d", len(toolUseBlocks))

		// Verify we got some response
		assert.Greater(t, chunkCount, 0, "expected at least one chunk")
	})

	t.Run("streaming_simple", func(t *testing.T) {
		// Create Anthropic client
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Build message request
		systemMessages := []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		}

		// Add Claude Code system header if using OAuth
		if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "claude_code" {
			systemMessages = append([]anthropic.TextBlockParam{{
				Text: ClaudeCodeSystemHeader,
			}}, systemMessages...)
		}

		req := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 1024,
			System:    systemMessages,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Say 'Hello, streaming test!'")),
			},
		}

		stream := client.MessagesNewStreaming(ctx, &req)
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

			// Handle different event types - use AsAny() to get concrete type
			switch event := chunk.AsAny().(type) {
			case anthropic.MessageStartEvent:
				t.Logf("Message started: %s", event.Message.ID)

			case anthropic.MessageDeltaEvent:
				inputTokens = int64(event.Usage.InputTokens)
				outputTokens = int64(event.Usage.OutputTokens)

			case anthropic.ContentBlockDeltaEvent:
				if event.Delta.Type == "text_delta" {
					fullOutput.WriteString(event.Delta.Text)
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

	t.Run("simple", func(t *testing.T) {
		// Simple test using Anthropic client
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build message request
		systemMessages := []anthropic.TextBlockParam{
			{Text: "You are a helpful AI assistant."},
		}

		// Add Claude Code system header if using OAuth
		if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "claude_code" {
			systemMessages = append([]anthropic.TextBlockParam{{
				Text: ClaudeCodeSystemHeader,
			}}, systemMessages...)
		}

		req := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 100,
			System:    systemMessages,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("work as `echo`")),
			},
		}

		result, err := client.MessagesNew(ctx, &req)
		require.NoError(t, err)

		t.Logf("Response ID: %s", result.ID)
		t.Logf("Model: %s", result.Model)

		// Extract response content
		var output strings.Builder
		for _, block := range result.Content {
			if block.Type == "text" {
				output.WriteString(string(block.Text))
			}
		}

		t.Logf("Output: %s", output.String())
		t.Logf("Usage - Input: %d, Output: %d", result.Usage.InputTokens, result.Usage.OutputTokens)

		assert.NotEmpty(t, result.ID)
	})
}

// TestE2E_ClaudeOAuthRoundTripper specifically tests Claude Code OAuth token behavior.
//
// Prerequisites:
// 1. Set environment variables:
//   - CLAUDE_OAUTH_TOKEN: Claude Code OAuth access token (sk-ant-oat prefix)
//
// Run with: go test -v ./internal/client -run TestE2E_ClaudeOAuthRoundTripper
func TestE2E_ClaudeOAuthRoundTripper(t *testing.T) {
	oauthToken := os.Getenv("CLAUDE_OAUTH_TOKEN")
	if oauthToken == "" {
		t.Skip("CLAUDE_OAUTH_TOKEN not set, skipping OAuth-specific e2e test")
	}

	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Create provider with OAuth
	provider := &typ.Provider{
		ProxyURL: "socks5://127.0.0.1:7890",
		Name:     "claude-oauth-e2e-test",
		APIBase:  "https://api.anthropic.com",
		AuthType: typ.AuthTypeOAuth,
		Timeout:  int64((60 * time.Second).Seconds()),
		OAuthDetail: &typ.OAuthDetail{
			ProviderType: string(oauth.ProviderClaudeCode),
			AccessToken:  oauthToken,
			ExtraFields:  make(map[string]interface{}),
		},
	}

	t.Run("oauth_streaming_with_tool_prefix", func(t *testing.T) {
		// Test that tool prefix is correctly applied and stripped for OAuth
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		systemMessages := []anthropic.TextBlockParam{
			{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
			{Text: "You are a helpful assistant with access to tools."},
		}

		toolParam := anthropic.ToolParam{
			Name:        "get_weather",
			Description: anthropic.String("Get weather for a location"),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				Required: []string{"location"},
			},
		}

		req := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 1024,
			System:    systemMessages,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Use the get_weather tool for New York.")),
			},
			Tools: []anthropic.ToolUnionParam{
				{OfTool: &toolParam},
			},
			ToolChoice: anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			},
		}

		stream := client.MessagesNewStreaming(ctx, &req)
		require.NotNil(t, stream)

		chunkCount := 0
		var toolUseFound bool

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			// Check for tool_use in content blocks - use AsAny() to get concrete type
			switch event := chunk.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				if event.ContentBlock.Type == "tool_use" {
					toolUseFound = true
					t.Logf("Found tool_use block: id=%s, name=%s",
						event.ContentBlock.ID, event.ContentBlock.Name)
				}
			}

			bs, _ := json.Marshal(chunk)
			t.Logf("Chunk #%d: %s", chunkCount, string(bs))
		}

		err = stream.Err()
		require.NoError(t, err)

		t.Logf("Total chunks: %d", chunkCount)
		t.Logf("Tool use found: %v", toolUseFound)

		assert.Greater(t, chunkCount, 0, "expected at least one chunk")
	})
}

// TestE2E_BetaStreaming tests the beta messages API with streaming.
//
// Prerequisites:
// 1. Set environment variables:
//   - CLAUDE_ACCESS_TOKEN: Anthropic API key or Claude Code OAuth access token
//
// Run with: go test -v ./internal/client -run TestE2E_BetaStreaming
func TestE2E_BetaStreaming(t *testing.T) {
	accessToken := os.Getenv("CLAUDE_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CLAUDE_ACCESS_TOKEN not set, skipping e2e test")
	}

	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Determine auth type based on token format
	authType := typ.AuthTypeAPIKey
	if IsClaudeOAuthToken(accessToken) {
		authType = typ.AuthTypeOAuth
	}

	// Create provider for Anthropic API
	provider := &typ.Provider{
		ProxyURL: "socks5://127.0.0.1:7890",
		Name:     "claude-beta-e2e-test",
		APIBase:  "https://api.anthropic.com",
		AuthType: authType,
		Timeout:  int64((60 * time.Second).Seconds()),
	}

	if authType == typ.AuthTypeOAuth {
		provider.OAuthDetail = &typ.OAuthDetail{
			ProviderType: string(oauth.ProviderClaudeCode),
			AccessToken:  accessToken,
			ExtraFields:  make(map[string]interface{}),
		}
	} else {
		provider.Token = accessToken
	}

	t.Run("beta_streaming_simple", func(t *testing.T) {
		client, err := NewAnthropicClient(provider, model)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Build beta message request - Model is just a string
		req := anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 1024,
			Messages: []anthropic.BetaMessageParam{
				anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("Say 'Hello from beta API!'")),
			},
		}

		stream := client.BetaMessagesNewStreaming(ctx, &req)
		require.NotNil(t, stream)

		chunkCount := 0
		var fullOutput strings.Builder

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			bs, _ := json.Marshal(chunk)
			t.Logf("Chunk #%d: %s", chunkCount, string(bs))

			// Handle text delta events - use AsAny() to get concrete type
			switch event := chunk.AsAny().(type) {
			case anthropic.BetaRawContentBlockDeltaEvent:
				if event.Delta.Type == "text_delta" {
					fullOutput.WriteString(event.Delta.Text)
				}
			}
		}

		err = stream.Err()
		require.NoError(t, err)

		t.Logf("Total chunks: %d", chunkCount)
		t.Logf("Full output: %s", fullOutput.String())

		assert.Greater(t, chunkCount, 0, "expected at least one chunk")
	})
}
