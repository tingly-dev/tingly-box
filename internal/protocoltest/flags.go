package protocoltest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the rule-flag behavior suite, shared by both the go-test entry
// point (TestRuleFlags) and the CLI matrix (`harness matrix --mode=flags`). It
// is deliberately separate from the protocol matrix (which stays flag-free): the
// matrix tests conversion fidelity, while this suite drives one request per flag
// through the REAL gateway with rule.Flags set and asserts the observable effect
// on either the upstream request the provider received (CapturedRequest) or the
// client response. Each case maps to exactly one key in typ.RuleFlagRegistry().
//
// See .design/rule-flag-testing.md.

// flagTB is the tiny subset of testing.TB the flag cases use, so the same case
// bodies run under `go test` (a *testing.T) and the CLI (a recording shim).
type flagTB interface {
	Helper()
	Cleanup(func())
	Errorf(format string, args ...any)
	Error(args ...any)
	Fatalf(format string, args ...any)
	Fatal(args ...any)
}

// flagCase is one rule-flag behavior test. run performs the full setup → send →
// assert against a fresh gateway env.
type flagCase struct {
	key string // must equal a typ.RuleFlagRegistry() key
	run func(t flagTB, env *TestEnv)
}

// flagScenario is the single shared scenario every flag case routes through.
// One representative fixture is enough: rather than a trivial single-turn text
// exchange, it serves the multi-turn mocks (which advertise a usage block, so
// skip_usage has something to strip) paired with flagBaseRequest below. It
// carries no assertions of its own — the flag suite asserts directly on the
// captured upstream request / client response.
func flagScenario() Scenario {
	s := MultiTurnScenario()
	s.Name = "flags"
	s.Description = "Unified multi-turn fixture for rule-flag behavior tests"
	s.Assertions = nil
	return s
}

// flagBaseRequest is the unified inbound request the flag suite sends: one
// representative, multi-turn conversation that bakes in the material the various
// flags act on, so individual cases set only their flag and assert their slice
// rather than each crafting a bespoke request.
//
//   - OpenAI: a system turn + user/assistant history, array-shaped user content
//     (cursor flattening), a tool list (block_tools), and max_tokens
//     (max_tokens→max_completion_tokens rewrite).
//   - Anthropic: a system block carrying an injected billing header
//     (clean_header) plus a normal block, and a multi-turn message history
//     (thinking budget, etc).
//
// Flags whose test is inherently about a field/shape swap (use_max_tokens,
// claude_code_compat) still pass a small mutate to sendFlag.
func flagBaseRequest(source protocol.APIType, model string, streaming bool) (string, []byte) {
	switch source {
	case protocol.TypeOpenAIChat:
		return "/tingly/openai/v1/chat/completions", mustMarshal(map[string]any{
			"model":      model,
			"max_tokens": 64,
			"stream":     streaming,
			"messages": []map[string]any{
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": []map[string]any{
					{"type": "text", "text": "Hello"},
					{"type": "text", "text": " world"},
				}},
				{"role": "assistant", "content": "Hi — how can I help?"},
				{"role": "user", "content": []map[string]any{
					{"type": "text", "text": "What is the capital of France?"},
				}},
			},
			"tools": []map[string]any{
				{"type": "function", "function": map[string]any{"name": "web_search", "parameters": map[string]any{"type": "object"}}},
				{"type": "function", "function": map[string]any{"name": "keep_me", "parameters": map[string]any{"type": "object"}}},
			},
		})
	case protocol.TypeAnthropicV1:
		return "/tingly/anthropic/v1/messages", mustMarshal(map[string]any{
			"model":      model,
			"max_tokens": 64,
			"stream":     streaming,
			"system": []map[string]any{
				{"type": "text", "text": "x-anthropic-billing-header: secret-token"},
				{"type": "text", "text": "You are a helpful assistant."},
			},
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": "It is Paris."},
				{"role": "user", "content": "And the capital of Germany?"},
			},
		})
	default:
		return buildRequest(source, model, streaming)
	}
}

