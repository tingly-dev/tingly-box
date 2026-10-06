package protocoltest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the rule-flag × path suite, shared by the go test entry point
// (TestFlagPaths) and the CLI (`harness matrix --mode=flag_paths`).
//
// The flags section (flags.go) proves each rule flag once, mostly on OpenAI
// Chat → Chat. But a flag is applied at a fixed step of the pipeline
// (.design/protocol-stage-pipeline.md) and the steps run differently per
// path: the full chain vs the Stage pipeline (Anthropic → OpenAI, OpenAI →
// Anthropic), streaming vs not, and some flags act before routing (vision
// proxy) or at the transport (headers). This suite crosses every flag with
// every source → target pair it applies to (Google out of scope) and both
// streaming modes, and checks the flag's effect where it must show: on the
// request the provider received, or on the response the client got.

const flagPathsScenario = "flag_paths"

var flagPathsSources = []protocol.APIType{
	protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta,
	protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses,
}

var flagPathsTargets = []protocol.APIType{
	protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses,
}

const (
	flagPathsDescription = "a red bicycle leaning on a wall"
	flagPathsImageMarker = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwC"
	flagPathsImageData   = flagPathsImageMarker + "AAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	flagPathsMidSystem   = "Answer tersely."
)

// flagPathsMaterial selects what the client request carries for a flag to act on.
type flagPathsMaterial struct {
	tools         bool // web_search + keep_me
	billingHeader bool // Anthropic system block with an injected billing header
	midSystem     bool // Anthropic mid-conversation system-role message
	arrayContent  bool // Chat user content as an array of text parts
	image         bool // an image in the latest user turn
}

// flagPathsBody builds the client request in the source's own protocol.
func flagPathsBody(source protocol.APIType, model string, streaming bool, mat flagPathsMaterial) (string, []byte) {
	path, _ := buildRequest(source, model, streaming) // only the path is reused
	body := map[string]any{"model": model, "stream": streaming}
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		body["max_tokens"] = 256
		system := []map[string]any{{"type": "text", "text": "You are a helpful assistant."}}
		if mat.billingHeader {
			system = append([]map[string]any{{"type": "text", "text": "x-anthropic-billing-header: secret-token"}}, system...)
		}
		body["system"] = system
		last := []map[string]any{{"type": "text", "text": "What is in this request?"}}
		if mat.image {
			last = append(last, map[string]any{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": "image/png", "data": flagPathsImageData,
			}})
		}
		messages := []map[string]any{{"role": "user", "content": last}}
		if mat.midSystem {
			messages = []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "system", "content": flagPathsMidSystem},
				{"role": "user", "content": last},
			}
		}
		body["messages"] = messages
		if mat.tools {
			body["tools"] = []map[string]any{
				{"name": "web_search", "input_schema": map[string]any{"type": "object"}},
				{"name": "keep_me", "input_schema": map[string]any{"type": "object"}},
			}
		}
	case protocol.TypeOpenAIChat:
		body["max_tokens"] = 256
		var user any = "What is in this request?"
		if mat.arrayContent || mat.image {
			parts := []map[string]any{{"type": "text", "text": "What is"}, {"type": "text", "text": " in this request?"}}
			if mat.image {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + flagPathsImageData}})
			}
			user = parts
		}
		body["messages"] = []map[string]any{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": user},
		}
		if mat.tools {
			body["tools"] = []map[string]any{
				{"type": "function", "function": map[string]any{"name": "web_search", "parameters": map[string]any{"type": "object"}}},
				{"type": "function", "function": map[string]any{"name": "keep_me", "parameters": map[string]any{"type": "object"}}},
			}
		}
	case protocol.TypeOpenAIResponses:
		body["max_output_tokens"] = 256
		body["instructions"] = "You are a helpful assistant."
		content := []map[string]any{{"type": "input_text", "text": "What is in this request?"}}
		if mat.image {
			content = append(content, map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + flagPathsImageData})
		}
		body["input"] = []map[string]any{{"type": "message", "role": "user", "content": content}}
		if mat.tools {
			body["tools"] = []map[string]any{
				{"type": "function", "name": "web_search", "parameters": map[string]any{"type": "object"}},
				{"type": "function", "name": "keep_me", "parameters": map[string]any{"type": "object"}},
			}
		}
	}
	return path, mustMarshal(body)
}

