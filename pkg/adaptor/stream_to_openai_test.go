package adaptor

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleAnthropicToOpenAIStreamResponse tests the Anthropic to OpenAI stream conversion
func TestHandleAnthropicToOpenAIStreamResponse(t *testing.T) {
	// Set your API key and base URL before running the test
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	baseURL := "" // Optional: custom base URL
	model := ""   // e.g., "claude-3-5-haiku-20241022"

	if apiKey == "" || model == "" {
		t.Skip("Skipping test: apiKey and model must be set")
	}

	// Create client
	var client anthropic.Client
	if baseURL != "" {
		client = anthropic.NewClient(
			anthropicOption.WithAPIKey(apiKey),
			anthropicOption.WithBaseURL(baseURL),
		)
	} else {
		client = anthropic.NewClient(anthropicOption.WithAPIKey(apiKey))
	}

	// Create a streaming request
	stream := client.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(100),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("What's the weather like in London?")),
		},
		Tools: ConvertOpenAIToAnthropicTools([]openai.ChatCompletionToolUnionParam{newExampleTool()}),
	})

	// Create a gin context for the response
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Run the handler
	err := HandleAnthropicToOpenAIStreamResponse(c, nil, stream, model)
	require.NoError(t, err)

	// Verify the response
	body := w.Body.String()
	lines := strings.Split(body, "\n")

	t.Logf("Response body:\n%s", body)

	// Check for proper SSE format
	foundDataChunk := false
	foundDone := false
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			foundDataChunk = true
			dataContent := strings.TrimPrefix(line, "data: ")
			if dataContent == "[DONE]" {
				foundDone = true
				continue
			}
			// Verify it's valid JSON
			var chunk map[string]interface{}
			err := json.Unmarshal([]byte(dataContent), &chunk)
			assert.NoError(t, err, "Chunk should be valid JSON")

			// Verify OpenAI format structure
			assert.Contains(t, chunk, "id")
			assert.Contains(t, chunk, "object")
			assert.Equal(t, "chat.completion.chunk", chunk["object"])
			assert.Contains(t, chunk, "created")
			assert.Contains(t, chunk, "model")
			assert.Contains(t, chunk, "choices")
		}
	}

	assert.True(t, foundDataChunk, "Should have at least one data chunk")
	assert.True(t, foundDone, "Should have [DONE] marker")
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
}

// TestSendOpenAIStreamChunk tests the helper function
func TestSendOpenAIStreamChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	chunk := map[string]interface{}{
		"id":      "test-id",
		"object":  "chat.completion.chunk",
		"created": int64(1234567890),
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{"content": "Hello"},
				"finish_reason": nil,
			},
		},
	}

	sendOpenAIStreamChunk(c, chunk)

	body := w.Body.String()
	assert.Contains(t, body, "data: ")
	assert.Contains(t, body, `"id":"test-id"`)
	assert.Contains(t, body, `"object":"chat.completion.chunk"`)
}
