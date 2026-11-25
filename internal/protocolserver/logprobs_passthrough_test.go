package protocolserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// These tests drive the real gateway routes against an httptest upstream and
// assert the two properties model fingerprinting needs from the gateway:
//
//   - a request's logprobs/top_logprobs reach the OpenAI-family upstream and
//     the resulting token probabilities come back to the client, on both the
//     same-protocol (Chat→Chat) and cross-protocol (Chat↔Responses) paths;
//   - an upstream Chat system_fingerprint survives every conversion that can
//     represent it;
//   - a request for logprobs on a provider protocol that has no such field
//     (Anthropic) fails loudly instead of being silently forwarded without it.

const logprobsFingerprint = "fp_gateway_probe_123"

// logprobsUpstream records the last request body/path and serves a canned
// response. handler decides JSON vs SSE and the payload.
type logprobsUpstream struct {
	srv   *httptest.Server
	mu    sync.Mutex
	body  string
	path  string
	calls int
}

func newLogprobsUpstream(t *testing.T, handler http.HandlerFunc) *logprobsUpstream {
	t.Helper()
	up := &logprobsUpstream{}
	up.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		up.mu.Lock()
		up.body = string(b)
		up.path = r.URL.Path
		up.calls++
		up.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(up.srv.Close)
	return up
}

func (u *logprobsUpstream) Body() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.body
}

func (u *logprobsUpstream) Path() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.path
}

func (u *logprobsUpstream) Calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
}

// newLogprobsRouter wires a real gateway stack (config, router, client pool,
// routes) around one provider and one request-model → upstream-model rule.
func newLogprobsRouter(t *testing.T, provider *typ.Provider, requestModel, upstreamModel string) *gin.Engine {
	t.Helper()
	return newLogprobsRouterForScenario(t, provider, typ.ScenarioOpenAI, requestModel, upstreamModel)
}

func newLogprobsRouterForScenario(t *testing.T, provider *typ.Provider, scenario typ.RuleScenario, requestModel, upstreamModel string) *gin.Engine {
	t.Helper()
	loadbalance.DefaultBreakerStore().Reset()

	cfg, err := config.NewConfig(
		config.WithConfigDir(t.TempDir()),
		config.WithDisableMigration(),
		config.WithDisableBuiltIn(),
	)
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(provider))
	require.NoError(t, cfg.AddRule(typ.Rule{
		Scenario: scenario, RequestModel: requestModel, Active: true,
		Services: []*loadbalance.Service{{Provider: provider.UUID, Model: upstreamModel, Active: true}},
	}))

	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{
		Config:          cfg,
		ClientPool:      client.NewClientPool(),
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)),
		LoadBalancer:    lb,
		HealthMonitor:   hm,
	})

	engine := gin.New()
	ph.RegisterRoutes(engine, func(c *gin.Context) {
		c.Set(constant.CtxKeyAuthKind, constant.AuthKindGlobalModelToken)
		c.Next()
	})
	return engine
}

func logprobsOpenAIProvider(uuid, base string, mode ai.OpenAIEndpointMode) *typ.Provider {
	return &typ.Provider{
		UUID: uuid, Name: uuid, Enabled: true,
		AuthType: typ.AuthTypeAPIKey, Token: "test-key",
		APIStyle:           protocol.APIStyleOpenAI,
		APIBase:            base + "/v1",
		APIBaseOpenAI:      base + "/v1",
		OpenAIEndpointMode: mode,
	}
}

func logprobsAnthropicProvider(uuid, base string) *typ.Provider {
	return &typ.Provider{
		UUID: uuid, Name: uuid, Enabled: true,
		AuthType: typ.AuthTypeAPIKey, Token: "test-key",
		APIStyle:         protocol.APIStyleAnthropic,
		APIBase:          base,
		APIBaseAnthropic: base,
	}
}

func logprobsPost(t *testing.T, engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Streaming handlers read the CloseNotifier off the gin writer; the shared
	// test recorder supplies it.
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	engine.ServeHTTP(w, req)
	return w.ResponseRecorder
}

// ssePayloads returns the JSON object of every data: line, skipping [DONE].
func ssePayloads(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var obj map[string]any
		require.NoErrorf(t, json.Unmarshal([]byte(data), &obj), "SSE data is not JSON: %s", data)
		out = append(out, obj)
	}
	return out
}

