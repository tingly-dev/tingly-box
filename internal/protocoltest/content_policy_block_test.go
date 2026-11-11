package protocoltest

// Content-policy blocks from a Codex upstream (#2417). The ChatGPT backend
// answers 200 and then refuses inside the stream with an "error" event whose
// code is cyber_policy, bio_policy or misalignment_policy_violation. The Codex
// client reports the block as an upstream 400
// (internal/client/codex_stream_errors.go), so:
//
//   - before anything reached the client, every client protocol gets a 400,
//     which clients do not retry and a multi-account rule does not fail over;
//   - a Responses stream that already started ends with an error event and
//     response.failed carrying the code, which is what Codex stops on.
//
// Any other in-stream error keeps its current handling.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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

// cpbItemDone completes the text item before the block, so a non-streaming
// assembly already has output when the error arrives.
func cpbItemDone() []string {
	return cpbEvent("response.output_item.done", `{"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"id":"msg_x","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Sure, ","annotations":[]}]}}`)
}

// cpbError is the in-stream error event the Codex backend sends: a nested
// "error" object, which openai-go alone would report as *ssestream.StreamError.
func cpbError(code string) []string { return cpbErrorWithMessage(code, cpbMessage) }

func cpbErrorWithMessage(code, message string) []string {
	return cpbEvent("error", fmt.Sprintf(`{"type":"error","sequence_number":9,"error":{"type":"invalid_request","code":%q,"message":%q,"param":null}}`, code, message))
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

// cpbClient is one client protocol: where it sends and what it sends.
type cpbClient struct {
	name   string
	source protocol.APIType
	path   string
	body   func(model string, stream bool) string
}

func cpbAnthropicBody(model string, stream bool) string {
	return fmt.Sprintf(`{"model":%q,"max_tokens":100,"stream":%v,"messages":[{"role":"user","content":"hello"}]}`, model, stream)
}

var (
	cpbResponses = cpbClient{"responses", protocol.TypeOpenAIResponses, "/tingly/openai/v1/responses", func(model string, stream bool) string {
		return fmt.Sprintf(`{"model":%q,"stream":%v,"input":"hello"}`, model, stream)
	}}
	cpbAnthropic     = cpbClient{"anthropic", protocol.TypeAnthropicV1, "/tingly/anthropic/v1/messages", cpbAnthropicBody}
	cpbAnthropicBeta = cpbClient{"anthropic_beta", protocol.TypeAnthropicBeta, "/tingly/anthropic/v1/messages?beta=true", cpbAnthropicBody}
	cpbChat          = cpbClient{"chat", protocol.TypeOpenAIChat, "/tingly/openai/v1/chat/completions", func(model string, stream bool) string {
		return fmt.Sprintf(`{"model":%q,"stream":%v,"messages":[{"role":"user","content":"hello"}]}`, model, stream)
	}}
)

type cpbResult struct {
	Status        int
	Body          string
	Names         []string // SSE event names, in order (stream responses only)
	Datas         []string // SSE data payloads, aligned with Names
	UpstreamCalls int
}

// cpbRoute routes client c to a Codex account whose upstream plays events,
// and returns the model name to request.
func cpbRoute(env *TestEnv, c cpbClient, name string, events []string) string {
	s := cpbScenario(name, events)
	env.SetupCodexAssemblyRoute(c.source, s)
	return env.findRouteModel(c.source, protocol.TypeOpenAIResponses, s.Name)
}

// cpbAddCodexAccount adds a Codex OAuth provider that points at the virtual
// upstream.
func cpbAddCodexAccount(t *testing.T, env *TestEnv, uuid string) {
	t.Helper()
	require.NoError(t, env.appConfig.AddProvider(&typ.Provider{
		UUID: uuid, Name: uuid, APIBase: env.virtual.URL() + "/v1", APIStyle: protocol.APIStyleOpenAI,
		Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
		OpenAIEndpointMode: targetToOpenAIEndpointMode(protocol.TypeOpenAIResponses),
		AuthType:           typ.AuthTypeOAuth,
		OAuthDetail:        &typ.OAuthDetail{Issuer: ai.IssuerCodex, AccessToken: "token-" + uuid},
	}))
}

func cpbPost(t *testing.T, env *TestEnv, c cpbClient, model string, stream bool) cpbResult {
	t.Helper()
	return cpbPostPath(t, env, c.path, c.body(model, stream))
}

func cpbPostPath(t *testing.T, env *TestEnv, path, body string) cpbResult {
	t.Helper()
	before := env.VirtualCallCount()
	req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.ModelToken())
	req.Header.Set("x-api-key", env.ModelToken())
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	r := cpbResult{Status: resp.StatusCode, Body: string(raw), UpstreamCalls: env.VirtualCallCount() - before}
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

// assertBlockTail checks that a Responses stream ends with the flat error event
// and a response.failed, both carrying code and the upstream's message,
// numbered on from the last forwarded event, with response.failed naming the
// upstream response.
func assertBlockTail(t *testing.T, r cpbResult, code string) {
	t.Helper()
	require.GreaterOrEqual(t, len(r.Names), 3, "body: %s", r.Body)
	n := len(r.Names)
	assert.Equal(t, []string{"error", "response.failed"}, r.Names[n-2:])
	errEvent, failed := r.Datas[n-2], r.Datas[n-1]
	prev := gjson.Get(r.Datas[n-3], "sequence_number").Int()
	assert.Equal(t, prev+1, gjson.Get(errEvent, "sequence_number").Int())
	assert.Equal(t, prev+2, gjson.Get(failed, "sequence_number").Int())
	assert.Equal(t, "resp_x", gjson.Get(failed, "response.id").String())
	assert.Equal(t, code, gjson.Get(errEvent, "code").String(), "flat Responses error event")
	assert.Equal(t, cpbMessage, gjson.Get(errEvent, "message").String())
	assert.False(t, gjson.Get(errEvent, "error").Exists(), "error event must use the flat Responses shape")
	assert.Equal(t, "failed", gjson.Get(failed, "response.status").String())
	assert.Equal(t, code, gjson.Get(failed, "response.error.code").String())
	assert.Equal(t, cpbMessage, gjson.Get(failed, "response.error.message").String())
}

// Nothing has reached the client yet: every client protocol gets a 400 that
// names the code, from one upstream call.
func TestContentPolicyBlock_BeforeOutput(t *testing.T) {
	positions := []struct {
		name   string
		stream bool
		code   string
		events []string
	}{
		{"nonstream/first_event", false, "cyber_policy", cpbError("cyber_policy")},
		{"nonstream/after_created", false, "bio_policy", cpbConcat(cpbCreated(), cpbError("bio_policy"))},
		{"nonstream/after_text", false, "misalignment_policy_violation", cpbConcat(cpbCreated(), cpbText(), cpbError("misalignment_policy_violation"))},
		{"nonstream/after_item_done", false, "cyber_policy", cpbConcat(cpbCreated(), cpbText(), cpbItemDone(), cpbError("cyber_policy"))},
		{"stream/first_event", true, "cyber_policy", cpbError("cyber_policy")},
	}
	for _, c := range []cpbClient{cpbResponses, cpbAnthropic, cpbAnthropicBeta, cpbChat} {
		for _, p := range positions {
			t.Run(c.name+"/"+p.name, func(t *testing.T) {
				t.Parallel()
				env := NewTestEnv(t)
				defer env.Close()
				model := cpbRoute(env, c, "cpb-pre-"+c.name+"-"+strings.ReplaceAll(p.name, "/", "-"), p.events)
				r := cpbPost(t, env, c, model, p.stream)
				assert.Equal(t, http.StatusBadRequest, r.Status, "body: %s", r.Body)
				assert.Equal(t, 1, r.UpstreamCalls)
				assert.Contains(t, r.Body, "code="+p.code)
				assert.Contains(t, r.Body, cpbMessage)
				assert.NotContains(t, r.Body, env.virtual.URL(), "no upstream address in the client response")
			})
		}
	}
}

// A Responses stream that already started keeps what was sent, then ends with
// an error event and response.failed carrying the code: Codex stops on
// response.failed, and retries a stream that has no terminal event.
func TestContentPolicyBlock_ResponsesMidStream(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		events []string
	}{
		{"after_created", "misalignment_policy_violation", cpbConcat(cpbCreated(), cpbError("misalignment_policy_violation"))},
		{"after_text", "bio_policy", cpbConcat(cpbCreated(), cpbText(), cpbError("bio_policy"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			model := cpbRoute(env, cpbResponses, "cpb-mid-"+tc.name, tc.events)
			r := cpbPost(t, env, cpbResponses, model, true)
			assert.Equal(t, http.StatusOK, r.Status)
			assert.Equal(t, 1, r.UpstreamCalls)
			assert.Equal(t, "response.created", r.Names[0], "what was sent stays sent")
			assertBlockTail(t, r, tc.code)
			assert.NotContains(t, r.Body, "stream_failed")
		})
	}
}

// gapCPBAnthropicMidStream: the Anthropic stream converters end a failed
// stream with a generic error event and do not read the error, so a block
// that arrives after streaming started reaches the client without its code.
var gapCPBAnthropicMidStream = KnownGap{
	ID:     "CPB1",
	Reason: "an Anthropic stream that already started ends with a generic error event; the content-policy code reaches neither the client nor the usage record or server log (#2417)",
}

var _ = registerKnownGaps(gapCPBAnthropicMidStream,
	"TestContentPolicyBlock_MidStreamOtherClients/anthropic",
	"TestContentPolicyBlock_MidStreamOtherClients/anthropic_beta")

// A stream that already started cannot change its status. The client should
// still learn the code from the stream's final event.
func TestContentPolicyBlock_MidStreamOtherClients(t *testing.T) {
	for _, c := range []cpbClient{cpbAnthropic, cpbAnthropicBeta, cpbChat} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			model := cpbRoute(env, c, "cpb-midx-"+c.name, cpbConcat(cpbCreated(), cpbText(), cpbError("cyber_policy")))
			r := cpbPost(t, env, c, model, true)
			var failures []string
			if r.Status != http.StatusOK {
				failures = append(failures, fmt.Sprintf("status = %d", r.Status))
			}
			// "code=cyber_policy" is how the gateway names the code; the raw
			// upstream JSON that a plain stream error relays does not match.
			if n := len(r.Datas); n == 0 || !strings.Contains(r.Datas[n-1], "code=cyber_policy") {
				failures = append(failures, "the stream's final event does not carry the code")
			}
			checkCase(t, t.Name(), failures, "client response:\n"+r.Body)
		})
	}
}

