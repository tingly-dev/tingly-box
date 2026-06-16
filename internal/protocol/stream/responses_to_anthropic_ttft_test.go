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

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestHandleResponsesToAnthropicV1Stream_RecordsTTFT verifies the streaming
// Responses API → Anthropic conversion records the first-token time (TTFT)
// through CommitFirstChunk, so the dashboard shows a real TTFT rather than "-".
func TestHandleResponsesToAnthropicV1Stream_RecordsTTFT(t *testing.T) {
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

	// The first emitted event must have recorded the first-token time.
	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "streaming Responses->Anthropic should record TTFT via CommitFirstChunk")
}