func firstLogprobToken(t *testing.T, logprobs any) string {
	t.Helper()
	obj, ok := logprobs.(map[string]any)
	require.True(t, ok, "logprobs must be an object, got %T", logprobs)
	content, ok := obj["content"].([]any)
	require.True(t, ok, "logprobs.content must be an array")
	require.NotEmpty(t, content, "logprobs.content must not be empty")
	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	token, _ := first["token"].(string)
	return token
}

// Chat upstream response: native logprobs plus a backend fingerprint.
const chatUpstreamResponse = `{
  "id":"chatcmpl-up","object":"chat.completion","created":1700000000,"model":"upstream-model",
  "system_fingerprint":"` + logprobsFingerprint + `",
  "choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop",
    "logprobs":{"content":[{"token":"hi","logprob":-0.1,"bytes":[104,105],"top_logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105]}]}],"refusal":null}}],
  "usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}
}`

// Responses upstream response: logprobs live on the output_text part.
const responsesUpstreamResponse = `{
  "id":"resp_up","object":"response","created_at":1700000000,"status":"completed","model":"upstream-model",
  "output":[{"id":"msg_up","type":"message","role":"assistant","status":"completed",
    "content":[{"type":"output_text","text":"hi","annotations":[],
      "logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105],"top_logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105]}]}]}]}],
  "usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
}`

func serveJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}
}

func chatSSEChunk(content, fingerprint string) string {
	return fmt.Sprintf(`{"id":"chatcmpl-up","object":"chat.completion.chunk","created":1700000000,"model":"upstream-model","system_fingerprint":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":null,"logprobs":{"content":[{"token":"hi","logprob":-0.1,"bytes":[104,105],"top_logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105]}]}],"refusal":null}}],"service_tier":"default"}`, fingerprint, content)
}

const chatSSEFinalChunk = `{"id":"chatcmpl-up","object":"chat.completion.chunk","created":1700000000,"model":"upstream-model","choices":[{"index":0,"delta":{},"finish_reason":"stop","logprobs":null}],"service_tier":"default","usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`

func serveChatSSE(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chatSSEChunk("hi", logprobsFingerprint))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chatSSEFinalChunk)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

// TestLogprobsPassthrough_ChatToChat_NonStream is the same-protocol baseline:
// the parameters go upstream untouched and both logprobs and the backend
// fingerprint come back on the wire.
func TestLogprobsPassthrough_ChatToChat_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(chatUpstreamResponse))
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-chat", up.srv.URL, ai.EndpointModeChat), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"logprobs":true,"top_logprobs":3}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "/v1/chat/completions", up.Path())
	assert.Contains(t, up.Body(), `"logprobs":true`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, logprobsFingerprint, resp["system_fingerprint"])
	choice := resp["choices"].([]any)[0].(map[string]any)
	assert.Equal(t, "hi", firstLogprobToken(t, choice["logprobs"]))
}

func TestLogprobsPassthrough_ChatToChat_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveChatSSE)
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-chat-stream", up.srv.URL, ai.EndpointModeChat), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"stream":true,"logprobs":true,"top_logprobs":3}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, up.Body(), `"logprobs":true`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var sawLogprobs, sawFingerprint bool
	for _, chunk := range ssePayloads(t, w.Body.String()) {
		if chunk["system_fingerprint"] == logprobsFingerprint {
			sawFingerprint = true
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice := choices[0].(map[string]any)
		// A Chat-side logprobs object may be present with null content (the
		// terminal chunk); only a populated content array counts as carried.
		if lp, ok := choice["logprobs"].(map[string]any); ok {
			if content, ok := lp["content"].([]any); ok && len(content) > 0 {
				if content[0].(map[string]any)["token"] == "hi" {
					sawLogprobs = true
				}
			}
		}
	}
	assert.True(t, sawLogprobs, "streaming Chat pass-through must keep choice.logprobs")
	assert.True(t, sawFingerprint, "streaming Chat pass-through must keep system_fingerprint")
}

