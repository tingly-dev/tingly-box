package protocoltest

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ids"
)

// cache_prefix is the cross-request prompt-cache regression section shared by
// go test and `harness matrix --mode=cache_prefix`.
//
// It is the complement of cache_controls. That section asks "does a cache
// marker survive one conversion?" — a property of a single request. This one
// asks the question the token bill actually answers: **do two consecutive
// requests of the same conversation still share a prefix by the time they
// reach the upstream?**
//
// Every prompt cache — OpenAI's, ChatGPT's Codex backend, Anthropic's — keys on
// the request prefix. A gateway that re-serializes history even slightly
// differently from one turn to the next invalidates the cache from the first
// differing byte, and nothing in the response says so: the request succeeds,
// the answer is correct, and the user is silently re-billed for the whole
// conversation. #1718's Codex collapse was exactly this and went unnoticed
// through every existing section, because each individual request was valid.
//
// Three properties are checked, per client shape × target protocol:
//
//   - rotation: a client moves its cache breakpoints forward every turn (Claude
//     Code carries four). Moving one must not change anything else about the
//     dispatched request.
//   - growth: turn N+1 appends to turn N's history. Every item turn N sent must
//     reach the upstream byte-identical on turn N+1.
//   - affinity: the conversation's identity must reach the upstream as a stable
//     cache key wherever the target protocol can express one, since a
//     byte-identical prefix still misses a cache it never reached.
//
// The client shapes are real ones (see cachePrefixClients): what Claude Code,
// a plain Anthropic SDK caller, and the Codex CLI each put on the wire differs
// enough that a prefix bug can hide in one and not the others.

// cachePrefixTurns is how many user/tool exchanges the base conversation
// carries; the growth check sends this many and then one more.
const cachePrefixTurns = 3

// cachePrefixSessionID is the conversation identity a client stamps on every
// turn — Claude Code in metadata.user_id, Codex in prompt_cache_key.
const cachePrefixSessionID = "16d97292-8713-438b-ad2e-76f495717258"
const cachePrefixOtherSessionID = "9c6f0f5e-2c1b-4a77-9f2e-1b0d7c3a5e84"

// cachePrefixClient is one real client's wire shape. breakpointAt selects
// which cacheable history block carries the client's rolling breakpoint
// (-1 = none); clients that send no breakpoints ignore it and declare
// rotates=false.
type cachePrefixClient struct {
	name    string
	source  protocol.APIType
	rotates bool
	// carriesAffinity is whether this client tells the gateway which
	// conversation a request belongs to. Without it there is nothing for the
	// gateway to derive an upstream cache key from.
	carriesAffinity bool
	build           func(model string, streaming bool, turns, breakpointAt int, sessionID string) map[string]any
}

func cachePrefixClients() []cachePrefixClient {
	return []cachePrefixClient{
		{
			name:            "claude_code",
			source:          protocol.TypeAnthropicBeta,
			rotates:         true,
			carriesAffinity: true,
			build: func(model string, streaming bool, turns, breakpointAt int, sessionID string) map[string]any {
				return anthropicCachePrefixBody(model, streaming, turns, breakpointAt, sessionID, true)
			},
		},
		{
			// A plain Anthropic SDK caller: one system block, no tool-level
			// cache_control, no metadata.user_id. It still rotates content
			// breakpoints, so it exercises the shape switch without the
			// Claude-Code-specific scaffolding around it.
			name:            "anthropic_sdk",
			source:          protocol.TypeAnthropicV1,
			rotates:         true,
			carriesAffinity: false,
			build: func(model string, streaming bool, turns, breakpointAt int, _ string) map[string]any {
				return anthropicCachePrefixBody(model, streaming, turns, breakpointAt, "", false)
			},
		},
		{
			// Codex sends no breakpoints at all (its backend rejects them) but
			// does send prompt_cache_key, and it replays reasoning items with
			// the ids we minted. Growth is the property that matters here.
			name:            "codex_cli",
			source:          protocol.TypeOpenAIResponses,
			rotates:         false,
			carriesAffinity: true,
			build:           responsesCachePrefixBody,
		},
	}
}

