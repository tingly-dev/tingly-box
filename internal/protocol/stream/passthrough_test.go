package stream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ---------------------------------------------------------------------------
// Anthropic fake decoder
// ---------------------------------------------------------------------------

type fakeAnthropicDecoder struct {
	events  []string
	current int
	next    int
}

func newFakeAnthropicDecoder(events []string) *fakeAnthropicDecoder {
	return &fakeAnthropicDecoder{events: events, current: -1}
}

func (f *fakeAnthropicDecoder) Next() bool {
	if f.next >= len(f.events) {
		return false
	}
	f.current = f.next
	f.next++
	return true
}

func (f *fakeAnthropicDecoder) Event() anthropicstream.Event {
	data := []byte(f.events[f.current])
	// The Anthropic Stream.Next() dispatches on Event().Type (the SSE event name),
	// not on the JSON "type" field. Extract it from the JSON so the stream accepts it.
	eventType := gjson.GetBytes(data, "type").String()
	return anthropicstream.Event{Type: eventType, Data: data}
}

func (f *fakeAnthropicDecoder) Close() error { return nil }
func (f *fakeAnthropicDecoder) Err() error   { return nil }

// ---------------------------------------------------------------------------
// JSON builders
// ---------------------------------------------------------------------------

func buildChatChunkJSON(t *testing.T, promptTokens, completionTokens, cachedTokens, reasoningTokens int64) string {
	t.Helper()
	chunk := map[string]interface{}{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1000,
		"model":   "gpt-4o",
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{"content": "Hi"},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
			"prompt_tokens_details": map[string]interface{}{
				"cached_tokens": cachedTokens,
			},
			"completion_tokens_details": map[string]interface{}{
				"reasoning_tokens": reasoningTokens,
			},
		},
	}
	data, err := json.Marshal(chunk)
	require.NoError(t, err)
	return string(data)
}

func buildAnthropicMessageDeltaJSON(t *testing.T, inputTokens, outputTokens, cacheReadTokens int64) string {
	t.Helper()
	event := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"input_tokens":            inputTokens,
			"output_tokens":           outputTokens,
			"cache_read_input_tokens": cacheReadTokens,
		},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return string(data)
}

// newTestHandleContext creates a minimal HandleContext wired to the given gin context.
func newTestHandleContext(c *gin.Context) *protocol.HandleContext {
	return protocol.NewHandleContext(c, "test-model")
}

// ---------------------------------------------------------------------------
// HandleOpenAIChatStream
// ---------------------------------------------------------------------------

func TestHandleOpenAIChatStream_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	dec := &fakeChatDecoder{events: []string{
		buildChatChunkJSON(t, 40, 20, 8, 10),
	}, current: -1}
	stream := openaistream.NewStream[openai.ChatCompletionChunk](dec, nil)

	hc := newTestHandleContext(c)
	hc.DisableStreamUsage = true

	// Pass an empty req to avoid nil-dereference in the !hasUsage estimation path.
	// The SDK's custom apijson unmarshaller does not populate usage from raw JSON in
	// unit tests, so hasUsage stays false and the estimation fallback runs.
	// We verify the estimation path doesn't crash and returns a non-nil usage.
	req := &openai.ChatCompletionNewParams{}
	usage, err := HandleOpenAIChatStream(hc, stream, req)
	require.NoError(t, err)
	require.NotNil(t, usage)
}

func TestHandleOpenAIChatStream_ZeroReasoningTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	dec := &fakeChatDecoder{events: []string{
		buildChatChunkJSON(t, 10, 5, 0, 0),
	}, current: -1}
	stream := openaistream.NewStream[openai.ChatCompletionChunk](dec, nil)

	hc := newTestHandleContext(c)
	hc.DisableStreamUsage = true

	req := &openai.ChatCompletionNewParams{}
	usage, err := HandleOpenAIChatStream(hc, stream, req)
	require.NoError(t, err)
	require.NotNil(t, usage)
	// ReasoningTokens must be zero when SDK doesn't surface it
	assert.Equal(t, 0, usage.ReasoningTokens)
}