// sendFlag builds the unified flag request for source, applies an optional body
// mutation and extra headers, drives it through the gateway, and returns the
// parsed result. It fails the case on transport errors.
func sendFlag(t flagTB, env *TestEnv, source, target protocol.APIType, reqModel string, streaming bool, mutate func(map[string]any), headers map[string]string) *RoundTripResult {
	t.Helper()
	path, body := flagBaseRequest(source, reqModel, streaming)
	if mutate != nil {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("unmarshal base request: %v", err)
		}
		mutate(m)
		body = mustMarshal(m)
	}
	res, err := env.dispatch(source, target, "text", path, body, headers, streaming)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return res
}

// upstreamToolNames extracts the function/tool names from a captured OpenAI-style
// upstream request body.
func upstreamToolNames(body map[string]any) []string {
	var names []string
	tools, _ := body["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if fn, ok := tool["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

// messageRoles returns the role of each message in an Anthropic/OpenAI-style body.
func messageRoles(body map[string]any) []string {
	var roles []string
	msgs, _ := body["messages"].([]any)
	for _, raw := range msgs {
		if m, ok := raw.(map[string]any); ok {
			if r, ok := m["role"].(string); ok {
				roles = append(roles, r)
			}
		}
	}
	return roles
}

// setupBothModeRoute wires a route whose provider advertises EndpointModeBoth,
// so the openai_endpoint_override flag has a real choice to make (a chat- or
// responses-only provider would ignore the override).
func setupBothModeRoute(env *TestEnv, s Scenario, flags typ.RuleFlags) string {
	env.virtual.RegisterScenario(s)
	providerName := "flag-both-" + s.Name
	registerProvider(env, providerName, env.virtual.URL(), ai.EndpointModeBoth)

	reqModel := "pv-flag-both-" + s.Name
	providerModel := "virtual-model-" + s.Name
	rule := typ.Rule{
		UUID:          reqModel,
		Scenario:      typ.ScenarioOpenAI,
		RequestModel:  reqModel,
		ResponseModel: providerModel,
		Services: []*loadbalance.Service{
			{Provider: providerName, Model: providerModel, Weight: 1, Active: true, TimeWindow: 300},
		},
		LBTactic: typ.Tactic{Type: loadbalance.TacticAdaptive, Params: typ.DefaultAdaptiveParams()},
		Active:   true,
		Flags:    flags,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)
	return reqModel
}

// newCountingServer starts an httptest provider that writes handler's response
// and counts the requests it received, so a test can tell which upstream a
// request was routed to. Unlike failover.go's vmodel-backed startFailingProvider,
// this returns a single canned response decoupled from the model registry — flag
// tests need a fixed body/header or an exact SSE description, not model lookups.
func newCountingServer(t flagTB, handler func(w http.ResponseWriter)) (baseURL string, hits *int64) {
	t.Helper()
	var n int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&n, 1)
		handler(w)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &n
}

// newCountingChatServer is a counting provider that returns a fixed non-stream
// chat completion (assistant content == content).
func newCountingChatServer(t flagTB, content string) (baseURL string, hits *int64) {
	body := mustMarshal(map[string]any{
		"id": "chatcmpl-flag", "object": "chat.completion", "created": 0, "model": "m",
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
	return newCountingServer(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}

// newDescriberServer is a counting provider that answers with an SSE stream (the
// vision adapter always uses the streaming OpenAI endpoint), whose assistant
// content is description.
func newDescriberServer(t flagTB, description string) (baseURL string, hits *int64) {
	delta := mustMarshal(map[string]any{
		"id": "desc", "object": "chat.completion.chunk", "created": 0, "model": "vision-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": description}, "finish_reason": nil}},
	})
	done := mustMarshal(map[string]any{
		"id": "desc", "object": "chat.completion.chunk", "created": 0, "model": "vision-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	})
	lines := []string{"data: " + string(delta), "data: " + string(done), "data: [DONE]"}
	return newCountingServer(t, func(w http.ResponseWriter) {
		sse.WriteSSEResponse(w, lines)
	})
}

// registerProvider registers an OpenAI-style provider pointing at the /v1 root of
// base, with the given endpoint mode.
func registerProvider(env *TestEnv, uuid, base string, mode ai.OpenAIEndpointMode) {
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID:               uuid,
		Name:               uuid,
		APIBase:            base + "/v1",
		APIStyle:           protocol.APIStyleOpenAI,
		OpenAIEndpointMode: mode,
		Token:              "virtual-token",
		Enabled:            true,
		Timeout:            int64(constant.DefaultRequestTimeout),
	})
}

// registerOpenAIProvider registers a default (chat) OpenAI provider.
func registerOpenAIProvider(env *TestEnv, uuid, base string) {
	registerProvider(env, uuid, base, ai.EndpointModeUnknown)
}

// assertFlattenedContent verifies cursor compatibility flattened every message's
// rich (array) content down to a plain string in the upstream request.
func assertFlattenedContent(t flagTB, body map[string]any) {
	t.Helper()
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("no upstream messages; body keys=%v", keysOf(body))
	}
	for i, raw := range msgs {
		m, _ := raw.(map[string]any)
		if _, isString := m["content"].(string); !isString {
			t.Errorf("cursor compat left message[%d] content un-flattened; got %T", i, m["content"])
		}
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func ruleFlagCases() []flagCase {
	return []flagCase{
		// ── custom_user_agent ────────────────────────────────────────────────
		{key: "custom_user_agent", run: func(t flagTB, env *TestEnv) {
			const ua = "HarnessFlagUA/9.9"
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{CustomUserAgent: ua})
			// Streaming path: the custom UA rides c.Request.Context() into the
			// forward context, which the OpenAI client's customUserAgentTransport
			// reads. (The non-streaming openai_chat path builds its forward
			// context with a nil baseCtx, so the UA override does not propagate
			// there — tracked separately.)
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, true, nil, nil)
			up := env.virtual.LastRequest(EndpointChat)
			if up == nil {
				t.Fatal("no upstream request captured")
			}
			if got := up.Headers.Get("User-Agent"); got != ua {
				t.Errorf("upstream User-Agent = %q, want %q", got, ua)
			}
		}},

		// ── use_max_completion_tokens ────────────────────────────────────────
		{key: "use_max_completion_tokens", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{UseMaxCompletionTokens: true})
			// The unified request already carries max_tokens.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			body := env.virtual.LastRequest(EndpointChat).JSON()
			if _, ok := body["max_completion_tokens"]; !ok {
				t.Errorf("upstream missing max_completion_tokens; body keys=%v", keysOf(body))
			}
			if _, ok := body["max_tokens"]; ok {
				t.Error("upstream still carries max_tokens after rewrite")
			}
		}},

		// ── use_max_tokens ───────────────────────────────────────────────────
		{key: "use_max_tokens", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{UseMaxTokens: true})
			// This rewrite is the inverse direction, so swap the unified request's
			// max_tokens for the newer field that the flag rewrites back.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, func(m map[string]any) {
				delete(m, "max_tokens")
				m["max_completion_tokens"] = 555
			}, nil)
			body := env.virtual.LastRequest(EndpointChat).JSON()
			if _, ok := body["max_tokens"]; !ok {
				t.Errorf("upstream missing max_tokens; body keys=%v", keysOf(body))
			}
			if _, ok := body["max_completion_tokens"]; ok {
				t.Error("upstream still carries max_completion_tokens after rewrite")
			}
		}},

		// ── block_tools ──────────────────────────────────────────────────────
		{key: "block_tools", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{BlockTools: "web_search"})
			// The unified request already carries the web_search + keep_me tools.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			names := upstreamToolNames(env.virtual.LastRequest(EndpointChat).JSON())
			if slices.Contains(names, "web_search") {
				t.Errorf("blocked tool web_search still forwarded; tools=%v", names)
			}
			if !slices.Contains(names, "keep_me") {
				t.Errorf("non-blocked tool keep_me missing; tools=%v", names)
			}
		}},

		// ── skip_usage ───────────────────────────────────────────────────────
		{key: "skip_usage", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{SkipUsage: true})
			res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			if strings.Contains(string(res.RawBody), "\"usage\"") {
				t.Errorf("client response still contains usage block: %s", truncate(string(res.RawBody), 300))
			}
		}},

		// ── thinking_effort ──────────────────────────────────────────────────
		{key: "thinking_effort", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, flagScenario(), typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortHigh})
			sendFlag(t, env, protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, model, false, nil, nil)
			body := env.virtual.LastRequest(EndpointAnthropic).JSON()
			thinking, ok := body["thinking"].(map[string]any)
			if !ok {
				t.Fatalf("upstream missing thinking block; body keys=%v", keysOf(body))
			}
			if thinking["type"] != "enabled" {
				t.Errorf("thinking.type = %v, want enabled", thinking["type"])
			}
		}},

		// ── claude_code_compat ───────────────────────────────────────────────
		{key: "claude_code_compat", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, flagScenario(), typ.RuleFlags{ClaudeCodeCompat: true})
			// Claude Code's non-standard quirk is a mid-conversation system-role
			// message inside the messages array; inject one for the fold to act on.
			sendFlag(t, env, protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, model, false, func(m map[string]any) {
				m["messages"] = []map[string]any{
					{"role": "user", "content": "What is the capital of France?"},
					{"role": "system", "content": "Answer tersely."},
					{"role": "user", "content": "And of Germany?"},
				}
			}, nil)
			roles := messageRoles(env.virtual.LastRequest(EndpointAnthropic).JSON())
			if slices.Contains(roles, "system") {
				t.Errorf("system-role message survived claude_code_compat fold; roles=%v", roles)
			}
		}},

		// ── clean_header ─────────────────────────────────────────────────────
		{key: "clean_header", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, flagScenario(), typ.RuleFlags{CleanHeader: true})
			// The unified request's system block already carries the injected
			// x-anthropic-billing-header that clean_header must strip.
			sendFlag(t, env, protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, model, false, nil, nil)
			up := string(env.virtual.LastRequest(EndpointAnthropic).Body)
			if strings.Contains(up, "x-anthropic-billing-header") {
				t.Errorf("billing header not stripped from upstream system: %s", truncate(up, 300))
			}
		}},

		// ── cursor_compat ────────────────────────────────────────────────────
		{key: "cursor_compat", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{CursorCompat: true})
			// The unified request already carries array-shaped user content.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			assertFlattenedContent(t, env.virtual.LastRequest(EndpointChat).JSON())
		}},

		// ── cursor_compat_auto ───────────────────────────────────────────────
		{key: "cursor_compat_auto", run: func(t flagTB, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{CursorCompatAuto: true})
			// cursor_compat_auto only fires when a Cursor client is detected.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil,
				map[string]string{"User-Agent": "Cursor/1.2.3"})
			assertFlattenedContent(t, env.virtual.LastRequest(EndpointChat).JSON())
		}},

		// ── openai_endpoint_override ─────────────────────────────────────────
		{key: "openai_endpoint_override", run: func(t flagTB, env *TestEnv) {
			model := setupBothModeRoute(env, flagScenario(), typ.RuleFlags{OpenAIEndpointOverride: "responses"})
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, model, false, nil, nil)
			if hits := env.virtual.EndpointHits(EndpointResponses); hits == 0 {
				t.Errorf("override=responses did not force the Responses endpoint (responses hits=0, chat hits=%d)", env.virtual.EndpointHits(EndpointChat))
			}
		}},

		// ── session_affinity ─────────────────────────────────────────────────
		// Two distinguishable upstreams behind one rule: with affinity on, every
		// request carrying the same session id must pin to the upstream the first
		// request landed on (all hits on one server, none on the other).
		{key: "session_affinity", run: func(t flagTB, env *TestEnv) {
			urlA, hitsA := newCountingChatServer(t, "from-A")
			urlB, hitsB := newCountingChatServer(t, "from-B")
			registerOpenAIProvider(env, "aff-A", urlA)
			registerOpenAIProvider(env, "aff-B", urlB)

			const reqModel = "pv-flag-affinity"
			rule := typ.Rule{
				UUID:          reqModel,
				Scenario:      typ.ScenarioOpenAI,
				RequestModel:  reqModel,
				ResponseModel: "affinity-model",
				Services: []*loadbalance.Service{
					{Provider: "aff-A", Model: "affinity-model", Weight: 1, Active: true, TimeWindow: 300},
					{Provider: "aff-B", Model: "affinity-model", Weight: 1, Active: true, TimeWindow: 300},
				},
				LBTactic: typ.Tactic{Type: loadbalance.TacticAdaptive, Params: typ.DefaultAdaptiveParams()},
				Active:   true,
				Flags:    typ.RuleFlags{SessionAffinity: 3600},
			}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

			const n = 5
			for i := 0; i < n; i++ {
				res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, reqModel, false, nil,
					map[string]string{"X-Tingly-Session-ID": "flag-affinity-session"})
				if res.HTTPStatus != 200 {
					t.Fatalf("request %d failed: status=%d body=%s", i, res.HTTPStatus, truncate(string(res.RawBody), 200))
				}
			}
			a, b := atomic.LoadInt64(hitsA), atomic.LoadInt64(hitsB)
			if a+b != n {
				t.Fatalf("expected %d upstream hits total, got A=%d B=%d", n, a, b)
			}
			if a != n && b != n {
				t.Errorf("session affinity did not pin: hits split A=%d B=%d (want all %d on one)", a, b, n)
			}
		}},

		// ── context_1m ───────────────────────────────────────────────────────
		// Asserted on the real claude_code path: (1) a [1m]-suffixed incoming
		// model still routes to its bare-named rule (suffix-normalized
		// matching), and (2) the rule flag alone — the client sends no beta
		// header here — gets the context-1m beta flag injected into the
		// upstream request (context1mBetaTransport via the context hint).
		{key: "context_1m", run: func(t flagTB, env *TestEnv) {
			s := flagScenario()
			env.virtual.RegisterScenario(s)
			const providerName = "flag-1m-anthropic"
			_ = env.appConfig.AddProvider(&typ.Provider{
				UUID:     providerName,
				Name:     providerName,
				APIBase:  env.virtual.URL(),
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "virtual-token",
				Enabled:  true,
				Timeout:  int64(constant.DefaultRequestTimeout),
			})

			const reqModel = "pv-flag-1m"
			providerModel := "virtual-model-" + s.Name
			rule := typ.Rule{
				UUID:          reqModel,
				Scenario:      typ.ScenarioClaudeCode,
				RequestModel:  reqModel,
				ResponseModel: providerModel,
				Services: []*loadbalance.Service{
					{Provider: providerName, Model: providerModel, Weight: 1, Active: true, TimeWindow: 300},
				},
				LBTactic: typ.Tactic{Type: loadbalance.TacticAdaptive, Params: typ.DefaultAdaptiveParams()},
				Active:   true,
				Flags:    typ.RuleFlags{Context1M: true},
			}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

			_, body := flagBaseRequest(protocol.TypeAnthropicV1, reqModel+"[1m]", false)
			res, err := env.dispatch(protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, s.Name,
				"/tingly/claude_code/v1/messages", body, nil, false)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if res.HTTPStatus != 200 {
				t.Fatalf("[1m]-suffixed model failed to route to its bare-named rule: status=%d body=%s",
					res.HTTPStatus, truncate(string(res.RawBody), 300))
			}
			up := env.virtual.LastRequest(EndpointAnthropic)
			if up == nil {
				t.Fatal("no upstream request captured")
			}
			if beta := up.Headers.Get("anthropic-beta"); !strings.Contains(beta, "context-1m") {
				t.Errorf("rule flag did not inject context-1m beta upstream; anthropic-beta=%q", beta)
			}
		}},

		// ── vision_proxy_service ─────────────────────────────────────────────
		// A request whose latest turn carries an image must have that image
		// described by the configured describer service and replaced with text
		// before it reaches the downstream model — so the upstream request the
		// main provider receives carries no image block, but does carry the
		// describer's text.
		{key: "vision_proxy_service", run: func(t flagTB, env *TestEnv) {
			descURL, descHits := newDescriberServer(t, "a red bicycle leaning on a wall")
			registerOpenAIProvider(env, "vision-describer", descURL)
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(),
				typ.RuleFlags{VisionProxyService: &typ.VisionProxyService{Provider: "vision-describer", Model: "vision-model"}})

			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, func(m map[string]any) {
				m["messages"] = []map[string]any{
					{"role": "user", "content": []map[string]any{
						{"type": "text", "text": "What is in this image?"},
						{"type": "image_url", "image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
						}},
					}},
				}
			}, nil)

			up := string(env.virtual.LastRequest(EndpointChat).Body)
			if strings.Contains(up, "image_url") {
				t.Errorf("vision proxy left an image_url block in the upstream request: %s", truncate(up, 300))
			}
			if atomic.LoadInt64(descHits) == 0 {
				t.Error("vision proxy did not call the describer service")
			}
			if !strings.Contains(up, "red bicycle") {
				t.Errorf("describer output not spliced into the upstream text; body=%s", truncate(up, 400))
			}
		}},
	}
}

