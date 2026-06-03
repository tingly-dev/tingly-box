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

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestHandleResponsesToAnthropicStreamIncompleteKeepsUsageAndMaxTokens guards
// against the regression where the migrated responsesToAnthropicConverter
// treated response.incomplete as a protocol error (discarding output + usage)
// instead of a terminal success. It feeds a truncated Responses API stream
// (max_output_tokens) through the real JSON decoder and asserts the converter
// preserves the partial text, emits a clean message_delta/message_stop with
// stop_reason=max_tokens, and reports upstream usage rather than an error.
func TestHandleResponsesToAnthropicStreamIncompleteKeepsUsageAndMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	events := eventsToStrings([]map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_incomplete"}},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "partial"},
		{"type": "response.incomplete", "response": map[string]any{
			"id":                 "resp_incomplete",
			"status":             "incomplete",
			"incomplete_details": map[string]any{"reason": "max_output_tokens"},
			"usage": map[string]any{
				"input_tokens": 100, "output_tokens": 20, "total_tokens": 120,
				"input_tokens_details":  map[string]any{"cached_tokens": 40},
				"output_tokens_details": map[string]any{"reasoning_tokens": 5},
			},
		}},
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](
		newFakeResponsesDecoder(events), nil,
	)

	usage, err := HandleResponsesToAnthropicV1Stream(protocol.NewHandleContext(c, "proxy-model"), stream, "proxy-model")
	require.NoError(t, err, "response.incomplete must be a terminal success, not an error")
	require.NotNil(t, usage)

	// Usage from the incomplete terminal event (input = 100 total - 40 cached).
	assert.Equal(t, 60, usage.InputTokens)
	assert.Equal(t, 40, usage.CacheInputTokens)
	assert.Equal(t, 20, usage.OutputTokens)
	assert.Equal(t, 5, usage.ReasoningTokens)

	body := w.Body.String()
	// Partial assistant text is preserved.
	assert.Contains(t, body, "partial")
	// stop_reason maps to Anthropic max_tokens; the stream ends cleanly.
	events2 := parseSSEEvents(body)
	msgDelta, ok := events2[eventTypeMessageDelta]
	require.True(t, ok, "should emit a message_delta")
	delta := msgDelta["delta"].(map[string]interface{})
	assert.Equal(t, "max_tokens", delta["stop_reason"])
	_, hasStop := events2[eventTypeMessageStop]
	assert.True(t, hasStop, "should emit message_stop")
	// No error event should be surfaced.
	assert.NotContains(t, body, `"type":"error"`)
}
