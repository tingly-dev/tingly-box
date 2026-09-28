package stream

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestHandleResponsesToOpenAIChatStreamFailedSurfacesUpstreamError guards
// against response.failed being silently dropped: previously the converter
// had no case for it, so once the stream closed it fell back to a generic
// "responses stream ended without a terminal event" error and the real
// upstream reason (content policy, rate limit, etc.) never reached the
// Chat Completions client.
func TestHandleResponsesToOpenAIChatStreamFailedSurfacesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	iter := &responseEventIter{events: []responses.ResponseStreamEventUnion{
		{Type: "response.created", Response: responses.Response{ID: "resp_failed"}},
		{Type: "response.failed", Response: responses.Response{
			ID:     "resp_failed",
			Status: responses.ResponseStatusFailed,
			Error: responses.ResponseError{
				Code:    responses.ResponseErrorCodeInvalidPrompt,
				Message: "Your request was rejected by the safety system.",
			},
		}},
	}}

	_, err := HandleResponsesToOpenAIChatStream(protocol.NewHandleContext(c, "proxy-model"), iter, "proxy-model")
	require.NoError(t, err)

	body := w.Body.String()
	require.Contains(t, body, "Your request was rejected by the safety system.")
	require.Contains(t, body, "invalid_prompt")
	require.Contains(t, body, `data: [DONE]`)
}

func TestHandleResponsesToOpenAIChatStreamErrorEventDoesNotDoubleError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	iter := &responseEventIter{events: []responses.ResponseStreamEventUnion{
		{Type: "response.created", Response: responses.Response{ID: "resp_err"}},
		{Type: "error", Code: "rate_limit_exceeded", Message: "Too many requests.", Param: "rate_limit_exceeded"},
	}}

	_, err := HandleResponsesToOpenAIChatStream(protocol.NewHandleContext(c, "proxy-model"), iter, "proxy-model")
	require.NoError(t, err)

	body := w.Body.String()
	require.Contains(t, body, "Too many requests.")
	require.Contains(t, body, `data: [DONE]`)
}