// ─── CLI execution (harness matrix --mode=flags) ─────────────────────────────

// flagAbort is the sentinel panic raised by flagRecorder.Fatal{,f} to unwind a
// single case the way testing.T.Fatal does, without aborting the whole run.
var flagAbort = errors.New("flag case aborted")

// flagRecorder is the CLI-side flagTB: it records assertion failures as
// AssertionErrors and defers cleanups, so flag cases run without a *testing.T.
type flagRecorder struct {
	errs     []AssertionError
	cleanups []func()
}

func (r *flagRecorder) Helper()          {}
func (r *flagRecorder) Cleanup(f func()) { r.cleanups = append(r.cleanups, f) }
func (r *flagRecorder) Errorf(f string, a ...any) {
	r.errs = append(r.errs, AssertionError{Assertion: "flag", Error: fmt.Sprintf(f, a...)})
}
func (r *flagRecorder) Error(a ...any) {
	r.errs = append(r.errs, AssertionError{Assertion: "flag", Error: fmt.Sprint(a...)})
}
func (r *flagRecorder) Fatalf(f string, a ...any) { r.Errorf(f, a...); panic(flagAbort) }
func (r *flagRecorder) Fatal(a ...any)            { r.Error(a...); panic(flagAbort) }
func (r *flagRecorder) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

