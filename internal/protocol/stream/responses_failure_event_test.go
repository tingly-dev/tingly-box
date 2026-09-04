package stream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// sseEventPayload returns the data payload of the named SSE event.
func sseEventPayload(t *testing.T, body, event string) string {
	t.Helper()
	marker := "event: " + event + "\ndata: "
	idx := strings.Index(body, marker)
	require.GreaterOrEqual(t, idx, 0, "event %q not found in stream:\n%s", event, body)
	rest := body[idx+len(marker):]
	end := strings.Index(rest, "\n")
	require.GreaterOrEqual(t, end, 0)
	return rest[:end]
}

func newResponsesStreamContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	w := &closeNotifyRecorder{ResponseRecorder: rec}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

// A truncated upstream must end the stream with response.failed, not only the
// legacy `error` frame: Codex's SSE reader has no arm for "error" and drops it,
// leaving the client with the generic "stream closed before response.completed"
// and no reason at all.
func TestHandleOpenAIResponsesStream_TruncationEmitsResponseFailed(t *testing.T) {
	c, rec := newResponsesStreamContext(t)

	decoder := newFakeResponsesDecoder([]string{
		`{"type":"response.created","response":{"id":"resp_42","model":"upstream-model"}}`,
		`{"type":"response.output_text.delta","item_id":"item_1","output_index":0,"delta":"partial"}`,
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	_, err := HandleOpenAIResponsesStream(newTestHandleContext(c), stream, "gpt-4o")
	require.Error(t, err)

	payload := sseEventPayload(t, rec.Body.String(), "response.failed")
	assert.Equal(t, "response.failed", gjson.Get(payload, "type").String())
	assert.Equal(t, "failed", gjson.Get(payload, "response.status").String())
	assert.Equal(t, "upstream_truncated", gjson.Get(payload, "response.error.code").String())
	assert.Contains(t, gjson.Get(payload, "response.error.message").String(), "without a terminal event")
	// The id the client was already streaming, not an invented one.
	assert.Equal(t, "resp_42", gjson.Get(payload, "response.id").String())
	// The legacy error frame stays for clients that key off it.
	assert.Contains(t, rec.Body.String(), "upstream_truncated")
	assert.NotContains(t, rec.Body.String(), "response.completed")
}

// response.created / response.in_progress carry "usage": null. Backfilling
// reasoning_tokens there would replace the null with a usage object missing
// every required field, which strict clients reject.
func TestHandleOpenAIResponsesStream_NullUsageStaysNull(t *testing.T) {
	c, rec := newResponsesStreamContext(t)

	decoder := newFakeResponsesDecoder([]string{
		`{"type":"response.created","response":{"id":"resp_1","model":"m","usage":null}}`,
		`{"type":"response.completed","response":{"id":"resp_1","model":"m","usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":2},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":13}}}`,
	})
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](decoder, nil)

	_, err := HandleOpenAIResponsesStream(newTestHandleContext(c), stream, "gpt-4o")
	require.NoError(t, err)

	created := sseEventPayload(t, rec.Body.String(), "response.created")
	assert.Equal(t, gjson.Null, gjson.Get(created, "response.usage").Type,
		"response.created must keep its null usage")
}

// An upstream event carrying only `"error": null` aborts the SDK stream with an
// empty message; the raw event is the only diagnostic left, so keep it.
func TestDescribeResponsesStreamError_EmptyInBandError(t *testing.T) {
	raw := `{"type":"response.completed","error":null}`
	err := describeResponsesStreamError(&openaistream.StreamError{
		Message: "received error while streaming: ",
		Event:   openaistream.Event{Type: "response.completed", Data: []byte(raw)},
	})
	assert.Contains(t, err.Error(), "error event with no message")
	assert.Contains(t, err.Error(), raw)
}

func TestDescribeResponsesStreamError_KeepsRealMessage(t *testing.T) {
	original := &openaistream.StreamError{
		Message: `received error while streaming: {"message":"boom"}`,
		Event:   openaistream.Event{Data: []byte(`{"error":{"message":"boom"}}`)},
	}
	assert.Equal(t, error(original), describeResponsesStreamError(original))
}
