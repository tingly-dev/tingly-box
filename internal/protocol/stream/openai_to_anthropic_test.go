package stream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOpenAIDecoder struct{}

func (f *fakeOpenAIDecoder) Event() openaistream.Event { return openaistream.Event{} }
func (f *fakeOpenAIDecoder) Next() bool                { return false }
func (f *fakeOpenAIDecoder) Close() error              { return nil }
func (f *fakeOpenAIDecoder) Err() error                { return nil }

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return make(chan bool) // never closed — client stays connected for the duration of the test
}

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
			NewExampleTool(),
		},
	})

	// Create a gin context for the response
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// the handler
	usage, err := HandleOpenAIToAnthropicStreamResponse(c, nil, stream, model)
	require.NoError(t, err)

	// Verify usage stats
	assert.Equal(t, 0, usage.InputTokens)
	assert.Equal(t, 0, usage.OutputTokens)

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
	assert.Contains(t, body, "event:message_start")
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, `"type":"message_start"`)
}

// TestHandleOpenAIToAnthropicStreamResponseWithThinking tests OpenAI to Anthropic
// stream conversion with reasoning_content/thinking block support
func TestHandleOpenAIToAnthropicStreamResponseWithThinking(t *testing.T) {
	// Set your API key and base URL before running the test
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := "" // Optional: custom base URL
	model := ""   // e.g., "o1-mini" or "o1-preview" for models with reasoning

	if apiKey == "" || model == "" {
		t.Skip("Skipping test: OPENAI_API_KEY and model must be set")
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
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a helpful assistant. Think step by step."),
			openai.UserMessage("What is 15 * 23? Show your reasoning."),
		},
	})

	// Create a gin context for the response
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Run the handler
	usage, err := HandleOpenAIToAnthropicStreamResponse(c, nil, stream, model)
	require.NoError(t, err)

	t.Logf("Usage stats: input=%d, output=%d", usage.InputTokens, usage.OutputTokens)

	// Verify the response
	body := w.Body.String()
	t.Logf("Response body:\n%s", body)

	// Parse SSE events
	events := parseSSEEvents(body)

	// Verify message_start event
	msgStart, ok := events[eventTypeMessageStart]
	require.True(t, ok, "Should have message_start event")
	assert.Equal(t, "message", msgStart["message"].(map[string]interface{})["type"])

	// Check for thinking block if the model supports reasoning
	foundThinkingBlock := false
	foundContentBlockStart := false
	foundThinkingDelta := false
	foundTextDelta := false

	for eventType, eventData := range events {
		switch eventType {
		case eventTypeContentBlockStart:
			blockData := eventData["content_block"].(map[string]interface{})
			blockType := blockData["type"]
			if blockType == blockTypeThinking {
				foundThinkingBlock = true
				t.Logf("Found thinking block: %+v", blockData)
			} else if blockType == blockTypeText {
				foundContentBlockStart = true
			}
		case eventTypeContentBlockDelta:
			delta := eventData["delta"].(map[string]interface{})
			deltaType := delta["type"]
			if deltaType == deltaTypeThinkingDelta {
				foundThinkingDelta = true
				thinkingContent := delta["thinking"]
				t.Logf("Found thinking delta: %s", thinkingContent)
			} else if deltaType == deltaTypeTextDelta {
				foundTextDelta = true
				textContent := delta["text"]
				if textContent != "" {
					t.Logf("Found text delta: %s", textContent)
				}
			}
		}
	}

	// Verify message_delta and message_stop events
	msgDelta, ok := events[eventTypeMessageDelta]
	require.True(t, ok, "Should have message_delta event")
	assert.Contains(t, msgDelta["delta"].(map[string]interface{}), "stop_reason")

	msgStop, ok := events[eventTypeMessageStop]
	require.True(t, ok, "Should have message_stop event")
	assert.Equal(t, "message", msgStop["message"].(map[string]interface{})["type"])

	// Note: thinking block depends on the model support
	// Some models may not return reasoning_content
	if foundThinkingBlock || foundThinkingDelta {
		t.Log("Thinking block detected - model supports reasoning_content")
	} else {
		t.Log("No thinking block detected - model may not support reasoning_content")
	}

	// Should have content block (text)
	assert.True(t, foundContentBlockStart || foundTextDelta, "Should have text content")

	// Verify SSE headers
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
}

func TestHandleOpenAIToAnthropicStreamResponse_ClientCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	c.Request = req.WithContext(ctx)

	stream := openaistream.NewStream[openai.ChatCompletionChunk](&fakeOpenAIDecoder{}, nil)

	usage, err := HandleOpenAIToAnthropicStreamResponse(c, nil, stream, "test-model")
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.InputTokens)
	assert.Equal(t, 0, usage.OutputTokens)
}