// Other in-stream errors keep their handling: a retryable 5xx before output,
// stream_failed after it. The exact 5xx is not this change's concern.
func TestContentPolicyBlock_OtherErrorsUnchanged(t *testing.T) {
	retryable := func(t *testing.T, r cpbResult) {
		assert.GreaterOrEqual(t, r.Status, http.StatusInternalServerError, "body: %s", r.Body)
	}
	cases := []struct {
		name   string
		client cpbClient
		stream bool
		events []string
		check  func(t *testing.T, r cpbResult)
	}{
		{"responses/nonstream/server_error", cpbResponses, false, cpbError("server_error"), retryable},
		{"responses/stream/first_event/server_error", cpbResponses, true, cpbError("server_error"), retryable},
		{"responses/stream/first_event/invalid_prompt", cpbResponses, true, cpbError("invalid_prompt"), retryable},
		// Passes the recognizer's "_policy" prefilter; only the code allowlist
		// keeps it retryable.
		{"responses/stream/first_event/server_error_policy_text", cpbResponses, true, cpbErrorWithMessage("server_error", "see usage_policy"), retryable},
		{"responses/stream/after_text/server_error", cpbResponses, true, cpbConcat(cpbCreated(), cpbText(), cpbError("server_error")), func(t *testing.T, r cpbResult) {
			require.NotEmpty(t, r.Names)
			assert.Equal(t, "error", r.Names[len(r.Names)-1])
			assert.Equal(t, "stream_failed", gjson.Get(r.Datas[len(r.Datas)-1], "code").String())
			assert.NotContains(t, r.Names, "response.failed")
		}},
		{"anthropic/nonstream/server_error", cpbAnthropic, false, cpbError("server_error"), retryable},
		{"chat/nonstream/server_error", cpbChat, false, cpbError("server_error"), retryable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			model := cpbRoute(env, tc.client, "cpb-other-"+strings.ReplaceAll(tc.name, "/", "-"), tc.events)
			tc.check(t, cpbPost(t, env, tc.client, model, tc.stream))
		})
	}
}

