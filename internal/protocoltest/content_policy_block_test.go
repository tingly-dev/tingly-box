package protocoltest

// Content-policy blocks on the OpenAI Responses passthrough (#2417): a Codex
// OAuth upstream answers 200 and then refuses inside the stream with an
// "error" event carrying code cyber_policy / bio_policy /
// misalignment_policy_violation. The client must see a non-retryable answer
// with that code, and a multi-account rule must not re-send the request to
// another account. Any other in-stream error keeps its current handling.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

const cpbMessage = "This content was flagged for possible cybersecurity risk."

// cpbEvent is one SSE event as the upstream writes it.
func cpbEvent(name, data string) []string { return []string{"event: " + name, "data: " + data} }

func cpbCreated() []string {
	r := `{"id":"resp_x","object":"response","created_at":1700000000,"status":"in_progress","model":"m","output":[]}`
	out := cpbEvent("response.created", `{"type":"response.created","sequence_number":0,"response":`+r+`}`)
	return append(out, cpbEvent("response.in_progress", `{"type":"response.in_progress","sequence_number":1,"response":`+r+`}`)...)
}

func cpbText() []string {
	out := cpbEvent("response.output_item.added", `{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"msg_x","type":"message","status":"in_progress","role":"assistant","content":[]}}`)
	out = append(out, cpbEvent("response.content_part.added", `{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_x","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`)...)
	return append(out, cpbEvent("response.output_text.delta", `{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_x","output_index":0,"content_index":0,"delta":"Sure, "}`)...)
}

// cpbError is the in-stream error event the Codex backend sends: a nested
// "error" object, which openai-go turns into *ssestream.StreamError.
func cpbError(code string) []string {
	return cpbEvent("error", fmt.Sprintf(`{"type":"error","sequence_number":9,"error":{"type":"invalid_request","code":%q,"message":%q,"param":null}}`, code, cpbMessage))
}

func cpbUpstreamFailed(code string) []string {
	return cpbEvent("response.failed", fmt.Sprintf(`{"type":"response.failed","sequence_number":9,"response":{"id":"resp_x","object":"response","created_at":1700000000,"status":"failed","model":"m","output":[],"error":{"code":%q,"message":%q}}}`, code, cpbMessage))
}

func cpbConcat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func cpbScenario(name string, events []string) Scenario {
	return Scenario{Name: name, MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{
		scenario.FormatOpenAIResponses: {Stream: func() []string { return events }},
	}}
}

type cpbResult struct {
	Status        int
	Header        http.Header
	Body          string
	Names         []string // SSE event names, in order (stream responses only)
	Datas         []string // SSE data payloads, aligned with Names
	UpstreamCalls int
}

// cpbPost sends a Responses request and keeps the response headers, which
// the shared send helpers drop.
func cpbPost(t *testing.T, env *TestEnv, model string, stream bool) cpbResult {
	t.Helper()
	before := env.VirtualCallCount()
	body := fmt.Sprintf(`{"model":%q,"stream":%v,"input":"hello"}`, model, stream)
	req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+"/tingly/openai/v1/responses", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.ModelToken())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	r := cpbResult{Status: resp.StatusCode, Header: resp.Header, Body: string(raw), UpstreamCalls: env.VirtualCallCount() - before}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		r.Names, r.Datas = cpbParseSSE(r.Body)
	}
	return r
}

func cpbParseSSE(body string) (names, datas []string) {
	for _, block := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var name, data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		names = append(names, name)
		datas = append(datas, data)
	}
	return names, datas
}

// assertBlockTail checks that a stream ends with the flat error event and a
// response.failed, both carrying code, and that response.failed is numbered
// after the upstream's error event (sequence_number 9).
func assertBlockTail(t *testing.T, r cpbResult, code string) {
	t.Helper()
	require.GreaterOrEqual(t, len(r.Names), 2, "body: %s", r.Body)
	n := len(r.Names)
	assert.Equal(t, []string{"error", "response.failed"}, r.Names[n-2:])
	errEvent, failed := r.Datas[n-2], r.Datas[n-1]
	assert.Equal(t, code, gjson.Get(errEvent, "code").String(), "flat Responses error event")
	assert.Equal(t, cpbMessage, gjson.Get(errEvent, "message").String())
	assert.False(t, gjson.Get(errEvent, "error").Exists(), "error event must use the flat Responses shape")
	assert.Equal(t, "failed", gjson.Get(failed, "response.status").String())
	assert.Equal(t, code, gjson.Get(failed, "response.error.code").String())
	assert.Equal(t, cpbMessage, gjson.Get(failed, "response.error.message").String())
	assert.Greater(t, gjson.Get(failed, "sequence_number").Int(), gjson.Get(errEvent, "sequence_number").Int())
}