// flagPathsUpstreamTools returns the tool names in an upstream request body.
func flagPathsUpstreamTools(target protocol.APIType, body map[string]any) []string {
	var names []string
	tools, _ := body["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if target == protocol.TypeOpenAIChat {
			fn, _ := tool["function"].(map[string]any)
			tool = fn
		}
		if name, ok := tool["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// flagPathsSystemTexts collects every system / developer-scoped text in an
// upstream body: top-level system and instructions, and messages / input items
// with a system or developer role.
func flagPathsSystemTexts(body map[string]any) string {
	var b strings.Builder
	if v, ok := body["system"]; ok {
		b.WriteString(string(mustMarshal(v)))
	}
	if v, ok := body["instructions"].(string); ok {
		b.WriteString(v)
	}
	for _, key := range []string{"messages", "input"} {
		items, _ := body[key].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if role, _ := item["role"].(string); role == "system" || role == "developer" {
				b.WriteString(string(mustMarshal(item)))
			}
		}
	}
	return b.String()
}

// flagPathsSend sends one request and returns the client result and the
// request the target endpoint received for it.
func flagPathsSend(t flagTB, env *TestEnv, source, target protocol.APIType, model string, streaming bool, mat flagPathsMaterial, headers map[string]string) (*RoundTripResult, *CapturedRequest) {
	t.Helper()
	endpoint := targetEndpoint(target)
	before := env.virtual.EndpointHits(endpoint)
	path, body := flagPathsBody(source, model, streaming, mat)
	res, err := env.dispatch(source, target, flagPathsScenario, path, body, headers, streaming)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.HTTPStatus != 200 {
		t.Fatalf("client got HTTP %d: %s", res.HTTPStatus, truncate(string(res.RawBody), 300))
	}
	if env.virtual.EndpointHits(endpoint) != before+1 {
		t.Fatalf("expected one upstream %s request, got %d", endpoint, env.virtual.EndpointHits(endpoint)-before)
	}
	return res, env.virtual.LastRequest(endpoint)
}

func flagPathsRoute(env *TestEnv, source, target protocol.APIType, flags typ.RuleFlags) string {
	s := TextScenario()
	s.Name = flagPathsScenario
	s.Assertions = nil
	return env.SetupRouteWithFlags(source, target, s, flags)
}

// clientResponseHasUsage reports whether the client-facing response carries a
// usage object (a JSON null does not count).
func clientResponseHasUsage(res *RoundTripResult) bool {
	return strings.Contains(strings.ReplaceAll(string(res.RawBody), " ", ""), `"usage":{`)
}

func streamLabel(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "nonstream"
}

func flagPathsCase(flag string, source, target protocol.APIType, streaming bool, run func(t flagTB, env *TestEnv)) recorderCase {
	return recorderCase{
		name:      fmt.Sprintf("flag_paths/%s/%s->%s/%s", flag, source, target, streamLabel(streaming)),
		scenario:  flagPathsScenario,
		source:    source,
		target:    target,
		streaming: streaming,
		run:       run,
	}
}

// forPairs adds one case per (source, target, streaming) the predicate keeps.
func forPairs(flag string, keep func(source, target protocol.APIType) bool, run func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool)) []recorderCase {
	var cases []recorderCase
	for _, source := range flagPathsSources {
		for _, target := range flagPathsTargets {
			if keep != nil && !keep(source, target) {
				continue
			}
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				cases = append(cases, flagPathsCase(flag, source, target, streaming, func(t flagTB, env *TestEnv) {
					run(t, env, source, target, streaming)
				}))
			}
		}
	}
	return cases
}

func fromAnthropic(source, _ protocol.APIType) bool { return isAnthropicAPI(source) }
func fromChat(source, _ protocol.APIType) bool      { return source == protocol.TypeOpenAIChat }

