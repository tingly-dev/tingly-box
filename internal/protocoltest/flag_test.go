package protocoltest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the rule-flag behavior suite. It is deliberately separate from
// the protocol matrix (which stays flag-free): the matrix tests conversion
// fidelity, while this suite drives one request per flag through the REAL
// gateway with rule.Flags set and asserts the observable effect on either the
// upstream request the provider received (CapturedRequest) or the client
// response. Each case maps to exactly one key in typ.RuleFlagRegistry(), and
// TestRuleFlagRegistry_FullyCovered fails if any registry flag lacks a case —
// so a new flag cannot ship without a behavior test (closing the silent-omission
// gap that caused the #1168 flag-loss bug).

// flagCase is one rule-flag behavior test. run performs the full setup → send →
// assert against a fresh gateway env.
type flagCase struct {
	key string // must equal a typ.RuleFlagRegistry() key
	run func(t *testing.T, env *TestEnv)
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
// parsed result. It fails the test on transport errors.
func sendFlag(t *testing.T, env *TestEnv, source, target protocol.APIType, reqModel string, streaming bool, mutate func(map[string]any), headers map[string]string) *RoundTripResult {
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
	provider := &typ.Provider{
		UUID:               providerName,
		Name:               providerName,
		APIBase:            env.virtual.URL() + "/v1",
		APIStyle:           protocol.APIStyleOpenAI,
		OpenAIEndpointMode: ai.EndpointModeBoth,
		Token:              "virtual-token",
		Enabled:            true,
		Timeout:            int64(constant.DefaultRequestTimeout),
	}
	_ = env.appConfig.AddProvider(provider)

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

func ruleFlagCases() []flagCase {
	return []flagCase{
		// ── custom_user_agent ────────────────────────────────────────────────
		{key: "custom_user_agent", run: func(t *testing.T, env *TestEnv) {
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
		{key: "use_max_completion_tokens", run: func(t *testing.T, env *TestEnv) {
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
		{key: "use_max_tokens", run: func(t *testing.T, env *TestEnv) {
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
		{key: "block_tools", run: func(t *testing.T, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{BlockTools: "web_search"})
			// The unified request already carries the web_search + keep_me tools.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			names := upstreamToolNames(env.virtual.LastRequest(EndpointChat).JSON())
			if contains(names, "web_search") {
				t.Errorf("blocked tool web_search still forwarded; tools=%v", names)
			}
			if !contains(names, "keep_me") {
				t.Errorf("non-blocked tool keep_me missing; tools=%v", names)
			}
		}},

		// ── skip_usage ───────────────────────────────────────────────────────
		{key: "skip_usage", run: func(t *testing.T, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{SkipUsage: true})
			res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			if strings.Contains(string(res.RawBody), "\"usage\"") {
				t.Errorf("client response still contains usage block: %s", truncate(string(res.RawBody), 300))
			}
		}},

		// ── thinking_effort ──────────────────────────────────────────────────
		{key: "thinking_effort", run: func(t *testing.T, env *TestEnv) {
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
		{key: "claude_code_compat", run: func(t *testing.T, env *TestEnv) {
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
			if contains(roles, "system") {
				t.Errorf("system-role message survived claude_code_compat fold; roles=%v", roles)
			}
		}},

		// ── clean_header ─────────────────────────────────────────────────────
		{key: "clean_header", run: func(t *testing.T, env *TestEnv) {
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
		{key: "cursor_compat", run: func(t *testing.T, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{CursorCompat: true})
			// The unified request already carries array-shaped user content.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			assertFlattenedContent(t, env.virtual.LastRequest(EndpointChat).JSON())
		}},

		// ── cursor_compat_auto ───────────────────────────────────────────────
		{key: "cursor_compat_auto", run: func(t *testing.T, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{CursorCompatAuto: true})
			// cursor_compat_auto only fires when a Cursor client is detected.
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil,
				map[string]string{"User-Agent": "Cursor/1.2.3"})
			assertFlattenedContent(t, env.virtual.LastRequest(EndpointChat).JSON())
		}},

		// ── openai_endpoint_override ─────────────────────────────────────────
		{key: "openai_endpoint_override", run: func(t *testing.T, env *TestEnv) {
			model := setupBothModeRoute(env, flagScenario(), typ.RuleFlags{OpenAIEndpointOverride: "responses"})
			sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, model, false, nil, nil)
			if hits := env.virtual.EndpointHits(EndpointResponses); hits == 0 {
				t.Errorf("override=responses did not force the Responses endpoint (responses hits=0, chat hits=%d)", env.virtual.EndpointHits(EndpointChat))
			}
		}},

		// ── session_affinity ─────────────────────────────────────────────────
		// Behavioral pinning is covered by the loadbalance affinity tests; here
		// we only verify the flag is accepted and the request still completes
		// through the gateway (guards against wiring/parse regressions).
		{key: "session_affinity", run: func(t *testing.T, env *TestEnv) {
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), typ.RuleFlags{SessionAffinity: 3600})
			res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil,
				map[string]string{"X-Tingly-Session-ID": "flag-affinity-session"})
			if res.HTTPStatus != 200 {
				t.Errorf("session_affinity request failed: status=%d body=%s", res.HTTPStatus, truncate(string(res.RawBody), 200))
			}
		}},

		// ── vision_proxy_service ─────────────────────────────────────────────
		// The vision proxy describe path has its own dedicated tests; a text-only
		// request leaves it dormant, so here we only assert the flag wires up
		// cleanly and a normal request still succeeds end-to-end.
		{key: "vision_proxy_service", run: func(t *testing.T, env *TestEnv) {
			flags := typ.RuleFlags{VisionProxyService: &typ.VisionProxyService{Provider: "describer", Model: "vision-model"}}
			model := env.SetupRouteWithFlags(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, flagScenario(), flags)
			res := sendFlag(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, model, false, nil, nil)
			if res.HTTPStatus != 200 {
				t.Errorf("vision_proxy_service request failed: status=%d body=%s", res.HTTPStatus, truncate(string(res.RawBody), 200))
			}
		}},
	}
}

// assertFlattenedContent verifies cursor compatibility flattened every message's
// rich (array) content down to a plain string in the upstream request.
func assertFlattenedContent(t *testing.T, body map[string]any) {
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

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestRuleFlags drives every rule flag through the real gateway and asserts its
// observable effect.
func TestRuleFlags(t *testing.T) {
	for _, fc := range ruleFlagCases() {
		fc := fc
		t.Run(fc.key, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			fc.run(t, env)
		})
	}
}

// TestRuleFlagRegistry_FullyCovered locks the contract that every flag in the
// canonical registry has a behavior test, and that no test references a flag key
// that no longer exists. Adding a flag to RuleFlagRegistry() without a case here
// fails this test.
func TestRuleFlagRegistry_FullyCovered(t *testing.T) {
	known := map[string]bool{}
	for _, spec := range typ.RuleFlagRegistry() {
		known[spec.Key] = true
	}
	covered := map[string]bool{}
	for _, fc := range ruleFlagCases() {
		if !known[fc.key] {
			t.Errorf("flag case %q does not match any typ.RuleFlagRegistry() key", fc.key)
		}
		if covered[fc.key] {
			t.Errorf("duplicate flag case for %q", fc.key)
		}
		covered[fc.key] = true
	}
	for key := range known {
		if !covered[key] {
			t.Errorf("rule flag %q has no behavior test in ruleFlagCases() — add one", key)
		}
	}
}
