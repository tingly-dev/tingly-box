package client

import (
	"context"
	"encoding/json"
	"os"
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
		APIBase:  protocol.ChatGPTBackendAPIBase,
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

		result, err := client.ResponsesNew(ctx, req)
		require.NoError(t, err)

		t.Logf("Response ID: %s", result.ID)
		t.Logf("Response status: %s", result.Status)

		// Verify response structure
		assert.NotEmpty(t, result.ID)
		assert.Equal(t, "completed", result.Status)

		// Check output
		assert.NotEmpty(t, result.Output)
		if len(result.Output) > 0 {
			outputItem := result.Output[0]
			t.Logf("Output type: %s", outputItem.Type)
			assert.Equal(t, "message", outputItem.Type)
			assert.NotEmpty(t, outputItem.Content)

			// Log content for debugging
			for i, content := range outputItem.Content {
				t.Logf("Content[%d]: type=%s", i, content.Type)
				if content.Type == "output_text" || content.Type == "text" {
					bs, _ := json.MarshalIndent(content, "", "\t")
					t.Logf("Data: %s\n", string(bs))
				}
			}
		}

		// Check usage
		assert.NotNil(t, result.Usage)
		t.Logf("Tokens - Input: %d, Output: %d, Total: %d",
			result.Usage.InputTokens,
			result.Usage.OutputTokens,
			result.Usage.TotalTokens)
	})
	//
	//t.Run("streaming_simple", func(t *testing.T) {
	//	// Create OpenAI client which already has the proper transport configured
	//	client, err := NewOpenAIClient(provider)
	//	require.NoError(t, err)
	//	defer client.Close()
	//
	//	// Build request using OpenAI Responses API
	//	req := responses.ResponseNewParams{
	//		Model:        openai.F(model),
	//		Instructions: openai.F("You are a helpful assistant."),
	//		Input: openai.F([]responses.ResponseNewParamsInputUnion{
	//			responses.InputMessage{
	//				Type: openai.F(responses.InputTypeMessage),
	//				Role: openai.F(responses.InputMessageRoleUser),
	//				Content: openai.F([]responses.InputMessageContentParamUnion{
	//					responses.InputText{
	//						Type: openai.F(responses.InputTextTypeInputText),
	//						Text: openai.F("Say 'Hello, streaming test!' and call get_weather for New York."),
	//					},
	//				}),
	//			},
	//		}),
	//		Tools: openai.F([]responses.ToolParam{
	//			responses.ToolFunction{
	//				Type: openai.F(responses.ToolTypeFunction),
	//				Function: openai.F(responses.FunctionParam{
	//					Name:        openai.F("get_weather"),
	//					Description: openai.F("Get weather for a location"),
	//					Parameters: openai.F(responses.FunctionParameters{
	//						Type: openai.F(responses.FunctionParametersTypeObject),
	//						Properties: openai.F(map[string]responses.FunctionParametersProperty{
	//							"location": responses.FunctionParametersString{
	//								Type:        openai.F(responses.FunctionParametersPropertyTypeString),
	//								Description: openai.F("City name"),
	//							},
	//						}),
	//						Required: openai.F([]string{"location"}),
	//					}),
	//				}),
	//			},
	//		}),
	//		ToolChoice: openai.F(responses.ToolChoiceAuto),
	//		Stream:     openai.F(true),
	//		Store:      openai.F(false),
	//		Include:    openai.F([]string{}),
	//	}
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	//	defer cancel()
	//
	//	stream := client.ResponsesNewStreaming(ctx, req)
	//	require.NotNil(t, stream)
	//
	//	// Read streaming response
	//	chunkCount := 0
	//	var fullOutput strings.Builder
	//	var inputTokens, outputTokens int
	//
	//	for stream.Next() {
	//		chunk := stream.Current()
	//		chunkCount++
	//
	//		if chunkCount <= 3 || chunk.Type == "response.done" {
	//			t.Logf("Chunk #%d: type=%s", chunkCount, chunk.Type)
	//		}
	//
	//		// Extract response data based on type
	//		switch r := chunk.Response.(type) {
	//		case responses.ResponseStreamEventResponseDone:
	//			// Response completed
	//			t.Logf("Response completed")
	//			if r.Usage != nil {
	//				inputTokens = r.Usage.InputTokens
	//				outputTokens = r.Usage.OutputTokens
	//			}
	//		case responses.ResponseStreamEventResponseInProgress:
	//			// Accumulate output text from in-progress chunks
	//			for _, item := range r.Output {
	//				if item.Type == "message" {
	//					for _, content := range item.Content {
	//						if outputText, ok := content.(responses.OutputText); ok && outputText.Type == "output_text" {
	//							fullOutput.WriteString(outputText.Text)
	//						}
	//					}
	//				}
	//			}
	//		}
	//	}
	//
	//	err = stream.Err()
	//	require.NoError(t, err)
	//
	//	t.Logf("Total chunks: %d", chunkCount)
	//	t.Logf("Full output: %s", fullOutput.String())
	//	t.Logf("Tokens - Input: %d, Output: %d", inputTokens, outputTokens)
	//
	//	// Verify we got some response
	//	assert.Greater(t, chunkCount, 0, "expected at least one chunk")
	//	assert.NotEmpty(t, fullOutput.String(), "expected some output text")
	//})
	//
	//t.Run("parameter_filtering", func(t *testing.T) {
	//	// Test that unsupported parameters are filtered out by the OpenAI client
	//	client, err := NewOpenAIClient(provider)
	//	require.NoError(t, err)
	//	defer client.Close()
	//
	//	// Build request with parameters - the client/transport will handle filtering
	//	req := responses.ResponseNewParams{
	//		Model:           openai.F(model),
	//		Instructions:    openai.F("Say hello"),
	//		MaxOutputTokens: openai.F(int64(100)), // Supported
	//		Input: openai.F([]responses.ResponseNewParamsInputUnion{
	//			responses.InputMessage{
	//				Type: openai.F(responses.InputTypeMessage),
	//				Role: openai.F(responses.InputMessageRoleUser),
	//				Content: openai.F([]responses.InputMessageContentParamUnion{
	//					responses.InputText{
	//						Type: openai.F(responses.InputTextTypeInputText),
	//						Text: openai.F("Hello!"),
	//					},
	//				}),
	//			},
	//		}),
	//		Stream:  openai.F(false),
	//		Store:   openai.F(false),
	//		Include: openai.F([]string{}),
	//	}
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//	defer cancel()
	//
	//	result, err := client.ResponsesNew(ctx, req)
	//	require.NoError(t, err)
	//
	//	// Success means parameters were handled correctly
	//	t.Logf("Request succeeded with filtered parameters")
	//	assert.NotEmpty(t, result.ID)
	//})
	//
	//t.Run("simple", func(t *testing.T) {
	//	// Simple test using OpenAI client
	//	client, err := NewOpenAIClient(provider)
	//	require.NoError(t, err)
	//	defer client.Close()
	//
	//	req := responses.ResponseNewParams{
	//		Model:        openai.F(model),
	//		Instructions: openai.F("You are a helpful AI assistant."),
	//		Input: openai.F([]responses.ResponseNewParamsInputUnion{
	//			responses.InputMessage{
	//				Type: openai.F(responses.InputTypeMessage),
	//				Role: openai.F(responses.InputMessageRoleUser),
	//				Content: openai.F([]responses.InputMessageContentParamUnion{
	//					responses.InputText{
	//						Type: openai.F(responses.InputTextTypeInputText),
	//						Text: openai.F("work as `echo`"),
	//					},
	//				}),
	//			},
	//		}),
	//		Stream:     openai.F(true),
	//		Store:      openai.F(false),
	//		ToolChoice: openai.F(responses.ToolChoiceAuto),
	//	}
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//	defer cancel()
	//
	//	stream := client.ResponsesNewStreaming(ctx, req)
	//	require.NotNil(t, stream)
	//
	//	var fullOutput strings.Builder
	//
	//	for stream.Next() {
	//		chunk := stream.Current()
	//
	//		switch r := chunk.Response.(type) {
	//		case responses.ResponseStreamEventResponseInProgress:
	//			for _, item := range r.Output {
	//				if item.Type == "message" {
	//					for _, content := range item.Content {
	//						if outputText, ok := content.(responses.OutputText); ok && outputText.Type == "output_text" {
	//							fullOutput.WriteString(outputText.Text)
	//						}
	//					}
	//				}
	//			}
	//		}
	//	}
	//
	//	err = stream.Err()
	//	require.NoError(t, err)
	//
	//	t.Logf("Full output: %s", fullOutput.String())
	//	assert.NotEmpty(t, fullOutput.String())
	//})
}