// An upstream response.failed already ends the stream with its code; it passes
// through as sent, with nothing added.
func TestContentPolicyBlock_UpstreamResponseFailedPassesThrough(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Close()
	model := cpbRoute(env, cpbResponses, "cpb-upstream-failed", cpbConcat(cpbCreated(), cpbUpstreamFailed("cyber_policy")))
	r := cpbPost(t, env, cpbResponses, model, true)
	require.NotEmpty(t, r.Names)
	assert.Equal(t, "response.failed", r.Names[len(r.Names)-1])
	assert.Equal(t, "resp_x", gjson.Get(r.Datas[len(r.Datas)-1], "response.id").String(), "the upstream's own event")
	assert.NotContains(t, r.Names, "error")
}

// Two Codex accounts in one tiered rule. A blocked request reaches one account
// and records no breaker failure; a transient error still fails over.
func TestContentPolicyBlock_NoFailover(t *testing.T) {
	for _, c := range []cpbClient{cpbResponses, cpbAnthropic, cpbChat} {
		for _, code := range []string{"cyber_policy", "server_error"} {
			for _, stream := range []bool{false, true} {
				// Unique names: the breaker store is process-global.
				name := fmt.Sprintf("cpb-two-%s-%s-%v", c.name, code, stream)
				t.Run(fmt.Sprintf("%s/%s/stream=%v", c.name, code, stream), func(t *testing.T) {
					t.Parallel()
					env := NewTestEnv(t)
					defer env.Close()
					s := cpbScenario(name, cpbError(code))
					env.virtual.RegisterScenario(s)
					upstreamModel := "virtual-model-" + s.Name
					accounts := [2]string{name + "-a", name + "-b"}
					for _, uuid := range accounts {
						cpbAddCodexAccount(t, env, uuid)
					}
					rule := newHarnessRule(name, sourceToRuleScenario(c.source), name, upstreamModel,
						tieredService(accounts[0], upstreamModel, 0), tieredService(accounts[1], upstreamModel, 1))
					rule.LBTactic = tierFailoverTactic()
					require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

					calls := 0
					var last cpbResult
					for range 3 {
						last = cpbPost(t, env, c, name, stream)
						calls += last.UpstreamCalls
					}
					store := loadbalance.DefaultBreakerStore()
					a := store.Get(rule.UUID, loadbalance.FormatServiceID(accounts[0], upstreamModel)).State()
					b := store.Get(rule.UUID, loadbalance.FormatServiceID(accounts[1], upstreamModel)).State()

					if code == "cyber_policy" {
						assert.Equal(t, 3, calls, "each blocked request reaches exactly one account")
						assert.Equal(t, http.StatusBadRequest, last.Status)
						assert.Equal(t, loadbalance.BreakerClosed, a)
						assert.Equal(t, loadbalance.BreakerClosed, b)
					} else {
						assert.Equal(t, 6, calls, "a transient error still fails over to the second account")
						assert.GreaterOrEqual(t, last.Status, http.StatusInternalServerError)
					}
				})
			}
		}
	}
}