// ExecuteAllFlags runs the rule-flag behavior suite without requiring testing.T.
// It is the CLI-compatible counterpart of TestRuleFlags, returning []TestResult.
// Name format: "flags/<flag key>".
func (m *Matrix) ExecuteAllFlags() []TestResult {
	results := make([]TestResult, 0, len(ruleFlagCases()))
	for _, fc := range ruleFlagCases() {
		results = append(results, runFlagCaseCLI(fc))
	}
	return results
}

func runFlagCaseCLI(fc flagCase) TestResult {
	// Scenario carries the flag key so the CLI table (which shows the Scenario
	// column, not Name) distinguishes the 13 rows; Name keeps the flags/ prefix.
	res := TestResult{Name: "flags/" + fc.key, Scenario: fc.key}
	start := time.Now()

	env, err := NewTestEnvForCLI()
	if err != nil {
		res.Errors = []AssertionError{{Assertion: "setup", Error: fmt.Sprintf("create test env: %v", err)}}
		res.Duration = time.Since(start)
		return res
	}
	defer env.Close()

	rec := &flagRecorder{}
	func() {
		defer func() {
			rec.runCleanups()
			if r := recover(); r != nil && r != flagAbort {
				panic(r)
			}
		}()
		fc.run(rec, env)
	}()

	res.Errors = rec.errs
	res.Passed = len(rec.errs) == 0
	res.Duration = time.Since(start)
	return res
}