// TestLogprobsPassthrough_ResponsesClientToChatUpstream covers the
// cross-protocol path the fingerprint work cares about: a Responses client on a
// Chat-only provider. include+top_logprobs must become Chat's logprobs+
// top_logprobs, and the Chat answer's logprobs and system_fingerprint must come
// back inside the Responses payload.
func TestLogprobsPassthrough_ResponsesClientToChatUpstream_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(chatUpstreamResponse))
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-resp-chat", up.srv.URL, ai.EndpointModeChat), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/responses",
		`{"model":"client-model","input":"hi","top_logprobs":3,"include":["message.output_text.logprobs"]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "/v1/chat/completions", up.Path())
	assert.Contains(t, up.Body(), `"logprobs":true`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, logprobsFingerprint, resp["system_fingerprint"], "Chat upstream fingerprint must survive into the Responses answer")

	item := resp["output"].([]any)[0].(map[string]any)
	part := item["content"].([]any)[0].(map[string]any)
	contentLogprobs := part["logprobs"].([]any)
	require.NotEmpty(t, contentLogprobs, "Responses output_text.logprobs must carry the upstream values")
	assert.Equal(t, "hi", contentLogprobs[0].(map[string]any)["token"])
}

func TestLogprobsPassthrough_ResponsesClientToChatUpstream_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveChatSSE)
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-resp-chat-stream", up.srv.URL, ai.EndpointModeChat), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/responses",
		`{"model":"client-model","input":"hi","stream":true,"top_logprobs":3,"include":["message.output_text.logprobs"]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, up.Body(), `"logprobs":true`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var sawLogprobs, sawFingerprint bool
	for _, evt := range ssePayloads(t, w.Body.String()) {
		if evt["type"] == "response.output_text.delta" {
			if logprobs, ok := evt["logprobs"].([]any); ok && len(logprobs) > 0 {
				if logprobs[0].(map[string]any)["token"] == "hi" {
					sawLogprobs = true
				}
			}
		}
		response, _ := evt["response"].(map[string]any)
		if response != nil && response["system_fingerprint"] == logprobsFingerprint {
			sawFingerprint = true
		}
	}
	assert.True(t, sawLogprobs, "Responses output_text.delta.logprobs must carry the upstream values")
	assert.True(t, sawFingerprint, "Chat upstream fingerprint must survive into Responses stream events")
}

// TestLogprobsPassthrough_ChatClientToResponsesUpstream covers the reverse
// cross-protocol direction: Chat's logprobs/top_logprobs become Responses'
// include+top_logprobs, and Responses output_text logprobs return as Chat
// choice.logprobs.
func TestLogprobsPassthrough_ChatClientToResponsesUpstream_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(responsesUpstreamResponse))
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-chat-resp", up.srv.URL, ai.EndpointModeResponses), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"logprobs":true,"top_logprobs":3}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "/v1/responses", up.Path())
	assert.Contains(t, up.Body(), `"message.output_text.logprobs"`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	choice := resp["choices"].([]any)[0].(map[string]any)
	assert.Equal(t, "hi", firstLogprobToken(t, choice["logprobs"]))
}

func TestLogprobsPassthrough_ChatClientToResponsesUpstream_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const created = `{"type":"response.created","sequence_number":0,"response":{"id":"resp_up","object":"response","created_at":1700000000,"status":"in_progress","model":"upstream-model","output":[]}}`
	const delta = `{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_up","output_index":0,"content_index":0,"delta":"hi","logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105],"top_logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105]}]}]}`
	const completed = `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_up","object":"response","created_at":1700000000,"status":"completed","model":"upstream-model","output":[{"id":"msg_up","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`
	up := newLogprobsUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range []struct{ name, data string }{
			{"response.created", created},
			{"response.output_text.delta", delta},
			{"response.completed", completed},
		} {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
		}
	})
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-chat-resp-stream", up.srv.URL, ai.EndpointModeResponses), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"stream":true,"logprobs":true,"top_logprobs":3}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, up.Body(), `"message.output_text.logprobs"`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var sawLogprobs bool
	for _, chunk := range ssePayloads(t, w.Body.String()) {
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice := choices[0].(map[string]any)
		// A Chat-side logprobs object may be present with null content (the
		// terminal chunk); only a populated content array counts as carried.
		if lp, ok := choice["logprobs"].(map[string]any); ok {
			if content, ok := lp["content"].([]any); ok && len(content) > 0 {
				if content[0].(map[string]any)["token"] == "hi" {
					sawLogprobs = true
				}
			}
		}
	}
	assert.True(t, sawLogprobs, "Responses output_text.delta.logprobs must return as Chat choice.logprobs")
}