func TestContentPolicyBlock_ResponsesPassthrough(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		stream bool
		check  func(t *testing.T, r cpbResult)
	}{
		{"stream_first_event_cyber", cpbError("cyber_policy"), true, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusBadRequest, r.Status, "nothing was sent yet: answer with a non-retryable status")
			assert.Equal(t, "false", r.Header.Get("x-should-retry"))
			assert.Equal(t, "cyber_policy", gjson.Get(r.Body, "error.code").String())
		}},
		{"stream_after_text_bio", cpbConcat(cpbCreated(), cpbText(), cpbError("bio_policy")), true, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusOK, r.Status)
			assert.Contains(t, r.Names, "response.output_text.delta", "output already sent stays sent")
			assertBlockTail(t, r, "bio_policy")
			assert.NotContains(t, r.Body, "stream_failed")
		}},
		{"stream_after_created_misalignment", cpbConcat(cpbCreated(), cpbError("misalignment_policy_violation")), true, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusOK, r.Status)
			assertBlockTail(t, r, "misalignment_policy_violation")
		}},
		{"nonstream_cyber", cpbError("cyber_policy"), false, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusBadRequest, r.Status)
			assert.Equal(t, "false", r.Header.Get("x-should-retry"))
			assert.Equal(t, "cyber_policy", gjson.Get(r.Body, "error.code").String())
			assert.Equal(t, "invalid_request", gjson.Get(r.Body, "error.type").String())
			assert.Equal(t, cpbMessage, gjson.Get(r.Body, "error.message").String())
			assert.False(t, gjson.Get(r.Body, "error.param").Exists(), "param is not relayed to clients")
		}},

		// Not a content-policy block: handling is unchanged.
		{"stream_first_event_server_error", cpbError("server_error"), true, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusInternalServerError, r.Status)
			assert.Empty(t, r.Header.Get("x-should-retry"))
		}},
		{"stream_after_text_server_error", cpbConcat(cpbCreated(), cpbText(), cpbError("server_error")), true, func(t *testing.T, r cpbResult) {
			require.NotEmpty(t, r.Names)
			assert.Equal(t, "error", r.Names[len(r.Names)-1])
			assert.Equal(t, "stream_failed", gjson.Get(r.Datas[len(r.Datas)-1], "code").String())
			assert.NotContains(t, r.Names, "response.failed")
		}},
		{"nonstream_server_error", cpbError("server_error"), false, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusInternalServerError, r.Status)
		}},
		{"stream_first_event_invalid_prompt", cpbError("invalid_prompt"), true, func(t *testing.T, r cpbResult) {
			assert.Equal(t, http.StatusInternalServerError, r.Status, "only the allowlisted policy codes change")
		}},
		{"stream_upstream_response_failed", cpbConcat(cpbCreated(), cpbUpstreamFailed("cyber_policy")), true, func(t *testing.T, r cpbResult) {
			failed := 0
			for _, name := range r.Names {
				if name == "response.failed" {
					failed++
				}
			}
			assert.Equal(t, 1, failed, "an upstream response.failed passes through once, nothing added")
			assert.NotContains(t, r.Names, "error")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnv(t)
			defer env.Close()
			s := cpbScenario("cpb-"+tc.name, tc.events)
			env.SetupCodexAssemblyRoute(protocol.TypeOpenAIResponses, s)
			model := env.findRouteModel(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s.Name)
			r := cpbPost(t, env, model, tc.stream)
			assert.Equal(t, 1, r.UpstreamCalls)
			assert.NotContains(t, r.Body, env.virtual.URL(), "no upstream address in the client response")
			tc.check(t, r)
		})
	}
}

// TestContentPolicyBlock_NoFailover: two Codex accounts in one tiered rule.
// A blocked request reaches one account only and does not count as a breaker
// failure; a transient error still fails over as before.
func TestContentPolicyBlock_NoFailover(t *testing.T) {
	for _, code := range []string{"cyber_policy", "server_error"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", code, stream), func(t *testing.T) {
				env := NewTestEnv(t)
				defer env.Close()

				// Unique names: the breaker store is process-global.
				name := fmt.Sprintf("cpb-two-%s-%v", code, stream)
				s := cpbScenario(name, cpbError(code))
				env.virtual.RegisterScenario(s)
				upstreamModel := "virtual-model-" + s.Name
				accounts := [2]string{name + "-a", name + "-b"}
				for _, uuid := range accounts {
					require.NoError(t, env.appConfig.AddProvider(&typ.Provider{
						UUID: uuid, Name: uuid, APIBase: env.virtual.URL() + "/v1", APIStyle: protocol.APIStyleOpenAI,
						Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
						OpenAIEndpointMode: targetToOpenAIEndpointMode(protocol.TypeOpenAIResponses),
						AuthType:           typ.AuthTypeOAuth,
						OAuthDetail:        &typ.OAuthDetail{Issuer: ai.IssuerCodex, AccessToken: "token-" + uuid},
					}))
				}
				requestModel := name
				rule := newHarnessRule(requestModel, typ.ScenarioOpenAI, requestModel, upstreamModel,
					tieredService(accounts[0], upstreamModel, 0), tieredService(accounts[1], upstreamModel, 1))
				rule.LBTactic = tierFailoverTactic()
				require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

				calls := 0
				var last cpbResult
				for range 3 {
					last = cpbPost(t, env, requestModel, stream)
					calls += last.UpstreamCalls
				}
				store := loadbalance.DefaultBreakerStore()
				a := store.Get(rule.UUID, loadbalance.FormatServiceID(accounts[0], upstreamModel)).State()
				b := store.Get(rule.UUID, loadbalance.FormatServiceID(accounts[1], upstreamModel)).State()

				if code == "cyber_policy" {
					assert.Equal(t, 3, calls, "each blocked request reaches exactly one account")
					assert.Equal(t, http.StatusBadRequest, last.Status)
					assert.Equal(t, "false", last.Header.Get("x-should-retry"))
					assert.Equal(t, loadbalance.BreakerClosed, a)
					assert.Equal(t, loadbalance.BreakerClosed, b)
				} else {
					assert.Equal(t, 6, calls, "a transient error still fails over to the second account")
					assert.Equal(t, http.StatusInternalServerError, last.Status)
				}
			})
		}
	}
}