// A block is final within the rule, including a tier on another vendor: the
// block reaches the client rather than an answer from a different model.
// Falling back after a block would be a routing choice for the rule to state
// explicitly; it is not made implicitly here.
func TestContentPolicyBlock_NoCrossVendorFallback(t *testing.T) {
	for _, c := range []cpbClient{cpbAnthropic, cpbChat} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			name := "cpb-xv-" + c.name
			blocked := cpbScenario(name, cpbError("cyber_policy"))
			env.virtual.RegisterScenario(blocked)
			answering := scenario.StreamingTextScenario()
			env.virtual.RegisterScenario(answering)
			codexUUID, otherUUID := name+"-codex", name+"-other"
			cpbAddCodexAccount(t, env, codexUUID)
			require.NoError(t, env.appConfig.AddProvider(&typ.Provider{
				UUID: otherUUID, Name: otherUUID, APIBase: env.virtual.URL() + "/v1", APIStyle: protocol.APIStyleOpenAI,
				Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
				OpenAIEndpointMode: targetToOpenAIEndpointMode(protocol.TypeOpenAIChat), Token: "virtual-token",
			}))
			blockedModel, answeringModel := "virtual-model-"+blocked.Name, "virtual-model-"+answering.Name
			rule := newHarnessRule(name, sourceToRuleScenario(c.source), name, answeringModel,
				tieredService(codexUUID, blockedModel, 0), tieredService(otherUUID, answeringModel, 1))
			rule.LBTactic = tierFailoverTactic()
			require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

			r := cpbPost(t, env, c, name, false)
			assert.Equal(t, http.StatusBadRequest, r.Status, "body: %s", r.Body)
			assert.Equal(t, 1, r.UpstreamCalls, "the second tier is not asked")
		})
	}
}

