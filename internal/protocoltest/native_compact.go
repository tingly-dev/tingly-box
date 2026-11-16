package protocoltest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type nativeCompactCase struct {
	name       string
	target     protocol.APIType
	status     int
	code       string
	wantStatus int
	wantCode   string
	malformed  bool
}

func nativeCompactCases() []recorderCase {
	specs := []nativeCompactCase{
		{name: "chat_configured_native_success", target: protocol.TypeOpenAIChat, status: 200, wantStatus: 200},
		{name: "native_compact_to_continue", target: protocol.TypeOpenAIResponses, status: 200, wantStatus: 200},
		{name: "malformed_success", target: protocol.TypeOpenAIResponses, status: 200, wantStatus: 502, malformed: true},
		{name: "chat_unsupported", target: protocol.TypeOpenAIChat, status: 404, wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "anthropic_unsupported", target: protocol.TypeAnthropicBeta, status: 404, wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "google_unsupported", target: protocol.TypeGoogle, status: 404, wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "missing_endpoint", target: protocol.TypeOpenAIResponses, status: 404, wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "not_implemented", target: protocol.TypeOpenAIResponses, status: 501, wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "unsupported_operation", target: protocol.TypeOpenAIResponses, status: 400, code: "unsupported_operation", wantStatus: 400, wantCode: "unsupported_compaction"},
		{name: "model_not_found", target: protocol.TypeOpenAIResponses, status: 404, code: "model_not_found", wantStatus: 404, wantCode: "model_not_found"},
		{name: "unauthorized", target: protocol.TypeOpenAIResponses, status: 401, code: "invalid_api_key", wantStatus: 401, wantCode: "invalid_api_key"},
		{name: "forbidden", target: protocol.TypeOpenAIResponses, status: 403, code: "permission_denied", wantStatus: 403, wantCode: "permission_denied"},
		{name: "rate_limit", target: protocol.TypeOpenAIResponses, status: 429, code: "rate_limit_exceeded", wantStatus: 429, wantCode: "rate_limit_exceeded"},
		{name: "protection_error", target: protocol.TypeOpenAIResponses, status: 400, code: "response_protection_error", wantStatus: 400, wantCode: "response_protection_error"},
		{name: "upstream_failure", target: protocol.TypeOpenAIResponses, status: 503, code: "server_error", wantStatus: 503, wantCode: "server_error"},
	}
	var cases []recorderCase
	for _, spec := range specs {
		cases = append(cases, recorderCase{name: "native_compact/" + spec.name, scenario: spec.name, source: protocol.TypeOpenAIResponses, target: spec.target, run: func(t flagTB, env *TestEnv) { runNativeCompact(t, env, spec) }})
	}
	return cases
}

