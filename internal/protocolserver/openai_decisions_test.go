package protocolserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

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
	u = decisionUsage([]byte(`{"usage":{"inputTokens":7,"outputTokens":1,"prompt_tokens":7}}`))
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

func TestSystemOneEntryAndDialectGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream, lastPath := newPathRecordingUpstream(t, map[string]any{
		"model": "upstream", "answers": map[string]any{"q": map[string]any{"type": "noul", "noul": 0.7}},
	})
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic,
		typ.ScenarioOpenAI, "decision-model", "upstream")
	for _, path := range []string{"/tingly/openai/v1/systemone", "/tingly/openai/systemone", "/tingly/openai/v1/decisions"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"decision-model","state":"x","questions":{"q":{"type":"noul"}}}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		require.Equal(t, "/v1/decisions", lastPath())
		require.Equal(t, "decision-model", gjson.Get(w.Body.String(), "model").String())
	}
	// Both aliases behave identically when the response does not match the request.
	for _, path := range []string{"/tingly/openai/v1/systemone", "/tingly/openai/v1/decisions"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"decision-model","input":"x","questions":[]}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadGateway, w.Code, "%s: %s", path, w.Body.String())
	}
}

func TestDecisionAliasesAcceptOpenAIFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream, _ := newPathRecordingUpstream(t, map[string]any{
		"model": "upstream", "answers": []any{map[string]any{"type": "predicate", "name": "q", "probability": 0.7}},
	})
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic,
		typ.ScenarioOpenAI, "decision-model", "upstream")
	for _, path := range []string{"/tingly/openai/v1/systemone", "/tingly/openai/v1/decisions"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"decision-model","input":"x","questions":[{"type":"predicate","name":"q","instructions":"Is it true?"}]}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		require.Equal(t, "predicate", gjson.Get(w.Body.String(), "answers.0.type").String())
	}
}

func TestDecisionKnownProvidersConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct{ name, base, path, request, response, wantRequest, wantAnswer string }{
		{"typesafe", "http://api.typesafe.ai", "/v1/systemone", `{"model":"client","input":"hello","questions":[{"type":"predicate","name":"q","instructions":"urgent?"}]}`, `{"model":"upstream","answers":{"q":{"type":"noul","noul":0.8}},"usage":{"input_tokens":7,"output_tokens":1}}`, `{"model":"upstream","state":"hello","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`, `{"type":"predicate","name":"q","probability":0.8}`},
		{"openrouter", "http://openrouter.ai/api", "/api/alpha/decisions", `{"model":"client","input":"hello","questions":[{"type":"predicate","name":"q","instructions":"urgent?"}]}`, `{"model":"upstream","answers":{"q":{"type":"noul","noul":0.8}}}`, `{"model":"upstream","state":"hello","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`, `{"type":"predicate","name":"q","probability":0.8}`},
		{"openai", "http://api.openai.com", "/v1/decisions", `{"model":"client","state":"hello","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`, `{"model":"upstream","answers":[{"type":"predicate","name":"q","probability":0.8}]}`, `{"model":"upstream","input":"hello","questions":[{"type":"predicate","name":"q","instructions":"urgent?"}]}`, `{"type":"noul","noul":0.8}`},
		{"vercel", "http://ai-gateway.vercel.sh", "/v1/decisions", `{"model":"client","state":"hello","questions":{"q":{"type":"noul","instructions":"urgent?"}}}`, `{"model":"upstream","answers":[{"type":"predicate","name":"q","probability":0.8}]}`, `{"model":"upstream","input":"hello","questions":[{"type":"predicate","name":"q","instructions":"urgent?"}]}`, `{"type":"noul","noul":0.8}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer proxy.Close()
			router := newDualProviderRouterWithProxy(t, tc.base, proxy.URL, protocol.APIStyleAnthropic, typ.ScenarioOpenAI, "client", "upstream")
			for _, path := range []string{"/tingly/openai/v1/decisions", "/tingly/openai/v1/systemone"} {
				gotPath = ""
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.request))
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code, "%s", w.Body.String())
				require.Equal(t, tc.path, gotPath)
				require.JSONEq(t, tc.wantRequest, string(gotBody))
				require.Equal(t, "client", gjson.Get(w.Body.String(), "model").String())
				answer := gjson.Get(w.Body.String(), "answers.0")
				if tc.name == "openai" || tc.name == "vercel" {
					answer = gjson.Get(w.Body.String(), "answers.q")
				}
				require.JSONEq(t, tc.wantAnswer, answer.Raw)
			}
			if tc.name == "typesafe" {
				gotPath = ""
				req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/decisions", strings.NewReader(`{"model":"client","input":"x","prompt_cache_key":"k","questions":[{"type":"predicate","name":"q","instructions":"x"}]}`))
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Empty(t, gotPath, "unsupported fields must be rejected before calling upstream")
			}
		})
	}
}
