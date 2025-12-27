package adaptor

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleOpenAIToAnthropicStreamResponse tests the OpenAI to Anthropic stream conversion
func TestHandleOpenAIToAnthropicStreamResponse(t *testing.T) {
	// Set your API key and base URL before running the test
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := "" // Optional: custom base URL
	model := ""

	if apiKey == "" || model == "" {
		t.Skip("Skipping test: apiKey and model must be set")
	}

	// Create client
	var client openai.Client
	if baseURL != "" {
		client = openai.NewClient(
			openaiOption.WithAPIKey(apiKey),
			openaiOption.WithBaseURL(baseURL),
		)
	} else {
		client = openai.NewClient(openaiOption.WithAPIKey(apiKey))
	}

	// Create a streaming request
	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Opt[int64](100),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("What's the weather like in London?"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			newExampleTool(),
		},
	})

	// Create a gin context for the response
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// the handler
	err := HandleOpenAIToAnthropicStreamResponse(c, stream, model)
	require.NoError(t, err)

	// Verify the response
	body := w.Body.String()
	lines := strings.Split(body, "\n")

	t.Logf("Response body:\n%s", body)

	// Check for proper SSE format
	foundMessageStart := false
	foundContentBlockDelta := false
	foundMessageStop := false

	currentEvent := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataContent := strings.TrimPrefix(line, "data: ")

			switch currentEvent {
			case "message_start":
				foundMessageStart = true
				var eventData map[string]interface{}
				err := json.Unmarshal([]byte(dataContent), &eventData)
				assert.NoError(t, err, "message_start data should be valid JSON")
				assert.Equal(t, "message_start", eventData["type"])

			case "content_block_delta":
				foundContentBlockDelta = true
				var eventData map[string]interface{}
				err := json.Unmarshal([]byte(dataContent), &eventData)
				assert.NoError(t, err, "content_block_delta data should be valid JSON")
				assert.Equal(t, "content_block_delta", eventData["type"])

			case "message_stop":
				foundMessageStop = true
				var eventData map[string]interface{}
				err := json.Unmarshal([]byte(dataContent), &eventData)
				assert.NoError(t, err, "message_stop data should be valid JSON")
				assert.Equal(t, "message_stop", eventData["type"])
			}
		}
	}

	assert.True(t, foundMessageStart, "Should have message_start event")
	assert.True(t, foundContentBlockDelta, "Should have content_block_delta event")
	assert.True(t, foundMessageStop, "Should have message_stop event")
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
}

// TestSendAnthropicStreamEvent tests the helper function
func TestSendAnthropicStreamEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	eventData := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":      "msg_123",
			"type":    "message",
			"role":    "assistant",
			"content": []interface{}{},
		},
	}

	sendAnthropicStreamEvent(c, "message_start", eventData, w)

	body := w.Body.String()
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "data: ")
	assert.Contains(t, body, `"type":"message_start"`)
}