// Image generation on a Codex rule runs over a Responses stream as well.
func TestContentPolicyBlock_ImageGeneration(t *testing.T) {
	for _, code := range []string{"cyber_policy", "server_error"} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			name := "cpb-img-" + code
			s := cpbScenario(name, cpbConcat(cpbCreated(), cpbError(code)))
			env.virtual.RegisterScenario(s)
			upstreamModel := "virtual-model-" + s.Name
			cpbAddCodexAccount(t, env, name)
			rule := newHarnessRule(name, typ.ScenarioImageGen, name, upstreamModel, harnessService(name, upstreamModel))
			require.NoError(t, env.appConfig.GetGlobalConfig().AddRequestConfig(rule))

			r := cpbPostPath(t, env, "/tingly/imagegen/v1/images/generations", fmt.Sprintf(`{"model":%q,"prompt":"a cat"}`, name))
			if code == "cyber_policy" {
				assert.Equal(t, http.StatusBadRequest, r.Status, "body: %s", r.Body)
				assert.Contains(t, r.Body, "code=cyber_policy")
			} else {
				assert.GreaterOrEqual(t, r.Status, http.StatusInternalServerError, "body: %s", r.Body)
			}
		})
	}
}

// The usage record names the code and leaves the upstream message out.
func TestContentPolicyBlock_UsageRecord(t *testing.T) {
	cases := []struct {
		name   string
		client cpbClient
		stream bool
		events []string
	}{
		{"anthropic/nonstream", cpbAnthropic, false, cpbError("cyber_policy")},
		{"responses/stream/after_text", cpbResponses, true, cpbConcat(cpbCreated(), cpbText(), cpbError("cyber_policy"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			model := cpbRoute(env, tc.client, "cpb-usage-"+strings.ReplaceAll(tc.name, "/", "-"), tc.events)
			cpbPost(t, env, tc.client, model, tc.stream)

			store := env.appConfig.GetGlobalConfig().StoreManager()
			require.NotNil(t, store)
			records, _, err := store.Usage().GetRecords(time.Time{}, time.Now().Add(time.Hour), nil, 10, 0)
			require.NoError(t, err)
			require.Len(t, records, 1)
			assert.Equal(t, "error", records[0].Status)
			assert.Contains(t, records[0].ErrorCode, "code=cyber_policy")
			assert.NotContains(t, records[0].ErrorCode, cpbMessage)
		})
	}
}

// What the server log keeps (param, other fields of the upstream error object)
// does not reach the client.
func TestContentPolicyBlock_NoUpstreamDetailLeak(t *testing.T) {
	leaky := cpbEvent("error", `{"type":"error","sequence_number":9,"error":{"type":"invalid_request","code":"cyber_policy","message":"flagged","param":"input.SECRET_PARAM","misalignment":{"category":"SECRET_EXTRA"}}}`)
	for _, c := range []cpbClient{cpbResponses, cpbAnthropic, cpbChat} {
		for _, midStream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mid_stream=%v", c.name, midStream), func(t *testing.T) {
				t.Parallel()
				env := NewTestEnv(t)
				defer env.Close()
				events, stream := leaky, false
				if midStream {
					events, stream = cpbConcat(cpbCreated(), cpbText(), leaky), true
				}
				model := cpbRoute(env, c, fmt.Sprintf("cpb-leak-%s-%v", c.name, midStream), events)
				r := cpbPost(t, env, c, model, stream)
				assert.NotContains(t, r.Body, "SECRET_PARAM")
				assert.NotContains(t, r.Body, "SECRET_EXTRA")
			})
		}
	}
}

// gapCPBNonCodexUpstream: the recognition lives in the Codex client, so a
// Responses upstream reached through the generic OpenAI client (for example a
// third-party relay of the same backend) still reports a block as an
// upstream failure.
var gapCPBNonCodexUpstream = KnownGap{
	ID:     "CPB2",
	Reason: "a content-policy block from a Responses upstream other than Codex OAuth is not recognized (#2417)",
}

var _ = registerKnownGaps(gapCPBNonCodexUpstream,
	"TestContentPolicyBlock_NonCodexUpstream/first_event",
	"TestContentPolicyBlock_NonCodexUpstream/after_created")

func TestContentPolicyBlock_NonCodexUpstream(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		want   func(r cpbResult) []string
	}{
		{"first_event", cpbError("cyber_policy"), func(r cpbResult) []string {
			if r.Status != http.StatusBadRequest {
				return []string{fmt.Sprintf("status = %d, want 400", r.Status)}
			}
			return nil
		}},
		{"after_created", cpbConcat(cpbCreated(), cpbError("cyber_policy")), func(r cpbResult) []string {
			if n := len(r.Names); n == 0 || r.Names[n-1] != "response.failed" {
				return []string{"the stream does not end with response.failed"}
			}
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			s := cpbScenario("cpb-noncodex-"+tc.name, tc.events)
			env.SetupRoute(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s)
			model := env.findRouteModel(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s.Name)
			r := cpbPost(t, env, cpbResponses, model, true)
			checkCase(t, t.Name(), tc.want(r), "client response:\n"+r.Body)
		})
	}
}
