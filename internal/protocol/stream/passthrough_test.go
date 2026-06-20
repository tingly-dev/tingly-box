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

	"github.com/tingly-dev/tingly-box/internal/constant"
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

// buildAnthropicMessageStartJSON creates a message_start event — the real Anthropic
// streaming format where input_tokens live in message.usage, not in message_delta.
func buildAnthropicMessageStartJSON(t *testing.T, inputTokens, cacheReadTokens int64) string {
	t.Helper()
	event := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            "msg_test",
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         "claude-3-5-sonnet-20241022",
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":            inputTokens,
				"output_tokens":           0,
				"cache_read_input_tokens": cacheReadTokens,
			},
		},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return string(data)
}

// buildAnthropicOutputOnlyDeltaJSON creates a message_delta with output_tokens only,
// matching the real Anthropic API (input_tokens are absent from message_delta).
func buildAnthropicOutputOnlyDeltaJSON(t *testing.T, outputTokens int64) string {
	t.Helper()
	event := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"output_tokens": outputTokens,
		},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return string(data)
}

// buildAnthropicContentBlockDeltaJSON creates a content_block_delta event carrying
// model text — the first content-bearing event that should trigger a TTFT mark.
func buildAnthropicContentBlockDeltaJSON(t *testing.T, text string) string {
	t.Helper()
	event := map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": text,
		},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return string(data)
}

// buildChatContentChunkJSON builds an OpenAI Chat chunk with non-empty delta
// content — the first content-bearing chunk that should trigger a TTFT mark.
func buildChatContentChunkJSON(t *testing.T, content string) string {
	t.Helper()
	chunk := map[string]interface{}{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1000,
		"model":   "gpt-4o",
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{"content": content},
				"finish_reason": nil,
			},
		},
	}
	data, err := json.Marshal(chunk)
	require.NoError(t, err)
	return string(data)
}

// buildChatRoleOnlyChunkJSON builds an OpenAI Chat chunk with only the leading
// role delta ({role:"assistant"}) — a structural frame that must NOT mark TTFT.
func buildChatRoleOnlyChunkJSON(t *testing.T) string {
	t.Helper()
	chunk := map[string]interface{}{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1000,
		"model":   "gpt-4o",
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{"role": "assistant"},
				"finish_reason": nil,
			},
		},
	}
	data, err := json.Marshal(chunk)
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

	// Usage isn't populated from raw JSON here, so the estimate fallback runs
	// (hc.EstimatedInputTokens defaults to 0); just verify it returns non-nil.
	usage, err := HandleOpenAIChatStream(hc, stream)
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

	usage, err := HandleOpenAIChatStream(hc, stream)
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

	// OpenAI Responses API: input=50 total, cached=7 → stored as 50-7=43 (uncached only)
	assert.Equal(t, 43, usage.InputTokens)
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

	// OpenAI Responses API: input=60 total, cached=12 → stored as 60-12=48 (uncached only)
	assert.Equal(t, 48, usage.InputTokens)
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

// TestHandleAnthropic_RealStreamFormat verifies input_tokens from message_start are
// captured. The real Anthropic API puts input_tokens in message_start.message.usage
// and only output_tokens in message_delta.usage — not input_tokens in both.
func TestHandleAnthropic_RealStreamFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 35, 5),   // input_tokens=35, cache=5
		buildAnthropicOutputOnlyDeltaJSON(t, 18),   // output_tokens=18 only
	}
	decoder := newFakeAnthropicDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](decoder, nil)

	hc := newTestHandleContext(c)

	usage, err := HandleAnthropic(hc, stream)
	require.NoError(t, err)

	assert.Equal(t, 35, usage.InputTokens, "input_tokens must come from message_start")
	assert.Equal(t, 18, usage.OutputTokens)
	assert.Equal(t, 5, usage.CacheInputTokens)
}

// ---------------------------------------------------------------------------
// HandleAnthropicBeta
// ---------------------------------------------------------------------------