// anthropicCachePrefixBody builds an Anthropic request carrying `turns`
// user/tool exchanges. claudeCode adds the parts specific to Claude Code: a
// second system block, a cache_control'd tool definition, and metadata.user_id.
func anthropicCachePrefixBody(model string, streaming bool, turns, breakpointAt int, sessionID string, claudeCode bool) map[string]any {
	ephemeral := map[string]any{"type": "ephemeral"}

	var system []map[string]any
	if claudeCode {
		system = []map[string]any{
			{"type": "text", "text": "You are Claude Code."},
			{"type": "text", "text": cachePrefixSystemText, "cache_control": ephemeral},
		}
	} else {
		system = []map[string]any{{"type": "text", "text": cachePrefixSystemText}}
	}

	tool := map[string]any{
		"name":         "read_file",
		"description":  "Read a file",
		"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
	}
	if claudeCode {
		tool["cache_control"] = ephemeral
	}

	var messages []map[string]any
	block := 0
	withBreakpoint := func(m map[string]any) map[string]any {
		if block == breakpointAt {
			m["cache_control"] = ephemeral
		}
		block++
		return m
	}
	for i := 0; i < turns; i++ {
		messages = append(messages, map[string]any{
			"role": "user",
			"content": []map[string]any{withBreakpoint(map[string]any{
				"type": "text", "text": fmt.Sprintf("user turn %d", i),
			})},
		})
		messages = append(messages, map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("working on turn %d", i)},
				{"type": "tool_use", "id": cachePrefixCallID(i), "name": "read_file", "input": map[string]any{"path": "x"}},
			},
		})
		messages = append(messages, map[string]any{
			"role": "user",
			"content": []map[string]any{withBreakpoint(map[string]any{
				"type":        "tool_result",
				"tool_use_id": cachePrefixCallID(i),
				"content":     []map[string]any{{"type": "text", "text": fmt.Sprintf("tool output %d", i)}},
			})},
		})
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 64,
		"stream":     streaming,
		"system":     system,
		"tools":      []map[string]any{tool},
		"messages":   messages,
	}
	if sessionID != "" {
		body["metadata"] = map[string]any{
			"user_id": fmt.Sprintf(
				`{"device_id":"harness-device","account_uuid":"harness-account","session_id":%q}`, sessionID),
		}
	}
	return body
}