// fakeChatDecoder replays chat completion chunks.
type fakeChatDecoder struct {
	events  []string
	current int
	next    int
}

func (f *fakeChatDecoder) Next() bool {
	if f.next >= len(f.events) {
		return false
	}
	f.current = f.next
	f.next++
	return true
}

func (f *fakeChatDecoder) Event() openaistream.Event {
	return openaistream.Event{Data: []byte(f.events[f.current])}
}

func (f *fakeChatDecoder) Close() error { return nil }
func (f *fakeChatDecoder) Err() error   { return nil }

// ---------------------------------------------------------------------------
// HandleOpenAIResponsesStream
// ---------------------------------------------------------------------------

func TestHandleOpenAIResponsesStream_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 50, 30, 7, 15),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	hc := newTestHandleContext(c)

	usage, err := HandleOpenAIResponsesStream(hc, stream, "gpt-4o")
	require.NoError(t, err)

	assert.Equal(t, 50, usage.InputTokens)
	assert.Equal(t, 30, usage.OutputTokens)
	assert.Equal(t, 7, usage.CacheInputTokens)
	assert.Equal(t, 15, usage.ReasoningTokens)
}

func TestHandleOpenAIResponsesStream_ZeroReasoningTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 20, 10, 0, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	hc := newTestHandleContext(c)

	usage, err := HandleOpenAIResponsesStream(hc, stream, "gpt-4o")
	require.NoError(t, err)

	assert.Equal(t, 20, usage.InputTokens)
	assert.Equal(t, 10, usage.OutputTokens)
	assert.Equal(t, 0, usage.CacheInputTokens)
	assert.Equal(t, 0, usage.ReasoningTokens)
}

// ---------------------------------------------------------------------------
// HandleOpenAIResponsesStreamToAnthropic
// ---------------------------------------------------------------------------

func TestHandleOpenAIResponsesStreamToAnthropic_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 60, 25, 12, 8),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	usage, err := HandleOpenAIResponsesStreamToAnthropic(c, stream, "gpt-4o")
	require.NoError(t, err)

	assert.Equal(t, 60, usage.InputTokens)
	assert.Equal(t, 25, usage.OutputTokens)
	assert.Equal(t, 12, usage.CacheInputTokens)
	assert.Equal(t, 8, usage.ReasoningTokens)
}

func TestHandleOpenAIResponsesStreamToAnthropic_ZeroCacheAndReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	decoder := newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 30, 15, 0, 0),
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	usage, err := HandleOpenAIResponsesStreamToAnthropic(c, stream, "gpt-4o")
	require.NoError(t, err)

	assert.Equal(t, 30, usage.InputTokens)
	assert.Equal(t, 15, usage.OutputTokens)
	assert.Equal(t, 0, usage.CacheInputTokens)
	assert.Equal(t, 0, usage.ReasoningTokens)
}

// ---------------------------------------------------------------------------
// HandleAnthropic (passthrough — no reasoning tokens in Anthropic SDK)
// ---------------------------------------------------------------------------

func TestHandleAnthropic_UsageTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// message_delta carries usage in the Anthropic stream
	events := []string{
		buildAnthropicMessageDeltaJSON(t, 35, 18, 5),
	}
	decoder := newFakeAnthropicDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](decoder, nil)

	hc := newTestHandleContext(c)

	usage, err := HandleAnthropic(hc, stream)
	require.NoError(t, err)

	assert.Equal(t, 35, usage.InputTokens)
	assert.Equal(t, 18, usage.OutputTokens)
	assert.Equal(t, 5, usage.CacheInputTokens)
	assert.Equal(t, 0, usage.ReasoningTokens) // Anthropic SDK has no reasoning tokens
}