func flagPathsCases() []recorderCase {
	var cases []recorderCase

	// ── Type 2: transport headers ─────────────────────────────────────────
	cases = append(cases, forPairs("custom_user_agent", nil, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		const ua = "HarnessFlagUA/9.9"
		model := flagPathsRoute(env, source, target, typ.RuleFlags{CustomUserAgent: ua})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{}, nil)
		if got := up.Headers.Get("User-Agent"); got != ua {
			t.Errorf("upstream User-Agent = %q, want %q", got, ua)
		}
	})...)
	cases = append(cases, forPairs("extra_headers", nil, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		model := flagPathsRoute(env, source, target, typ.RuleFlags{ExtraHeaders: map[string]string{"X-Team-Tag": "research"}})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{}, nil)
		if got := up.Headers.Get("X-Team-Tag"); got != "research" {
			t.Errorf("upstream X-Team-Tag = %q, want research", got)
		}
	})...)

	// ── Type 1b-pre: client-shape transforms ──────────────────────────────
	cases = append(cases, forPairs("block_tools", nil, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		model := flagPathsRoute(env, source, target, typ.RuleFlags{BlockTools: "web_search"})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{tools: true}, nil)
		names := flagPathsUpstreamTools(target, up.JSON())
		if slices.Contains(names, "web_search") {
			t.Errorf("blocked tool web_search still forwarded; tools=%v", names)
		}
		if !slices.Contains(names, "keep_me") {
			t.Errorf("non-blocked tool keep_me missing; tools=%v", names)
		}
	})...)
	cases = append(cases, forPairs("clean_header", fromAnthropic, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		model := flagPathsRoute(env, source, target, typ.RuleFlags{CleanHeader: true})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{billingHeader: true}, nil)
		if strings.Contains(string(up.Body), "x-anthropic-billing-header") {
			t.Errorf("billing header reached the provider: %s", truncate(string(up.Body), 300))
		}
	})...)
	cases = append(cases, forPairs("claude_code_compat", fromAnthropic, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		model := flagPathsRoute(env, source, target, typ.RuleFlags{ClaudeCodeCompat: true})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{midSystem: true}, nil)
		if !strings.Contains(string(up.Body), flagPathsMidSystem) {
			t.Errorf("mid-conversation system text dropped: %s", truncate(string(up.Body), 300))
		}
		if strings.Contains(flagPathsSystemTexts(up.JSON()), flagPathsMidSystem) {
			t.Errorf("mid-conversation system text not folded into a user turn: %s", truncate(string(up.Body), 400))
		}
	})...)
	for _, auto := range []bool{false, true} {
		flag, flags, headers := "cursor_compat", typ.RuleFlags{CursorCompat: true}, map[string]string(nil)
		if auto {
			flag, flags, headers = "cursor_compat_auto", typ.RuleFlags{CursorCompatAuto: true}, map[string]string{"User-Agent": "Cursor/1.2.3"}
		}
		cases = append(cases, forPairs(flag, fromChat, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
			model := flagPathsRoute(env, source, target, flags)
			res, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{arrayContent: true}, headers)
			if target == protocol.TypeOpenAIChat {
				assertFlattenedContent(t, up.JSON())
			}
			if clientResponseHasUsage(res) {
				t.Errorf("client response still carries usage: %s", truncate(string(res.RawBody), 300))
			}
		})...)
	}

	// ── Type 3: response shaping ──────────────────────────────────────────
	// skip_usage is for OpenAI Chat clients that choke on the usage chunk
	// (Cursor): their answer leaves usage out, every other client keeps it.
	cases = append(cases, forPairs("skip_usage", nil, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		model := flagPathsRoute(env, source, target, typ.RuleFlags{SkipUsage: true})
		res, _ := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{}, nil)
		switch has := clientResponseHasUsage(res); {
		case source == protocol.TypeOpenAIChat && has:
			t.Errorf("Chat client response still carries usage: %s", truncate(string(res.RawBody), 300))
		case source != protocol.TypeOpenAIChat && !has:
			t.Errorf("%s client response lost its usage: %s", source, truncate(string(res.RawBody), 300))
		}
	})...)

	// ── Before routing: vision proxy ──────────────────────────────────────
	cases = append(cases, forPairs("vision_proxy_service", nil, func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
		descURL, descHits := newDescriberServer(t, flagPathsDescription)
		registerOpenAIProvider(env, "vision-describer", descURL)
		model := flagPathsRoute(env, source, target, typ.RuleFlags{
			VisionProxyService: &typ.VisionProxyService{Provider: "vision-describer", Model: "vision-model"},
		})
		_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{image: true}, nil)
		if atomic.LoadInt64(descHits) == 0 {
			t.Error("vision proxy did not call the describer service")
		}
		if strings.Contains(string(up.Body), flagPathsImageMarker) {
			t.Errorf("image reached the provider: %s", truncate(string(up.Body), 300))
		}
		if !strings.Contains(string(up.Body), flagPathsDescription) {
			t.Errorf("describer output not in the upstream request: %s", truncate(string(up.Body), 400))
		}
	})...)

	// ── Type 4: endpoint routing (plan) ───────────────────────────────────
	for _, source := range flagPathsSources {
		for _, override := range []string{"chat", "responses"} {
			for _, streaming := range []bool{false, true} {
				source, override, streaming := source, override, streaming
				target := protocol.TypeOpenAIChat
				if override == "responses" {
					target = protocol.TypeOpenAIResponses
				}
				cases = append(cases, recorderCase{
					name:     fmt.Sprintf("flag_paths/openai_endpoint_override=%s/%s/%s", override, source, streamLabel(streaming)),
					scenario: flagPathsScenario, source: source, target: target, streaming: streaming,
					run: func(t flagTB, env *TestEnv) {
						s := TextScenario()
						s.Name = flagPathsScenario
						s.Assertions = nil
						env.virtual.RegisterScenario(s)
						registerProvider(env, "flag-paths-both", env.virtual.URL(), ai.EndpointModeBoth)
						const reqModel = "pv-flag-paths-override"
						rule := newHarnessRule(reqModel, sourceToRuleScenario(source), reqModel, "virtual-model-"+s.Name,
							harnessService("flag-paths-both", "virtual-model-"+s.Name))
						rule.Flags = typ.RuleFlags{OpenAIEndpointOverride: override}
						_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)
						flagPathsSend(t, env, source, target, reqModel, streaming, flagPathsMaterial{}, nil)
					},
				})
			}
		}
	}

	// ── Anthropic SDK parameters from the context ─────────────────────────
	cases = append(cases, forPairs("context_1m", func(_, target protocol.APIType) bool { return target == protocol.TypeAnthropicBeta },
		func(t flagTB, env *TestEnv, source, target protocol.APIType, streaming bool) {
			model := flagPathsRoute(env, source, target, typ.RuleFlags{Context1M: true})
			_, up := flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{}, nil)
			if beta := up.Headers.Get("anthropic-beta"); !strings.Contains(beta, "context-1m") {
				t.Errorf("context-1m beta not sent upstream; anthropic-beta=%q", beta)
			}
		})...)

	// ── Recording (StagePre / StagePost) ──────────────────────────────────
	for _, source := range flagPathsSources {
		for _, target := range flagPathsTargets {
			source, target := source, target
			cases = append(cases, flagPathsCase("recording", source, target, false, func(t flagTB, _ *TestEnv) {
				dir, err := os.MkdirTemp("", "flag-paths-recording-*")
				if err != nil {
					t.Fatalf("temp record dir: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(dir) })
				renv, err := NewTestEnvForCLI(NewTestEnvOptionWithRecordDir(dir))
				if err != nil {
					t.Fatalf("create recording env: %v", err)
				}
				t.Cleanup(renv.Close)
				model := flagPathsRoute(renv, source, target, typ.RuleFlags{Recording: "client_request,upstream_request"})
				flagPathsSend(t, renv, source, target, model, false, flagPathsMaterial{}, nil)
				renv.FlushRecordSinks(context.Background(), sourceToRuleScenario(source))
				records := readRecordedLines(t, dir)
				if len(records) == 0 {
					t.Fatal("rule-level recording flag produced no records")
				}
				rec := records[len(records)-1]
				if rec["original_request"] == nil {
					t.Error("record missing original_request")
				}
				if rec["transformed_request"] == nil {
					t.Error("record missing transformed_request")
				}
			}))
		}
	}

	// ── Boundary recording (.design/recording.md R1) ──────────────────────
	// Every source → target pair, streaming or not, must produce one trace
	// with the inbound request and the upstream exchange — recording no
	// longer depends on which protocol path served the request (cf. FP3).
	for _, source := range flagPathsSources {
		for _, target := range flagPathsTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				cases = append(cases, flagPathsCase("recording_capture", source, target, streaming, func(t flagTB, _ *TestEnv) {
					checkBoundaryRecording(t, source, target, streaming)
				}))
			}
		}
	}

	// ── Type 5: session affinity (two Chat upstreams) ─────────────────────
	for _, source := range flagPathsSources {
		source := source
		cases = append(cases, flagPathsCase("session_affinity", source, protocol.TypeOpenAIChat, false, func(t flagTB, env *TestEnv) {
			urlA, hitsA := newCountingChatServer(t, "from-A")
			urlB, hitsB := newCountingChatServer(t, "from-B")
			registerOpenAIProvider(env, "aff-A", urlA)
			registerOpenAIProvider(env, "aff-B", urlB)
			const reqModel = "pv-flag-paths-affinity"
			rule := newHarnessRule(reqModel, sourceToRuleScenario(source), reqModel, "affinity-model",
				harnessService("aff-A", "affinity-model"), harnessService("aff-B", "affinity-model"))
			rule.Flags = typ.RuleFlags{SessionAffinity: 3600}
			_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)
			const n = 5
			for i := 0; i < n; i++ {
				path, body := flagPathsBody(source, reqModel, false, flagPathsMaterial{})
				res, err := env.dispatch(source, protocol.TypeOpenAIChat, flagPathsScenario, path, body,
					map[string]string{"X-Tingly-Session-ID": "flag-paths-affinity"}, false)
				if err != nil {
					t.Fatalf("dispatch: %v", err)
				}
				if res.HTTPStatus != 200 {
					t.Fatalf("request %d failed: status=%d body=%s", i, res.HTTPStatus, truncate(string(res.RawBody), 200))
				}
			}
			if a, b := atomic.LoadInt64(hitsA), atomic.LoadInt64(hitsB); a != n && b != n {
				t.Errorf("session affinity did not pin: hits A=%d B=%d (want all %d on one)", a, b, n)
			}
		}))
	}

	// ── Claude OAuth provider: organization header ────────────────────────
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		for _, streaming := range []bool{false, true} {
			source, streaming := source, streaming
			cases = append(cases, flagPathsCase("claude_org_id", source, protocol.TypeAnthropicBeta, streaming, func(t flagTB, env *TestEnv) {
				s := TextScenario()
				s.Name = flagPathsScenario
				s.Assertions = nil
				env.virtual.RegisterScenario(s)
				const providerName = "flag-paths-claude"
				const orgID = "99999999-8888-7777-6666-555555555555"
				_ = env.appConfig.AddProvider(&typ.Provider{
					UUID: providerName, Name: providerName, APIBase: env.virtual.URL(),
					APIStyle: protocol.APIStyleAnthropic, AuthType: ai.AuthTypeOAuth,
					OAuthDetail: &ai.OAuthDetail{Issuer: ai.IssuerClaudeCode, AccessToken: "sk-ant-oat01-virtual"},
					Enabled:     true, Timeout: int64(constant.DefaultRequestTimeout),
				})
				const reqModel = "pv-flag-paths-orgid"
				rule := newHarnessRule(reqModel, typ.ScenarioClaudeCode, reqModel, "virtual-model-"+s.Name,
					harnessService(providerName, "virtual-model-"+s.Name))
				rule.Flags = typ.RuleFlags{ClaudeOrgID: orgID}
				_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

				path, body := flagPathsBody(source, reqModel, streaming, flagPathsMaterial{})
				var m map[string]any
				_ = json.Unmarshal(body, &m)
				m["metadata"] = map[string]any{"user_id": `{"device_id":"d","account_uuid":"a","session_id":"flag-paths-orgid"}`}
				path = strings.Replace(path, "/tingly/anthropic/", "/tingly/claude_code/", 1)
				res, err := env.dispatch(source, protocol.TypeAnthropicBeta, s.Name, path, mustMarshal(m), nil, streaming)
				if err != nil {
					t.Fatalf("dispatch: %v", err)
				}
				if res.HTTPStatus != 200 {
					t.Fatalf("client got HTTP %d: %s", res.HTTPStatus, truncate(string(res.RawBody), 300))
				}
				if got := env.virtual.LastRequest(EndpointAnthropic).Headers.Get("anthropic-organization-id"); got != orgID {
					t.Errorf("anthropic-organization-id = %q, want %q", got, orgID)
				}
			}))
		}
	}
	return cases
}