// TestLogprobsPassthrough_ResponsesToResponses_NonStream is the native
// Responses path: include+top_logprobs go upstream untouched and the
// output_text logprobs come back verbatim.
func TestLogprobsPassthrough_ResponsesToResponses_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(responsesUpstreamResponse))
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-resp", up.srv.URL, ai.EndpointModeResponses), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/responses",
		`{"model":"client-model","input":"hi","top_logprobs":3,"include":["message.output_text.logprobs"]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "/v1/responses", up.Path())
	assert.Contains(t, up.Body(), `"message.output_text.logprobs"`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	item := resp["output"].([]any)[0].(map[string]any)
	part := item["content"].([]any)[0].(map[string]any)
	contentLogprobs := part["logprobs"].([]any)
	require.NotEmpty(t, contentLogprobs)
	assert.Equal(t, "hi", contentLogprobs[0].(map[string]any)["token"])
}

// responsesSSE mirrors what a Responses upstream emits for a logprobs
// request: the include entry produces logprobs on the text delta events.
const responsesSSE = `event: response.created
data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_up","object":"response","created_at":1700000000,"status":"in_progress","model":"upstream-model","output":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":1,"item_id":"msg_up","output_index":0,"content_index":0,"delta":"hi","logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105],"top_logprobs":[{"token":"hi","logprob":-0.1,"bytes":[104,105]}]}]}

event: response.completed
data: {"type":"response.completed","sequence_number":2,"response":{"id":"resp_up","object":"response","created_at":1700000000,"status":"completed","model":"upstream-model","output":[{"id":"msg_up","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}

`

func TestLogprobsPassthrough_ResponsesToResponses_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, responsesSSE)
	})
	router := newLogprobsRouter(t, logprobsOpenAIProvider("p-resp-stream", up.srv.URL, ai.EndpointModeResponses), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/responses",
		`{"model":"client-model","input":"hi","stream":true,"top_logprobs":3,"include":["message.output_text.logprobs"]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, up.Body(), `"message.output_text.logprobs"`)
	assert.Contains(t, up.Body(), `"top_logprobs":3`)

	var sawLogprobs bool
	for _, evt := range ssePayloads(t, w.Body.String()) {
		if evt["type"] != "response.output_text.delta" {
			continue
		}
		if logprobs, ok := evt["logprobs"].([]any); ok && len(logprobs) > 0 {
			if logprobs[0].(map[string]any)["token"] == "hi" {
				sawLogprobs = true
			}
		}
	}
	assert.True(t, sawLogprobs, "native Responses stream must keep output_text.delta.logprobs")
}

// TestLogprobsPassthrough_AnthropicClientToChatUpstream pins the N/A row: an
// Anthropic Messages request cannot express logprobs, so the gateway forwards
// the converted Chat request without inventing the parameters.
func TestLogprobsPassthrough_AnthropicClientToChatUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(chatUpstreamResponse))
	router := newLogprobsRouterForScenario(t, logprobsOpenAIProvider("p-anthropic-client", up.srv.URL, ai.EndpointModeChat), typ.ScenarioAnthropic, "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/anthropic/v1/messages",
		`{"model":"client-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "/v1/chat/completions", up.Path())
	assert.NotContains(t, up.Body(), "logprobs")
	assert.NotContains(t, up.Body(), "top_logprobs")
}

// TestLogprobsPassthrough_RejectsAnthropicTarget: the Anthropic Messages
// protocol has no logprobs field. A Chat request that asks for them must fail
// with an explicit 400 rather than reaching Anthropic without the parameter.
func TestLogprobsPassthrough_RejectsAnthropicTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := newLogprobsUpstream(t, serveJSON(`{"id":"msg_up","type":"message","role":"assistant","model":"upstream-model","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`))
	router := newLogprobsRouter(t, logprobsAnthropicProvider("p-anthropic", up.srv.URL), "client-model", "upstream-model")

	w := logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"logprobs":true,"top_logprobs":3}`)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "logprobs_unsupported")
	assert.Contains(t, w.Body.String(), "Anthropic Messages", "the error must name the protocol the client can act on")
	assert.Equal(t, 0, up.Calls(), "a rejected logprobs request must not reach the upstream")

	// A request that does not ask for logprobs still goes through.
	w = logprobsPost(t, router, "/tingly/openai/v1/chat/completions",
		`{"model":"client-model","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, up.Calls())

	// The Responses client gets the same explicit rejection on the same route.
	w = logprobsPost(t, router, "/tingly/openai/v1/responses",
		`{"model":"client-model","input":"hi","include":["message.output_text.logprobs"]}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "logprobs_unsupported")
	assert.Equal(t, 1, up.Calls(), "a rejected Responses logprobs request must not reach the upstream")
}
