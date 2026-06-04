package stream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type responseEventIter struct {
	events []responses.ResponseStreamEventUnion
	idx    int
}

func (s *responseEventIter) Next() bool {
	if s.idx >= len(s.events) {
		return false
	}
	s.idx++
	return true
}

func (s *responseEventIter) Current() responses.ResponseStreamEventUnion { return s.events[s.idx-1] }
func (s *responseEventIter) Err() error                                  { return nil }
func (s *responseEventIter) Close() error                                { return nil }

func TestHandleResponsesToOpenAIChatStreamIncompleteKeepsUsageAndLengthFinish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	iter := &responseEventIter{events: []responses.ResponseStreamEventUnion{
		{Type: "response.created", Response: responses.Response{ID: "resp_incomplete"}},
		{Type: "response.output_text.delta", Delta: "partial"},
		{Type: "response.incomplete", Response: responses.Response{
			ID:     "resp_incomplete",
			Status: responses.ResponseStatusIncomplete,
			IncompleteDetails: responses.ResponseIncompleteDetails{
				Reason: "max_output_tokens",
			},
			Usage: responses.ResponseUsage{
				InputTokens:  100,
				OutputTokens: 20,
				TotalTokens:  120,
				InputTokensDetails: responses.ResponseUsageInputTokensDetails{
					CachedTokens: 40,
				},
				OutputTokensDetails: responses.ResponseUsageOutputTokensDetails{
					ReasoningTokens: 5,
				},
			},
		}},
	}}

	usage, err := HandleResponsesToOpenAIChatStream(protocol.NewHandleContext(c, "proxy-model"), iter, "proxy-model")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 60, usage.InputTokens)
	require.Equal(t, 40, usage.CacheInputTokens)
	require.Equal(t, 20, usage.OutputTokens)
	require.Equal(t, 5, usage.ReasoningTokens)
	body := w.Body.String()
	require.Contains(t, body, `"content":"partial"`)
	require.Contains(t, body, `"finish_reason":"length"`)
	require.Contains(t, body, `"prompt_tokens":100`)
	require.Contains(t, body, `"completion_tokens":20`)
	require.Contains(t, body, `data: [DONE]`)
	require.Equal(t, 1, strings.Count(body, `"finish_reason":"length"`))
}
