package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDecisionRequestModel(t *testing.T) {
	m, err := decisionRequestModel([]byte(`{"model":"luna","anything":{"goes":1}}`))
	require.NoError(t, err)
	require.Equal(t, "luna", m)

	for name, raw := range map[string]string{
		"not json":   `[{`,
		"no model":   `{"options":["a"]}`,
		"blank":      `{"model":"  "}`,
		"non-string": `{"model":3}`,
	} {
		_, err := decisionRequestModel([]byte(raw))
		require.Error(t, err, name)
	}
}

func TestDecisionUsage(t *testing.T) {
	require.Equal(t, 0, int(decisionUsage([]byte(`{}`)).InputTokens))
	u := decisionUsage([]byte(`{"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	require.Equal(t, 5, int(u.InputTokens))
	require.Equal(t, 2, int(u.OutputTokens))
	u = decisionUsage([]byte(`{"usage":{"input_tokens":7,"output_tokens":1}}`))
	require.Equal(t, 7, int(u.InputTokens))
	require.Equal(t, 1, int(u.OutputTokens))
}

func TestDecisionEchoModel(t *testing.T) {
	same := []byte(`{"model":"luna",  "a":1}`)
	require.Equal(t, string(same), string(decisionEchoModel(same, "luna")))
	require.JSONEq(t, `{"model":"luna","a":1}`, string(decisionEchoModel([]byte(`{"model":"upstream","a":1}`), "luna")))
	none := []byte(`{"a":1}`)
	require.Equal(t, string(none), string(decisionEchoModel(none, "luna")))
}

// End to end through the real route: the routed model replaces the request
// model upstream, the response echoes the caller's model, and the request
// lands on {APIBase}/decisions. The provider is doubly adversarial (anthropic
// primary style, dead primary base) so a served request also proves the
// OpenAI dual URL is honored.
func TestDecisionsForwarding_RoutesToOpenAIDecisionsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream, lastPath := newPathRecordingUpstream(t, map[string]any{
		"answer": "b",
		"model":  "decision-upstream-model",
		"usage":  map[string]any{"prompt_tokens": 4, "completion_tokens": 1},
	})
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic,
		typ.ScenarioOpenAI, "decision-model", "decision-upstream-model")

	body := `{"model":"decision-model","options":["a","b"],"context":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/decisions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Equal(t, "/v1/decisions", lastPath())
	require.JSONEq(t, `{"answer":"b","model":"decision-model","usage":{"prompt_tokens":4,"completion_tokens":1}}`, w.Body.String())

	// A body without a model never reaches the upstream.
	req = httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/decisions", strings.NewReader(`{"options":["a"]}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