// responsesCachePrefixBody builds the Codex CLI's wire shape: instructions +
// a flat input item list that replays our minted reasoning ids verbatim, a
// stable prompt_cache_key, and no cache breakpoints anywhere.
func responsesCachePrefixBody(model string, streaming bool, turns, _ int, sessionID string) map[string]any {
	input := []map[string]any{}
	for i := 0; i < turns; i++ {
		input = append(input,
			map[string]any{
				"type": "message", "role": "user",
				"content": []map[string]any{{"type": "input_text", "text": fmt.Sprintf("user turn %d", i)}},
			},
			map[string]any{
				"type": "reasoning", "id": cachePrefixReasoningID(i), "summary": []any{},
			},
			map[string]any{
				"type": "function_call", "call_id": cachePrefixCallID(i),
				"name": "read_file", "arguments": `{"path":"x"}`,
			},
			map[string]any{
				"type": "function_call_output", "call_id": cachePrefixCallID(i),
				"output": fmt.Sprintf("tool output %d", i),
			},
		)
	}

	body := map[string]any{
		"model":        model,
		"stream":       streaming,
		"store":        false,
		"instructions": cachePrefixSystemText,
		"input":        input,
		"tools": []map[string]any{{
			"type": "function", "name": "read_file", "description": "Read a file",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	}
	if sessionID != "" {
		body["prompt_cache_key"] = sessionID
	}
	return body
}

const cachePrefixSystemText = "STABLE SYSTEM PROMPT — the whole point is that this never moves."

func cachePrefixCallID(i int) string {
	return fmt.Sprintf("%s_cacheprefix%02d", ids.PrefixCall, i)
}

// cachePrefixReasoningID is shaped like the ids internal/protocol/ids mints
// (<prefix>_<32 hex>), because that is what Codex replays back to us. The
// prefix comes from that package so a change to it fails the compile here
// rather than silently making the fixture unrepresentative.
func cachePrefixReasoningID(i int) string {
	return fmt.Sprintf("%s_%032x", ids.PrefixReasoning, 0xcace0000+i)
}

// cachePrefixCacheableBlocks is how many history blocks can carry a breakpoint
// in a conversation of n turns: the user text and the tool result of each.
func cachePrefixCacheableBlocks(turns int) int { return turns * 2 }

// ─── what a prompt cache actually sees ────────────────────────────────────────

// cachePrefixView is the part of a captured upstream request an upstream
// prompt cache is keyed on: the instruction/system prefix, the tool
// definitions, and the ordered conversation items — with every prompt-cache
// directive stripped, because those are hints *about* the prefix, not part of
// the content being cached. Two requests whose views differ cannot share a
// cache entry past the first difference.
type cachePrefixView struct {
	Prefix string
	Tools  string
	Items  []string
}

// stripCacheDirectives removes every prompt-cache hint, in either protocol
// family's spelling, from a decoded request body.
func stripCacheDirectives(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, child := range node {
			if slices.Contains(protocol.PromptCacheHintFields, k) {
				continue
			}
			out[k] = stripCacheDirectives(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(node))
		for _, child := range node {
			out = append(out, stripCacheDirectives(child))
		}
		return out
	default:
		return v
	}
}

// canonicalJSON renders a value with map keys sorted (encoding/json's own
// ordering), so two structurally equal bodies compare equal as strings.
func canonicalJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(raw)
}

func capturedCachePrefixView(t flagTB, env *TestEnv, target protocol.APIType, label string) cachePrefixView {
	t.Helper()
	return cachePrefixViewOf(requireLastRequest(t, env, target, label).JSON(), cacheControlEndpoint(target))
}

func cachePrefixViewOf(body map[string]any, endpoint EndpointKind) cachePrefixView {
	clean, _ := stripCacheDirectives(body).(map[string]any)
	if clean == nil {
		clean = map[string]any{}
	}

	view := cachePrefixView{Tools: canonicalJSON(clean["tools"])}
	var items []any
	switch endpoint {
	case EndpointAnthropic:
		view.Prefix = canonicalJSON(clean["system"])
		items, _ = clean["messages"].([]any)
	case EndpointChat:
		// Chat carries the system prompt as messages[0]; keeping it in Items
		// is exactly right — a system message that changes shape shifts every
		// item after it, which is the failure this section exists to catch.
		items, _ = clean["messages"].([]any)
	case EndpointResponses:
		view.Prefix = canonicalJSON(clean["instructions"])
		items, _ = clean["input"].([]any)
	}
	for _, item := range items {
		view.Items = append(view.Items, canonicalJSON(item))
	}
	return view
}

// requireSharedPrefix asserts the second turn still shares the first's cached
// prefix: same instruction/system text, same tool definitions, and the items
// they have in common byte-identical. The label already says which property is
// under test, so both callers use the same wording.
func requireSharedPrefix(t flagTB, want, got cachePrefixView, label string) {
	t.Helper()
	if want.Prefix != got.Prefix {
		t.Errorf("%s: instruction/system prefix changed\n  before: %s\n  after:  %s",
			label, truncate(want.Prefix, 600), truncate(got.Prefix, 600))
	}
	if want.Tools != got.Tools {
		t.Errorf("%s: tool definitions changed\n  before: %s\n  after:  %s",
			label, truncate(want.Tools, 600), truncate(got.Tools, 600))
	}
	requireItemPrefix(t, want.Items, got.Items, label)
}

// requireIdenticalPrefix is the rotation property: moving a breakpoint must
// leave the request whole.
func requireIdenticalPrefix(t flagTB, want, got cachePrefixView, label string) {
	t.Helper()
	requireSharedPrefix(t, want, got, label)
	if len(want.Items) != len(got.Items) {
		t.Errorf("%s: item count changed: %d -> %d", label, len(want.Items), len(got.Items))
	}
}

// requireGrowthPreservesPrefix is the growth property: a longer turn still
// replays the shorter turn's history byte-identically.
func requireGrowthPreservesPrefix(t flagTB, short, long cachePrefixView, label string) {
	t.Helper()
	if len(long.Items) < len(short.Items) {
		t.Errorf("%s: turn N+1 sent %d items, fewer than turn N's %d — history was dropped, not appended to",
			label, len(long.Items), len(short.Items))
		return
	}
	requireSharedPrefix(t, short, long, label)
}

// requireItemPrefix reports the first item position at which two histories
// diverge — the position an upstream cache would stop matching at.
func requireItemPrefix(t flagTB, want, got []string, label string) {
	t.Helper()
	n := min(len(want), len(got))
	for i := 0; i < n; i++ {
		if want[i] == got[i] {
			continue
		}
		t.Errorf("%s: conversation item %d of %d changed — an upstream cache stops matching here\n  before: %s\n  after:  %s",
			label, i, len(want), truncate(want[i], 600), truncate(got[i], 600))
		return
	}
}

// ─── affinity ─────────────────────────────────────────────────────────────────

// capturedCacheKey returns the upstream affinity hint the gateway attached:
// prompt_cache_key in the body, or the session-id header at the Codex
// boundary. Anthropic has no such field — its cache is addressed purely by
// prefix — so an Anthropic target legitimately returns "".
func capturedCacheKey(t flagTB, env *TestEnv, target protocol.APIType, label string) (bodyKey, headerKey string) {
	t.Helper()
	captured := requireLastRequest(t, env, target, label)
	bodyKey, _ = captured.JSON()["prompt_cache_key"].(string)
	return bodyKey, captured.Headers.Get("session-id")
}

// ─── case bodies ──────────────────────────────────────────────────────────────

func cachePrefixScenario() Scenario {
	s := TextScenario()
	s.Name = "cache_prefix"
	s.Description = "Shared fixture for cross-request prompt-cache prefix checks"
	s.Assertions = nil
	return s
}

// cachePrefixRun is one configured case: everything that is fixed for the
// duration of it, so the per-turn calls carry only what actually varies.
type cachePrefixRun struct {
	env       *TestEnv
	client    cachePrefixClient
	target    protocol.APIType
	scenario  string
	model     string
	streaming bool
	// codex is set for the Codex-boundary case, where the request passes
	// through the real Codex RoundTripper and the ChatGPT backend's own wire
	// rules and session-id affinity header apply on top.
	codex bool
	base  string
}

func (r cachePrefixRun) send(t flagTB, turns, breakpointAt int, sessionID string) {
	t.Helper()
	path, _ := buildRequest(r.client.source, r.model, r.streaming)
	body := r.client.build(r.model, r.streaming, turns, breakpointAt, sessionID)
	if _, err := r.env.dispatch(r.client.source, r.target, r.scenario, path, mustMarshal(body), nil, r.streaming); err != nil {
		t.Fatalf("dispatch %s %s -> %s (turns=%d, breakpoint=%d, streaming=%v): %v",
			r.client.name, r.client.source, r.target, turns, breakpointAt, r.streaming, err)
	}
}

func (r cachePrefixRun) capture(t flagTB, suffix string) cachePrefixView {
	t.Helper()
	return capturedCachePrefixView(t, r.env, r.target, r.base+suffix)
}

// runCachePrefixCase drives one client shape against one target and checks all
// three properties against the request the final provider actually received.
func runCachePrefixCase(t flagTB, env *TestEnv, c cachePrefixClient, target protocol.APIType, streaming bool) {
	t.Helper()
	s := cachePrefixScenario()
	env.SetupRoute(c.source, target, s)
	model := env.findRouteModel(c.source, target, s.Name)
	if model == "" {
		t.Fatalf("%s: %s -> %s route model not configured", c.name, c.source, target)
	}
	runCachePrefixChecks(t, newCachePrefixRun(env, c, target, s.Name, model, streaming, false))
}

func newCachePrefixRun(env *TestEnv, c cachePrefixClient, target protocol.APIType,
	scenario, model string, streaming, codex bool) cachePrefixRun {
	return cachePrefixRun{
		env: env, client: c, target: target, scenario: scenario,
		model: model, streaming: streaming, codex: codex,
		base: fmt.Sprintf("%s/%s→%s/%s", c.name, c.source, target, streamMode(streaming)),
	}
}

// runCachePrefixChecks is the assertion body, shared by the generic-provider
// case and the Codex-boundary case (which only differs in how the route was
// provisioned). codexBoundary additionally asserts the Codex wire rules.
func runCachePrefixChecks(t flagTB, r cachePrefixRun) {
	t.Helper()
	blocks := cachePrefixCacheableBlocks(cachePrefixTurns)

	// Rotation: the same history, with the client's breakpoint parked on the
	// last cacheable block and then on the one before it — what happens
	// naturally as a client's fixed pool of breakpoints rolls forward.
	r.send(t, cachePrefixTurns, blocks-1, cachePrefixSessionID)
	rotationBaseline := r.capture(t, "/rotation")
	if r.client.rotates {
		for _, at := range []int{blocks - 2, 0, -1} {
			r.send(t, cachePrefixTurns, at, cachePrefixSessionID)
			requireIdenticalPrefix(t, rotationBaseline, r.capture(t, "/rotation"),
				fmt.Sprintf("%s/rotation/breakpoint=%d", r.base, at))
		}
	}

	// Growth: turn N+1 appends one exchange and rolls the breakpoint onto it.
	// Everything turn N sent must still arrive byte-identical.
	r.send(t, cachePrefixTurns, blocks-1, cachePrefixSessionID)
	shortView := r.capture(t, "/growth")
	r.send(t, cachePrefixTurns+1, cachePrefixCacheableBlocks(cachePrefixTurns+1)-1, cachePrefixSessionID)
	requireGrowthPreservesPrefix(t, shortView, r.capture(t, "/growth"), r.base+"/growth")

	// Affinity: the conversation identity must reach the upstream, stably, and
	// must distinguish one conversation from another.
	assertCachePrefixAffinity(t, r)

	if r.codex {
		assertCodexCacheWireRules(t, r.env, r.base)
	}
}

// assertCachePrefixAffinity checks the upstream affinity hint: stable within a
// conversation, different across conversations, and absent when there is
// nothing to derive it from.
func assertCachePrefixAffinity(t flagTB, r cachePrefixRun) {
	t.Helper()
	label := r.base + "/affinity"

	// Anthropic has no cache-key field; its cache is addressed purely by
	// prefix, which the rotation and growth checks above already cover. Both
	// OpenAI shapes carry prompt_cache_key.
	wantKey := r.client.carriesAffinity && cacheControlEndpoint(r.target) != EndpointAnthropic

	r.send(t, cachePrefixTurns, 0, cachePrefixSessionID)
	firstBody, firstHeader := capturedCacheKey(t, r.env, r.target, label)

	r.send(t, cachePrefixTurns+1, 1, cachePrefixSessionID)
	secondBody, secondHeader := capturedCacheKey(t, r.env, r.target, label)

	if !wantKey {
		if firstBody != "" {
			t.Errorf("%s: %s target carries prompt_cache_key %q, but this protocol has no such field",
				label, r.target, firstBody)
		}
		return
	}

	if firstBody == "" {
		t.Errorf("%s: no prompt_cache_key reached the upstream — a byte-identical prefix can still miss the cache it never routed to",
			label)
		return
	}
	if firstBody != secondBody {
		t.Errorf("%s: prompt_cache_key changed between turns of one conversation: %q -> %q",
			label, firstBody, secondBody)
	}

	r.send(t, cachePrefixTurns, 0, cachePrefixOtherSessionID)
	otherBody, _ := capturedCacheKey(t, r.env, r.target, label)
	if otherBody == firstBody {
		t.Errorf("%s: two different conversations share prompt_cache_key %q — they would contend for one cache slot",
			label, firstBody)
	}

	if !r.codex {
		return
	}
	// At the Codex boundary the header is what ChatGPT keys affinity on.
	if firstHeader == "" {
		t.Errorf("%s: no session-id header reached the Codex backend (prompt_cache_key was %q); "+
			"ChatGPT derives Responses cache affinity from that header", label, firstBody)
	}
	if firstHeader != secondHeader {
		t.Errorf("%s: session-id header changed between turns of one conversation: %q -> %q",
			label, firstHeader, secondHeader)
	}
	if firstHeader != "" && firstHeader != firstBody {
		t.Errorf("%s: session-id header %q disagrees with prompt_cache_key %q — one conversation, two affinity scopes",
			label, firstHeader, firstBody)
	}
}

// assertCodexCacheWireRules checks what the ChatGPT Codex backend requires of
// the dispatched body, all of which the cache depends on: it rejects
// prompt-cache directives outright, and rejects role="system" input messages,
// so the system prompt has to arrive in `instructions` — identically, however
// the converter chose to represent it upstream of this boundary.
func assertCodexCacheWireRules(t flagTB, env *TestEnv, base string) {
	t.Helper()
	captured := requireLastRequest(t, env, protocol.TypeOpenAIResponses, base+"/codex")
	body := captured.JSON()
	raw := truncate(string(captured.Body), 1200)

	if n := countCacheMarkers(t, body, "prompt_cache_breakpoint", "mode", "explicit"); n != 0 {
		t.Errorf("%s/codex: %d prompt_cache_breakpoint marker(s) reached the Codex backend, which rejects them; body=%s",
			base, n, raw)
	}
	for _, key := range []string{"prompt_cache_options", "prompt_cache_retention"} {
		if _, ok := body[key]; ok {
			t.Errorf("%s/codex: %s reached the Codex backend, which rejects it; body=%s", base, key, raw)
		}
	}
	if instructions, _ := body["instructions"].(string); instructions == "" {
		t.Errorf("%s/codex: no instructions reached the Codex backend; body=%s", base, raw)
	}
	input, _ := body["input"].([]any)
	for i, item := range input {
		obj, _ := item.(map[string]any)
		if role, _ := obj["role"].(string); role == "system" {
			t.Errorf("%s/codex: input[%d] is a role=\"system\" message, which the Codex backend rejects — "+
				"it must be lifted into instructions; body=%s", base, i, raw)
		}
	}
}

// runCodexBoundaryCachePrefixCase runs the same three properties against a
// provider wired with a Codex OAuth identity, so the request passes through the
// real Codex RoundTripper (body filtering, system lift, affinity header) on its
// way out — the path an actual Codex-subscription provider takes.
func runCodexBoundaryCachePrefixCase(t flagTB, env *TestEnv, c cachePrefixClient, streaming bool) {
	t.Helper()
	s := cachePrefixScenario()
	s.Name = "cache_prefix_codex"
	target := protocol.TypeOpenAIResponses
	env.SetupCodexAssemblyRoute(c.source, s)
	model := env.findRouteModel(c.source, target, s.Name)
	if model == "" {
		t.Fatalf("%s: Codex route model not configured", c.name)
	}
	runCachePrefixChecks(t, newCachePrefixRun(env, c, target, s.Name, model, streaming, true))
}

// ExecuteAllCachePrefix runs the cross-request prompt-cache section. Name
// formats:
//
//   - cache_prefix/generic/<client>→<target>/{stream|nonstream}
//   - cache_prefix/codex/<client>/{stream|nonstream}
func (m *Matrix) ExecuteAllCachePrefix() []TestResult {
	var cases []recorderCase
	for _, c := range cachePrefixClients() {
		for _, target := range m.cachePrefixTargets() {
			for _, streaming := range m.Streaming {
				c, target, streaming := c, target, streaming
				cases = append(cases, recorderCase{
					name: fmt.Sprintf("cache_prefix/generic/%s→%s/%s",
						c.name, target, streamMode(streaming)),
					scenario:  "cache_prefix/generic",
					source:    c.source,
					target:    target,
					streaming: streaming,
					run: func(t flagTB, env *TestEnv) {
						runCachePrefixCase(t, env, c, target, streaming)
					},
				})
			}
		}

		for _, streaming := range m.Streaming {
			c, streaming := c, streaming
			cases = append(cases, recorderCase{
				name:      fmt.Sprintf("cache_prefix/codex/%s/%s", c.name, streamMode(streaming)),
				scenario:  "cache_prefix/codex",
				source:    c.source,
				target:    protocol.TypeOpenAIResponses,
				streaming: streaming,
				run: func(t flagTB, env *TestEnv) {
					runCodexBoundaryCachePrefixCase(t, env, c, streaming)
				},
			})
		}
	}
	return m.runRecorderCases(cases)
}

// cachePrefixTargets is the distinct set of upstream protocols the configured
// pairs reach. A prefix bug lives in a converter, which is chosen by the
// target, so the section iterates targets rather than source/target pairs —
// the source is supplied by the client fixture.
func (m *Matrix) cachePrefixTargets() []protocol.APIType {
	seen := map[protocol.APIType]bool{}
	var targets []protocol.APIType
	for _, pair := range m.Pairs {
		if seen[pair.Target] || cacheControlEndpoint(pair.Target) == "" {
			continue
		}
		seen[pair.Target] = true
		targets = append(targets, pair.Target)
	}
	return targets
}