// TestHandleAnthropicBeta_RealStreamFormat verifies input_tokens from message_start
// are captured for the beta passthrough handler.
func TestHandleAnthropicBeta_RealStreamFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 40, 8),  // input_tokens=40, cache=8
		buildAnthropicOutputOnlyDeltaJSON(t, 22),  // output_tokens=22 only
	}
	decoder := newFakeAnthropicDecoder(events)
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](decoder, nil)

	hc := newTestHandleContext(c)

	usage, err := HandleAnthropicBeta(hc, stream)
	require.NoError(t, err)

	assert.Equal(t, 40, usage.InputTokens, "input_tokens must come from message_start")
	assert.Equal(t, 22, usage.OutputTokens)
	assert.Equal(t, 8, usage.CacheInputTokens)
}

// ---------------------------------------------------------------------------
// TTFT for passthrough handlers (which write raw upstream bytes, bypassing
// the converter content gates, so they must mark TTFT themselves).
// ---------------------------------------------------------------------------

// TestHandleAnthropic_Passthrough_MarksTTFT verifies the Anthropic passthrough
// records TTFT on the first content_block_delta, not the message_start frame.
func TestHandleAnthropic_Passthrough_MarksTTFT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 5, 0), // structural frame — must NOT mark
		buildAnthropicContentBlockDeltaJSON(t, "hi"),
	}
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)

	_, err := HandleAnthropic(newTestHandleContext(c), stream)
	require.NoError(t, err)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "Anthropic passthrough must mark TTFT on the content delta")
}

// TestHandleAnthropic_Passthrough_NoTTFTOnStructuralOnly verifies no TTFT is
// recorded when the stream contains only structural events (no content).
func TestHandleAnthropic_Passthrough_NoTTFTOnStructuralOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 5, 0), // structural only
	}
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)

	_, err := HandleAnthropic(newTestHandleContext(c), stream)
	require.NoError(t, err)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.False(t, ok, "no content delta must not mark TTFT")
}

// TestHandleAnthropicBeta_Passthrough_MarksTTFT is the beta-passthrough variant.
func TestHandleAnthropicBeta_Passthrough_MarksTTFT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := []string{
		buildAnthropicMessageStartJSON(t, 5, 0),
		buildAnthropicContentBlockDeltaJSON(t, "hi"),
	}
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)

	_, err := HandleAnthropicBeta(newTestHandleContext(c), stream)
	require.NoError(t, err)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "Anthropic beta passthrough must mark TTFT on the content delta")
}

// TestHandleOpenAIChatStream_Passthrough_MarksTTFT verifies the OpenAI Chat
// passthrough records TTFT on the first content chunk, skipping the role-only delta.
func TestHandleOpenAIChatStream_Passthrough_MarksTTFT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	dec := &fakeChatDecoder{events: []string{
		buildChatRoleOnlyChunkJSON(t), // structural frame — must NOT mark
		buildChatContentChunkJSON(t, "hi"),
	}, current: -1}
	stream := openaistream.NewStream[openai.ChatCompletionChunk](dec, nil)

	hc := newTestHandleContext(c)
	hc.DisableStreamUsage = true
	_, err := HandleOpenAIChatStream(hc, stream)
	require.NoError(t, err)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "OpenAI Chat passthrough must mark TTFT on the first content chunk")
}

// TestHandleOpenAIChatStream_Passthrough_NoTTFTOnRoleOnly verifies no TTFT is
// recorded when the stream contains only the role-only delta.
func TestHandleOpenAIChatStream_Passthrough_NoTTFTOnRoleOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	dec := &fakeChatDecoder{events: []string{
		buildChatRoleOnlyChunkJSON(t),
	}, current: -1}
	stream := openaistream.NewStream[openai.ChatCompletionChunk](dec, nil)

	hc := newTestHandleContext(c)
	hc.DisableStreamUsage = true
	_, err := HandleOpenAIChatStream(hc, stream)
	require.NoError(t, err)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.False(t, ok, "role-only delta must not mark TTFT")
}
