package protocolserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNativeStandaloneCompactToContinue(t *testing.T) {
	const window = `[{"type":"message","role":"user","content":"keep"},{"type":"compaction","id":"cmp_native","encrypted_content":"native-state","future":7}]`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		switch r.URL.Path {
		case "/v1/responses/compact":
			_, _ = io.WriteString(w, `{"object":"response.compaction","output":`+window+`}`)
		case "/v1/responses":
			require.Equal(t, "native-state", gjson.GetBytes(body, "input.1.encrypted_content").String())
			require.Equal(t, int64(7), gjson.GetBytes(body, "input.1.future").Int())
			require.Equal(t, "continue", gjson.GetBytes(body, "input.2.content").String())
			_, _ = io.WriteString(w, `{"id":"resp_next","object":"response","status":"completed","output":[{"id":"msg_next","type":"message","role":"assistant","content":[{"type":"output_text","text":"continued"}]}]}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioOpenAI, "compact-model", "native-model")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(`{"model":"compact-model","input":"keep"}`)))
	require.Equal(t, 200, w.Code, w.Body.String())
	var input []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(gjson.Get(w.Body.String(), "output").Raw), &input))
	input = append(input, json.RawMessage(`{"role":"user","content":"continue"}`))
	replay, err := json.Marshal(map[string]any{"model": "compact-model", "input": input})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses", strings.NewReader(string(replay))))
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "continued")
}

func TestOpaqueReplayExtensionsSurviveParsingAndCloning(t *testing.T) {
	body := []byte(`{"model":"alias","input":[{"type":"compaction","encrypted_content":"native","future":{"counter":9007199254740993}},{"type":"reasoning","encrypted_content":"reasoning","summary":[],"future.key":7},{"type":"compaction_trigger","future":true}]}`)
	ph := &ProtocolHandler{}
	params, err := ph.convertToResponsesParams(body, "actual")
	require.NoError(t, err)
	for range 2 {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		require.Equal(t, "9007199254740993", gjson.GetBytes(raw, "input.0.future.counter").Raw)
		require.Equal(t, int64(7), gjson.GetBytes(raw, `input.1.future\.key`).Int())
		require.True(t, gjson.GetBytes(raw, "input.2.future").Bool())
		require.False(t, gjson.GetBytes(raw, "input.0.id").Exists())
		require.False(t, gjson.GetBytes(raw, "input.2.id").Exists())
		params, err = CloneResponsesParams(params)
		require.NoError(t, err)
	}
}

func TestCompactEndpointNativeForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const result = `{"object":"response.compaction", "output":[{"type":"message","role":"user","content":"retained"},{"type":"compaction","encrypted_content":"opaque-result","future":true}],"usage":{"input_tokens":9,"output_tokens":2}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/responses/compact", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "native-model", gjson.GetBytes(body, "model").String())
		require.Equal(t, `[{"type":"reasoning","encrypted_content":"opaque-input","future":7}]`, gjson.GetBytes(body, "input").Raw)
		require.False(t, gjson.GetBytes(body, "stream").Exists())
		require.False(t, gjson.GetBytes(body, "store").Exists())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, result)
	}))
	defer upstream.Close()
	// Exercise dual-URL resolution as well as the real routing/client stack.
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic, typ.ScenarioOpenAI, "compact-model", "native-model")
	for _, path := range []string{"/tingly/openai/v1/responses/compact", "/tingly/openai/responses/compact"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"compact-model", "input":[{"type":"reasoning","encrypted_content":"opaque-input","future":7}]}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
		require.Equal(t, result, w.Body.String())
	}
}