// ExecuteAllFlagPaths runs the rule-flag × path suite without testing.T.
func (m *Matrix) ExecuteAllFlagPaths() []TestResult {
	results := m.runRecorderCases(flagPathsCases())
	for i := range results {
		results[i] = judgeFlagPathsResult(results[i])
	}
	return results
}

// judgeFlagPathsResult applies the known-gap registry to one case.
func judgeFlagPathsResult(r TestResult) TestResult {
	var failures []string
	for _, e := range r.Errors {
		failures = append(failures, e.Error)
	}
	errs, gap := judgeCase(r.Name, failures)
	switch {
	case gap != nil:
		r.KnownGap = gap.ID
		for i := range r.Errors {
			r.Errors[i].Assertion = "known gap " + gap.ID
			r.Errors[i].Context = gap.Reason
		}
	case len(errs) > 0 && len(failures) == 0:
		r.Errors = []AssertionError{{Assertion: "flag_paths", Error: errs[0]}}
		r.Passed = false
	}
	return r
}

// Known gaps the suite exposed when it was added; each fails identically on
// the base it was added against.
var (
	gapRecordingResponsesCross = KnownGap{
		ID:     "FP3",
		Reason: "rule-level recording writes no record for an OpenAI Responses client served by an Anthropic or OpenAI Chat provider",
	}
)

var _ = func() bool {
	for _, target := range []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat} {
		registerKnownGaps(gapRecordingResponsesCross, fmt.Sprintf("flag_paths/recording/openai_responses->%s/nonstream", target))
	}
	return true
}()