// fakeResponsesDecoder replays a fixed sequence of JSON events as a Responses API stream.
type fakeResponsesDecoder struct {
	events  []string // raw JSON payloads to emit
	current int      // index of the event returned by Event()
	next    int      // index of the next event to advance to
}

func newFakeResponsesDecoder(events []string) *fakeResponsesDecoder {
	return &fakeResponsesDecoder{events: events, current: -1}
}

func (f *fakeResponsesDecoder) Next() bool {
	if f.next >= len(f.events) {
		return false
	}
	f.current = f.next
	f.next++
	return true
}

func (f *fakeResponsesDecoder) Event() openaistream.Event {
	return openaistream.Event{Data: []byte(f.events[f.current])}
}

func (f *fakeResponsesDecoder) Close() error { return nil }
func (f *fakeResponsesDecoder) Err() error   { return nil }

// buildResponsesCompletedJSON builds a minimal response.completed SSE payload
// with the given token counts.
func buildResponsesCompletedJSON(t *testing.T, inputTokens, outputTokens, cacheTokens, reasoningTokens int64) string {
	t.Helper()
	payload := map[string]interface{}{
		"type":            "response.completed",
		"sequence_number": 1,
		"response": map[string]interface{}{
			"id":         "resp_test",
			"object":     "response",
			"created_at": 1000,
			"status":     "completed",
			"output": []interface{}{
				map[string]interface{}{
					"id":     "msg_1",
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []interface{}{
						map[string]interface{}{
							"type": "output_text",
							"text": "Hello",
						},
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
				"input_tokens_details": map[string]interface{}{
					"cached_tokens": cacheTokens,
				},
				"output_tokens_details": map[string]interface{}{
					"reasoning_tokens": reasoningTokens,
				},
			},
			"model": "gpt-4o",
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

// TestHandleResponsesToAnthropicV1Stream_UsageTokens verifies that input, output,
// cache, and reasoning tokens from response.completed are captured and returned.
func TestHandleResponsesToAnthropicV1Stream_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 50, 30, 8, 12),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	usage, err := HandleResponsesToAnthropicV1Stream(c, stream, "gpt-4o")
	require.NoError(t, err)

	// OpenAI Responses API: input=50 total, cached=8 → stored as 50-8=42 (uncached only)
	assert.Equal(t, 42, usage.InputTokens)
	assert.Equal(t, 30, usage.OutputTokens)
	assert.Equal(t, 8, usage.CacheInputTokens)
	assert.Equal(t, 12, usage.ReasoningTokens)
}

// TestHandleResponsesToAnthropicV1Stream_MessageDeltaCacheTokens verifies that
// cache_read_input_tokens is emitted in the message_delta SSE event.
func TestHandleResponsesToAnthropicV1Stream_MessageDeltaCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 40, 20, 10, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	_, err := HandleResponsesToAnthropicV1Stream(c, stream, "gpt-4o")
	require.NoError(t, err)

	// Find the message_delta event and verify its usage block
	events := parseSSEEvents(w.Body.String())
	msgDelta, ok := events[eventTypeMessageDelta]
	require.True(t, ok, "should have message_delta event")

	usage := msgDelta["usage"].(map[string]interface{})
	assert.Equal(t, float64(20), usage["output_tokens"])
	assert.Equal(t, float64(10), usage["cache_read_input_tokens"])
}

// TestHandleResponsesToAnthropicV1Stream_ZeroCacheTokens verifies that
// cache_read_input_tokens is absent from message_delta when cache tokens are zero.
func TestHandleResponsesToAnthropicV1Stream_ZeroCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 20, 10, 0, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	_, err := HandleResponsesToAnthropicV1Stream(c, stream, "gpt-4o")
	require.NoError(t, err)

	events := parseSSEEvents(w.Body.String())
	msgDelta, ok := events[eventTypeMessageDelta]
	require.True(t, ok, "should have message_delta event")

	usage := msgDelta["usage"].(map[string]interface{})
	assert.NotContains(t, usage, "cache_read_input_tokens")
}

// parseSSEEvents parses SSE response body into a map of events
func parseSSEEvents(body string) map[string]map[string]interface{} {
	events := make(map[string]map[string]interface{})
	lines := strings.Split(body, "\n")

	var currentEventType string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "event:") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" {
				// Parse the JSON data
				var eventData map[string]interface{}
				err := json.Unmarshal([]byte(data), &eventData)
				if err == nil {
					// Store by event type if available, otherwise use a counter
					key := currentEventType
					if key == "" {
						key = "unknown"
					}
					// Store the last event of each type for easy verification
					events[key] = eventData
				}
			}
		} else if line == "" {
			// Reset for next event
			currentEventType = ""
		}
	}

	return events
}
