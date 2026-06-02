package stream

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/request"
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
	stream := client.Beta.Messages.NewStreaming(context.Background(), anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(100),
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("What's the weather like in London?")),
		},
		Tools: request.ConvertOpenAIToAnthropicTools([]openai.ChatCompletionToolUnionParam{NewExampleTool()}),
	})

	// Create a gin context for the response
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Run the handler
	_, _, err := AnthropicToOpenAIStream(c, nil, stream, model, false)
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

// buildMessageStopJSON builds a minimal message_stop event for tests.
func buildMessageStopJSON() string {
	return `{"type":"message_stop"}`
}

// TestAnthropicToOpenAIStream_RealFormatUsage verifies that when the stream follows
// the real Anthropic SSE protocol (input_tokens in message_start, output_tokens in
// message_delta), both the returned counts and the SSE usage chunk are correct.
func TestAnthropicToOpenAIStream_RealFormatUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 35, 0),  // input_tokens=35, cache=0
		buildAnthropicOutputOnlyDeltaJSON(t, 18),   // output_tokens=18 only
		buildMessageStopJSON(),
	}
	decoder := newFakeAnthropicDecoder(events)
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](decoder, nil)

	inputTokens, outputTokens, err := AnthropicToOpenAIStream(c, nil, stream, "claude-3-5-sonnet", false)
	require.NoError(t, err)

	// Returned counts must be correct
	assert.Equal(t, 35, inputTokens, "input_tokens must come from message_start")
	assert.Equal(t, 18, outputTokens)

	// The SSE body must contain a usage chunk with correct prompt_tokens.
	// The OpenAI SDK always serializes Usage even when zero, so there may be an early
	// zero-usage chunk from message_start. We track the LAST non-zero usage seen.
	body := w.Body.String()
	var lastPromptTokens, lastCompletionTokens float64
	foundNonZeroUsage := false
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == " [DONE]" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if u, ok := chunk["usage"].(map[string]interface{}); ok {
			if pt, ok := u["prompt_tokens"].(float64); ok && pt > 0 {
				lastPromptTokens = pt
				lastCompletionTokens, _ = u["completion_tokens"].(float64)
				foundNonZeroUsage = true
			}
		}
	}
	assert.True(t, foundNonZeroUsage, "stream must emit a non-zero usage chunk")
	assert.EqualValues(t, 35, lastPromptTokens, "final SSE usage chunk prompt_tokens must be 35")
	assert.EqualValues(t, 18, lastCompletionTokens)
}

// TestAnthropicToOpenAIStream_NonStandardDelta verifies a non-standard provider that
// delivers input_tokens only inside message_delta (not message_start) still works.
func TestAnthropicToOpenAIStream_NonStandardDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	events := []string{
		// message_start with no usage (non-standard)
		`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"custom","usage":{"input_tokens":0,"output_tokens":0}}}`,
		// message_delta with both input and output (non-standard but backward-compat)
		buildAnthropicMessageDeltaJSON(t, 40, 20, 0),
		buildMessageStopJSON(),
	}
	decoder := newFakeAnthropicDecoder(events)
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](decoder, nil)

	inputTokens, outputTokens, err := AnthropicToOpenAIStream(c, nil, stream, "custom-model", false)
	require.NoError(t, err)

	assert.Equal(t, 40, inputTokens)
	assert.Equal(t, 20, outputTokens)
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

	sendOpenAIStreamChunkForce(c, chunk)

	body := w.Body.String()
	assert.Contains(t, body, "data: ")
	assert.Contains(t, body, `"id":"test-id"`)
	assert.Contains(t, body, `"object":"chat.completion.chunk"`)
}
