package stream

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type fakeOpenAIDecoder struct{}

func (f *fakeOpenAIDecoder) Event() openaistream.Event { return openaistream.Event{} }
func (f *fakeOpenAIDecoder) Next() bool                { return false }
func (f *fakeOpenAIDecoder) Close() error              { return nil }
func (f *fakeOpenAIDecoder) Err() error                { return nil }

type failingOpenAIChatStream struct {
	err error
}

func (s *failingOpenAIChatStream) Next() bool { return false }
func (s *failingOpenAIChatStream) Current() openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{}
}
func (s *failingOpenAIChatStream) Err() error { return s.err }

func TestOpenAIChatToAnthropicConvertersPropagateIteratorError(t *testing.T) {
	want := errors.New("upstream iterator failed")
	constructors := map[string]func(OpenAIChatStream) StreamConverter{
		"v1": func(stream OpenAIChatStream) StreamConverter {
			return NewOpenAIChatToAnthropicV1Converter(stream, "model", nil)
		},
		"beta": func(stream OpenAIChatStream) StreamConverter {
			return NewOpenAIChatToAnthropicBetaConverter(stream, "model", nil)
		},
	}

	for name, newConverter := range constructors {
		t.Run(name, func(t *testing.T) {
			event, done, err := newConverter(&failingOpenAIChatStream{err: want}).Next()
			assert.Nil(t, event)
			assert.False(t, done)
			assert.ErrorIs(t, err, want)
		})
	}
}

// openAIChatSliceStream replays fixed chunks, then reports err from Err() once
// exhausted — simulating a provider whose connection teardown is dirty.
type openAIChatSliceStream struct {
	chunks []openai.ChatCompletionChunk
	index  int
	err    error
}

func (s *openAIChatSliceStream) Next() bool {
	if s.index >= len(s.chunks) {
		return false
	}
	s.index++
	return true
}

func (s *openAIChatSliceStream) Current() openai.ChatCompletionChunk {
	return s.chunks[s.index-1]
}

func (s *openAIChatSliceStream) Err() error { return s.err }

func openAIChatChunks(t *testing.T, bodies ...string) []openai.ChatCompletionChunk {
	t.Helper()
	chunks := make([]openai.ChatCompletionChunk, 0, len(bodies))
	for _, body := range bodies {
		var chunk openai.ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(body), &chunk))
		chunks = append(chunks, chunk)
	}
	return chunks
}

// TestOpenAIChatToAnthropicSalvagesDirtyTeardownAfterFinish pins the salvage
// semantics: once finish_reason arrived the turn is semantically complete, so a
// scanner error surfaced while draining to physical EOF (a provider closing
// without a clean shutdown) must not discard the response — the client still
// gets message_delta + message_stop and no error.
func TestOpenAIChatToAnthropicSalvagesDirtyTeardownAfterFinish(t *testing.T) {
	stream := &openAIChatSliceStream{
		chunks: openAIChatChunks(t,
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		),
		err: errors.New("unexpected EOF"),
	}
	converter := NewOpenAIChatToAnthropicV1Converter(stream, "model", nil)

	var types []string
	for {
		event, done, err := converter.Next()
		require.NoError(t, err, "dirty teardown after finish_reason must be salvaged, not surfaced")
		if done {
			break
		}
		typed, ok := AsAnthropicEvent(event)
		require.Truef(t, ok, "event type = %T", event)
		types = append(types, typed.Type)
	}

	require.GreaterOrEqual(t, len(types), 2)
	assert.Equal(t, "message_start", types[0])
	assert.Equal(t, "message_delta", types[len(types)-2])
	assert.Equal(t, "message_stop", types[len(types)-1])
}

// TestOpenAIChatToAnthropicPropagatesMidStreamError pins the complementary
// case: the same teardown error before finish_reason is a genuine truncation
// and must surface as an error instead of fabricating a clean ending.
func TestOpenAIChatToAnthropicPropagatesMidStreamError(t *testing.T) {
	want := errors.New("connection reset by peer")
	stream := &openAIChatSliceStream{
		chunks: openAIChatChunks(t,
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
		),
		err: want,
	}
	converter := NewOpenAIChatToAnthropicV1Converter(stream, "model", nil)

	for {
		event, done, err := converter.Next()
		if err != nil {
			assert.ErrorIs(t, err, want)
			return
		}
		require.False(t, done, "mid-stream teardown error must surface, got clean end")
		_, ok := AsAnthropicEvent(event)
		require.Truef(t, ok, "event type = %T", event)
	}
}

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return make(chan bool) // never closed — client stays connected for the duration of the test
}

// TestOpenAIChatToAnthropicStream tests the OpenAI to Anthropic stream conversion
func TestOpenAIChatToAnthropicStream(t *testing.T) {
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
	usage, err := writeAnthropicSSE(protocol.NewHandleContext(c, model), NewOpenAIChatToAnthropicBetaConverter(stream, model, nil))
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

// TestOpenAIChatToAnthropicStreamWithThinking tests OpenAI to Anthropic
// stream conversion with reasoning_content/thinking block support
func TestOpenAIChatToAnthropicStreamWithThinking(t *testing.T) {
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
	usage, err := writeAnthropicSSE(protocol.NewHandleContext(c, model), NewOpenAIChatToAnthropicBetaConverter(stream, model, nil))
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

// TestResponsesToAnthropicStream_UsageTokens verifies that input, output,
// cache, and reasoning tokens from response.completed are captured and returned.
func TestResponsesToAnthropicStream_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 50, 30, 8, 12),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	hc := protocol.NewHandleContext(c, "gpt-4o")
	usage, err := writeAnthropicSSE(hc, NewOpenAIResponsesToAnthropicConverter(context.Background(), stream, "gpt-4o"))
	require.NoError(t, err)

	// OpenAI Responses API: input=50 total, cached=8 → stored as 50-8=42 (uncached only)
	assert.Equal(t, 42, usage.InputTokens)
	assert.Equal(t, 30, usage.OutputTokens)
	assert.Equal(t, 8, usage.CacheReadTokens)
	assert.Equal(t, 12, usage.ReasoningTokens)
}

// TestResponsesToAnthropicStream_MessageDeltaCacheTokens verifies that
// cache_read_input_tokens is emitted in the message_delta SSE event.
func TestResponsesToAnthropicStream_MessageDeltaCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 40, 20, 10, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	hc := protocol.NewHandleContext(c, "gpt-4o")
	_, err := writeAnthropicSSE(hc, NewOpenAIResponsesToAnthropicConverter(context.Background(), stream, "gpt-4o"))
	require.NoError(t, err)

	// Find the message_delta event and verify its usage block
	events := parseSSEEvents(w.Body.String())
	msgDelta, ok := events[eventTypeMessageDelta]
	require.True(t, ok, "should have message_delta event")

	usage := msgDelta["usage"].(map[string]interface{})
	assert.Equal(t, float64(20), usage["output_tokens"])
	assert.Equal(t, float64(10), usage["cache_read_input_tokens"])
}

// TestResponsesToAnthropicStream_ZeroCacheTokens verifies that
// cache_read_input_tokens is absent from message_delta when cache tokens are zero.
func TestResponsesToAnthropicStream_ZeroCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 20, 10, 0, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	hc := protocol.NewHandleContext(c, "gpt-4o")
	_, err := writeAnthropicSSE(hc, NewOpenAIResponsesToAnthropicConverter(context.Background(), stream, "gpt-4o"))
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