func runNativeCompact(t flagTB, env *TestEnv, spec nativeCompactCase) {
	t.Helper()
	const window = `[{"type":"message","role":"user","content":"retained"},{"type":"compaction","id":"cmp_1","encrypted_content":"opaque-provider-state","future":{"counter":9007199254740993}}]`
	const result = `{"id":"cmp_response","object":"response.compaction","output":` + window + `,"usage":{"input_tokens":10,"output_tokens":2}}`
	var compactCalls, continueCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if (spec.target == protocol.TypeAnthropicBeta && r.Header.Get("x-api-key") != "virtual-token") || (spec.target == protocol.TypeGoogle && r.Header.Get("x-goog-api-key") != "virtual-token") || (spec.target != protocol.TypeAnthropicBeta && spec.target != protocol.TypeGoogle && r.Header.Get("Authorization") != "Bearer virtual-token") {
			t.Errorf("missing provider authentication")
		}
		switch r.URL.Path {
		case "/v1/responses/compact":
			compactCalls.Add(1)
			if gjson.GetBytes(body, "model").String() != "virtual-model-nativecompact" {
				t.Errorf("model alias not resolved: %s", body)
			}
			if gjson.GetBytes(body, "input.0.encrypted_content").String() != "opaque-history" {
				t.Errorf("native history lost: %s", body)
			}
			if gjson.GetBytes(body, "input.0.future").Raw != "9007199254740993" {
				t.Errorf("opaque number changed: %s", body)
			}
			if gjson.GetBytes(body, "stream").Exists() || gjson.GetBytes(body, "store").Exists() {
				t.Errorf("create-only defaults added to compact: %s", body)
			}
			w.WriteHeader(spec.status)
			if spec.status != 200 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": spec.code, "message": "fixture failure"}})
				return
			}
			if spec.malformed {
				_, _ = io.WriteString(w, `{"output":null}`)
				return
			}
			_, _ = io.WriteString(w, result)
		case "/v1/responses":
			continueCalls.Add(1)
			if gjson.GetBytes(body, "input.1.encrypted_content").String() != "opaque-provider-state" || gjson.GetBytes(body, "input.1.future.counter").Raw != "9007199254740993" {
				t.Errorf("compact state changed on continuation: %s", body)
			}
			_, _ = io.WriteString(w, `{"id":"resp_continue","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"continued"}]}],"usage":{"input_tokens":10,"output_tokens":2}}`)
		default:
			t.Errorf("unexpected provider endpoint: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer upstream.Close()
	fixture := TextScenario()
	fixture.Name = "nativecompact"
	env.setupRouteCore(protocol.TypeOpenAIResponses, spec.target, fixture, nil, func(p *typ.Provider) { p.APIBase = upstream.URL + "/v1" })
	model := env.findRouteModel(protocol.TypeOpenAIResponses, spec.target, fixture.Name)
	body := []byte(fmt.Sprintf(`{"model":%q,"input":[{"type":"reasoning","encrypted_content":"opaque-history","future":9007199254740993}],"instructions":"keep facts"}`, model))
	response, err := env.dispatch(protocol.TypeOpenAIResponses, spec.target, fixture.Name, "/tingly/openai/v1/responses/compact", body, nil, false)
	if err != nil {
		t.Fatalf("compact request: %v", err)
	}
	if response.HTTPStatus != spec.wantStatus {
		t.Fatalf("compact HTTP %d, want %d: %s", response.HTTPStatus, spec.wantStatus, response.RawBody)
	}
	if spec.wantCode != "" && gjson.GetBytes(response.RawBody, "error.code").String() != spec.wantCode {
		t.Errorf("error code: %s, want %s", response.RawBody, spec.wantCode)
	}
	wantCalls := int32(1)
	if spec.target == protocol.TypeAnthropicBeta {
		wantCalls = 0
	}
	if compactCalls.Load() != wantCalls {
		t.Errorf("compact calls=%d, want %d", compactCalls.Load(), wantCalls)
	}
	if spec.wantStatus != 200 {
		if continueCalls.Load() != 0 {
			t.Errorf("failure triggered inference")
		}
		return
	}
	if string(response.RawBody) != result {
		t.Errorf("compact window was rewritten: %s", response.RawBody)
	}
	if spec.target != protocol.TypeOpenAIResponses {
		return
	} // Continuation requires a native Responses inference route.
	replay := fmt.Sprintf(`{"model":%q,"input":%s}`, model, strings.TrimSuffix(window, "]")+`,{"role":"user","content":"continue"}]`)
	next, err := env.dispatch(protocol.TypeOpenAIResponses, spec.target, fixture.Name, "/tingly/openai/v1/responses", []byte(replay), nil, false)
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if next.HTTPStatus != 200 || !strings.Contains(string(next.RawBody), "continued") {
		t.Errorf("continuation failed: HTTP %d: %s", next.HTTPStatus, next.RawBody)
	}
	if continueCalls.Load() != 1 {
		t.Errorf("continuation calls=%d, want 1", continueCalls.Load())
	}
}

func (m *Matrix) ExecuteAllNativeCompact() []TestResult {
	return m.runRecorderCases(nativeCompactCases())
}
