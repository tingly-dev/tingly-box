package protocolserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
)

// Every AI endpoint's early rejection must reach the request's timeline and
// echo the request_id — count_tokens and GET /responses/:id included.
func TestAIEndpointRejections_LandOnRequestTimeline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph := &ProtocolHandler{}

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		params     gin.Params
		handler    gin.HandlerFunc
		wantStatus int
	}{
		{"count_tokens bad body", http.MethodPost, "/tingly/claude_code/v1/messages/count_tokens", "{", gin.Params{{Key: "scenario", Value: "claude_code"}}, ph.AnthropicCountTokens, http.StatusBadRequest},
		{"count_tokens no model", http.MethodPost, "/tingly/claude_code/v1/messages/count_tokens", `{"messages":[]}`, gin.Params{{Key: "scenario", Value: "claude_code"}}, ph.AnthropicCountTokens, http.StatusBadRequest},
		{"responses get", http.MethodGet, "/tingly/codex/v1/responses/resp_1", "", gin.Params{{Key: "scenario", Value: "codex"}, {Key: "id", Value: "resp_1"}}, ph.HandleResponsesGet, http.StatusNotFound},
		{"embeddings bad body", http.MethodPost, "/tingly/openai/v1/embeddings", "{", gin.Params{{Key: "scenario", Value: "openai"}}, ph.HandleOpenAIEmbeddings, http.StatusBadRequest},
		{"chat bad body", http.MethodPost, "/tingly/openai/v1/chat/completions", "{", gin.Params{{Key: "scenario", Value: "openai"}}, ph.HandleOpenAIChatCompletions, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hook := test.NewGlobal()
			defer hook.Reset()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			c.Request = req.WithContext(obs.ContextWithRequestID(req.Context(), "req-1"))
			c.Set(constant.CtxKeyRequestID, "req-1")
			c.Params = tc.params

			tc.handler(c)

			assert.Equal(t, tc.wantStatus, w.Code)
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "req-1", body.Error.RequestID)
			assert.NotEmpty(t, c.Errors, "error must reach the access log")

			var onTimeline bool
			for _, e := range hook.AllEntries() {
				if obs.RequestIDFromContext(e.Context) == "req-1" && e.Message == "request rejected" {
					onTimeline = true
				}
			}
			assert.True(t, onTimeline, "rejection must be logged with the request context")
		})
	}
}

// A request rejected before routing must still be findable in the Logs page:
// a request-scoped event carrying the error, scenario/model for the access
// log, the error on c.Errors, and the request_id in the body.
func TestRejectRequest_RecordsForLogsPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hook := test.NewGlobal()
	defer hook.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/tingly/imagegen/v1/images/generations", nil)
	c.Request = req.WithContext(obs.ContextWithRequestID(req.Context(), "req-1"))
	c.Set(constant.CtxKeyRequestID, "req-1")

	rejectRequest(c, "routing", "gpt-image-2", errors.New("no rule for model gpt-image-2"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "req-1", body.Error.RequestID)
	assert.Equal(t, "no rule for model gpt-image-2", body.Error.Message)

	assert.Len(t, c.Errors, 1)
	assert.Equal(t, "imagegen", c.GetString(ContextKeyScenario))
	assert.Equal(t, "gpt-image-2", c.GetString(ContextKeyRequestModel))

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.WarnLevel, entry.Level)
	assert.Equal(t, "req-1", obs.RequestIDFromContext(entry.Context))
	assert.Equal(t, "routing", entry.Data["stage"])
	assert.NotNil(t, entry.Data[logrus.ErrorKey])
}