// Compact has a JSON response contract, but TB must not impose its own
// stream-parameter policy on native upstreams.
func TestCompactEndpointStreamPassthrough(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Equal(t, value, gjson.GetBytes(body, "stream").Raw)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if status == http.StatusOK {
						_, _ = io.WriteString(w, `{"object":"response.compaction","output":[]}`)
					} else {
						_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"upstream rejected stream"}}`)
					}
				}))
				router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioOpenAI, "compact-model", "native-model")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(`{"model":"compact-model","stream":`+value+`}`)))
				upstream.Close()
				require.Equal(t, status, w.Code, w.Body.String())
				require.Equal(t, int32(1), calls.Load())
				if status == http.StatusBadRequest {
					require.Contains(t, w.Body.String(), "upstream rejected stream")
				}
			}
		})
	}
}

func TestCompactEndpointValidationAndUpstreamFailures(t *testing.T) {
	for _, status := range []int{400, 403, 429, 502} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":{"type":"internal_error","message":"response protection is unavailable"}}`)
			}))
			defer upstream.Close()
			router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioOpenAI, "compact-model", "native-model")
			for _, bad := range []string{`{`, `[]`, `{}`, `{"model":3}`, `{"model":" "}`} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(bad)))
				require.Equal(t, 400, w.Code, w.Body.String())
			}
			require.Zero(t, calls.Load())
			// input is optional in the published schema (e.g. previous_response_id).
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(`{"model":"compact-model","previous_response_id":"resp_prior"}`)))
			require.Equal(t, status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "response protection is unavailable")
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestCompactEndpointAuthAndTeamScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	for _, path := range []string{"/tingly/openai/v1/responses/compact", "/tingly/openai/responses/compact", "/tingly/team/v1/responses/compact"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"m"}`)))
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
	router = gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)
	req := httptest.NewRequest(http.MethodPost, "/tingly/team/v1/responses/compact", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("X-Test-Team-ID", "team-a")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), "not configured", "team-scoped request reaches rule resolution")
}

func TestCompactUnsupportedAndUpstreamErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		wantStatus int
		wantCode   string
	}{
		{"missing endpoint", 404, "", 400, "unsupported_compaction"},
		{"endpoint not found", 404, "endpoint_not_found", 400, "unsupported_compaction"},
		{"wrong method", 405, "", 400, "unsupported_compaction"},
		{"not implemented", 501, "", 400, "unsupported_compaction"},
		{"explicit unsupported", 400, "unsupported_operation", 400, "unsupported_compaction"},
		{"model not found", 404, "model_not_found", 404, "model_not_found"},
		{"authentication", 401, "invalid_api_key", 401, "invalid_api_key"},
		{"permission", 403, "permission_denied", 403, "permission_denied"},
		{"quota", 429, "rate_limit_exceeded", 429, "rate_limit_exceeded"},
		{"protection failure", 400, "response_protection_error", 400, "response_protection_error"},
		{"transient failure", 503, "server_error", 503, "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/v1/responses/compact", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": tc.code, "message": "fixture error"}})
			}))
			defer upstream.Close()
			router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioOpenAI, "compact-model", "native-model")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(`{"model":"compact-model","input":"history"}`)))
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.Equal(t, tc.wantCode, gjson.Get(w.Body.String(), "error.code").String())
			require.Equal(t, int32(1), calls.Load(), "native compact must not retry or switch to summary inference")
		})
	}
}

func TestCompactDispatchAndAnthropicShortCircuit(t *testing.T) {
	for _, style := range []protocol.APIStyle{protocol.APIStyleOpenAI, protocol.APIStyleAnthropic, protocol.APIStyleGoogle} {
		t.Run(string(style), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/v1/responses/compact", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(404)
				_, _ = io.WriteString(w, `{"error":{"type":"not_found_error","message":"endpoint not found"}}`)
			}))
			defer upstream.Close()
			cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()), config.WithDisableMigration(), config.WithDisableBuiltIn())
			require.NoError(t, err)
			require.NoError(t, cfg.AddProvider(&typ.Provider{UUID: "unsupported", Name: "unsupported", Enabled: true, AuthType: typ.AuthTypeAPIKey, Token: "test", APIStyle: style, APIBase: upstream.URL + "/v1", OpenAIEndpointMode: ai.EndpointModeChat}))
			require.NoError(t, cfg.AddRule(typ.Rule{Scenario: typ.ScenarioOpenAI, RequestModel: "compact-model", Active: true, Services: []*loadbalance.Service{{Provider: "unsupported", Model: "native-model", Active: true}}}))
			hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
			lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
			ph := NewHandler(ProtocolHandlerDeps{Config: cfg, ClientPool: client.NewClientPool(), RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)), LoadBalancer: lb, HealthMonitor: hm})
			router := gin.New()
			ph.RegisterRoutes(router, func(c *gin.Context) { c.Next() })
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/responses/compact", strings.NewReader(`{"model":"compact-model","input":"ordinary history"}`)))
			require.Equal(t, 400, w.Code, w.Body.String())
			require.Equal(t, "unsupported_compaction", gjson.Get(w.Body.String(), "error.code").String())
			if style == protocol.APIStyleAnthropic {
				require.Zero(t, calls.Load(), "Anthropic must fail before network dispatch")
			} else {
				require.Equal(t, int32(1), calls.Load())
			}
		})
	}
}
