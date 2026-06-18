package stream

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// TestSendAnthropicStreamEvent_TTFTOnlyOnContentDelta verifies TTFT is marked
// only on content_block_delta, not on structural events.
func TestSendAnthropicStreamEvent_TTFTOnlyOnContentDelta(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		marks     bool
	}{
		{"message_start is structural", "message_start", false},
		{"content_block_start is structural", "content_block_start", false},
		{"ping is structural", "ping", false},
		{"content_block_stop is structural", "content_block_stop", false},
		{"content_block_delta text marks TTFT", "content_block_delta", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			sendAnthropicStreamEvent(c, tc.eventType, map[string]any{"type": tc.eventType}, w)

			_, ok := c.Get(constant.CtxKeyFirstTokenTime)
			assert.Equal(t, tc.marks, ok, "TTFT mark expectation for %s", tc.eventType)
		})
	}
}

// TestHandleResponsesToAnthropicV1Stream_RecordsTTFTOnDelta verifies the
// Responses→Anthropic conversion records TTFT at the first content delta, not
// the leading response.created / message_start.
func TestHandleResponsesToAnthropicV1Stream_RecordsTTFTOnDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// No first-token time before streaming starts.
	_, exists := c.Get(constant.CtxKeyFirstTokenTime)
	require.False(t, exists)

	events := eventsToStrings([]map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_1"}},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "hi"},
		{"type": "response.completed", "response": map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"usage": map[string]any{
				"input_tokens": 5, "output_tokens": 2, "total_tokens": 7,
			},
		}},
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](
		newFakeResponsesDecoder(events), nil,
	)

	_, err := HandleResponsesToAnthropicV1Stream(protocol.NewHandleContext(c, "proxy-model"), stream, "proxy-model")
	require.NoError(t, err)

	// TTFT must be recorded once content flowed.
	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "streaming Responses->Anthropic should record TTFT on first content delta")

	// message_start proves TTFT fired on the delta, not the first byte.
	body := w.Body.String()
	assert.Contains(t, body, "event:message_start", "converter emits message_start framing")
	assert.Contains(t, body, "event:content_block_delta", "converter emits the content delta")
}

// TestIsAnthropicContentDeltaEvent verifies the gate used to mark TTFT.
func TestIsAnthropicContentDeltaEvent(t *testing.T) {
	assert.True(t, isAnthropicContentDeltaEvent("content_block_delta"))
	assert.False(t, isAnthropicContentDeltaEvent("message_start"))
	assert.False(t, isAnthropicContentDeltaEvent("content_block_start"))
	assert.False(t, isAnthropicContentDeltaEvent("ping"))
	assert.False(t, isAnthropicContentDeltaEvent(""))
}

// TestIsOpenAIResponsesContentEvent verifies the Responses gate: content
// arrives as *.delta events, structural events do not count.
func TestIsOpenAIResponsesContentEvent(t *testing.T) {
	assert.True(t, isOpenAIResponsesContentEvent("response.output_text.delta"))
	assert.True(t, isOpenAIResponsesContentEvent("response.function_call_arguments.delta"))
	assert.True(t, isOpenAIResponsesContentEvent("response.reasoning.delta"))
	assert.False(t, isOpenAIResponsesContentEvent("response.created"))
	assert.False(t, isOpenAIResponsesContentEvent("response.in_progress"))
	assert.False(t, isOpenAIResponsesContentEvent("response.output_item.added"))
	assert.False(t, isOpenAIResponsesContentEvent("response.completed"))
	assert.False(t, isOpenAIResponsesContentEvent(""))
}

// TestOpenAIResponsesEvent_TTFTOnlyOnContentDelta verifies the Responses funnel
// marks TTFT only on content-bearing events.
func TestOpenAIResponsesEvent_TTFTOnlyOnContentDelta(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event string
		marks bool
	}{
		{"created", "response.created", false},
		{"in_progress", "response.in_progress", false},
		{"output_item.added", "response.output_item.added", false},
		{"output_text.delta", "response.output_text.delta", true},
		{"function_call_arguments.delta", "response.function_call_arguments.delta", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			OpenAIResponsesEvent(c, tc.event, map[string]any{"type": tc.event})

			_, ok := c.Get(constant.CtxKeyFirstTokenTime)
			assert.Equal(t, tc.marks, ok, "TTFT for %s", tc.event)
		})
	}
}

// TestIsOpenAIChatContentChunk verifies the typed OpenAI-Chat content gate used
// by the converter output funnel (openaiChatSSEWriter).
func TestIsOpenAIChatContentChunk(t *testing.T) {
	roleOnly := wire.ChatStreamChunk{Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{Role: "assistant"}}}}
	assert.False(t, isOpenAIChatContentChunk(roleOnly), "role-only delta is not content")

	text := wire.ChatStreamChunk{Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{Content: "hi"}}}}
	assert.True(t, isOpenAIChatContentChunk(text), "text delta is content")

	tool := wire.ChatStreamChunk{Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{ToolCalls: []wire.ChatStreamToolCall{{Index: 0}}}}}}
	assert.True(t, isOpenAIChatContentChunk(tool), "tool_calls delta is content")

	reasoning := wire.ChatStreamChunk{Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{ReasoningContent: "thinking"}}}}
	assert.True(t, isOpenAIChatContentChunk(reasoning), "reasoning delta is content")
}

// TestIsOpenAIChatChunkMapContent verifies the raw-map variant used by handlers
// that build OpenAI Chat chunks directly (e.g. Google→OpenAI).
func TestIsOpenAIChatChunkMapContent(t *testing.T) {
	assert.False(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}}},
	}), "role-only is not content")

	assert.True(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": "hi"}}},
	}), "content text is content")

	assert.True(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"tool_calls": []map[string]any{{"index": 0}}}}},
	}), "tool_calls is content")

	assert.True(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"reasoning_content": "thinking"}}},
	}), "reasoning_content is content")

	assert.True(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"refusal": "I can't help with that"}}},
	}), "refusal is content")

	assert.False(t, isOpenAIChatChunkMapContent(map[string]any{
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	}), "finish-only is not content")
}

// TestSendGoogleStreamChunk_TTFTOnlyOnContent verifies the Google-output funnel
// marks TTFT only when a chunk carries text or function-call content.
func TestSendGoogleStreamChunk_TTFTOnlyOnContent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		resp  *genai.GenerateContentResponse
		marks bool
	}{
		{"nil resp", nil, false},
		{"empty candidates", &genai.GenerateContentResponse{}, false},
		{"text content", &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: "hi"}}}}},
		}, true},
		{"function call", &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "f"}}}}}},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/x:streamGenerateContent", nil)

			sendGoogleStreamChunk(c, tc.resp, w)

			_, ok := c.Get(constant.CtxKeyFirstTokenTime)
			assert.Equal(t, tc.marks, ok, "TTFT for %s", tc.name)
		})
	}
}
