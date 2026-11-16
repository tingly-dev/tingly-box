package protocoltest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the server-tool (MCP) section: the gateway-owned tool loop and
// its composition with Guardrails, driven through the real HTTP gateway. The
// same case bodies back the go tests (mcp_loop_test.go,
// guardrails_mcp_test.go) and the CLI (`harness matrix --mode=servertool`).
// Each case is keyed by its go test name, which is also the key of the
// known-gap registry (known_gaps.go), so both entry points judge a case the
// same way: a registered case that fails is a known gap, a registered case
// that passes is a failure.

// serverToolCase is one case: the go (sub)test it runs as, the CLI row
// metadata, and the body, which returns the violated expectations (empty =
// pass) and a detail dump shown for unexpected failures.
type serverToolCase struct {
	test      string // top-level go test name
	sub       string // subtest name; "" when the go test has no subtests
	scenario  string // CLI scenario column
	source    protocol.APIType
	target    protocol.APIType
	streaming bool
	run       func(t flagTB) (failures []string, detail string)
}

// key is the case's go test name (t.Name()) and known-gap registry key.
func (c serverToolCase) key() string {
	if c.sub == "" {
		return c.test
	}
	return c.test + "/" + c.sub
}

// caseBody is a per-(source, target, streaming) case body.
type caseBody func(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string)

// pairCases expands body over pairs and streaming modes, naming subtests
// "<source>-><target>/stream=<bool>".
func pairCases(test, scenario string, pairs []ProtocolPair, streams []bool, body caseBody) []serverToolCase {
	var cases []serverToolCase
	for _, pair := range pairs {
		for _, streaming := range streams {
			cases = append(cases, serverToolCase{
				test:      test,
				sub:       fmt.Sprintf("%s->%s/stream=%v", pair.Source, pair.Target, streaming),
				scenario:  scenario,
				source:    pair.Source,
				target:    pair.Target,
				streaming: streaming,
				run: func(t flagTB) ([]string, string) {
					return body(t, pair.Source, pair.Target, streaming)
				},
			})
		}
	}
	return cases
}

// crossPairs is the (source, target) product.
func crossPairs(sources, targets []protocol.APIType) []ProtocolPair {
	var pairs []ProtocolPair
	for _, s := range sources {
		for _, tg := range targets {
			pairs = append(pairs, ProtocolPair{Source: s, Target: tg})
		}
	}
	return pairs
}

// pairsWithTarget filters DefaultPairs to one target protocol.
func pairsWithTarget(target protocol.APIType) []ProtocolPair {
	var pairs []ProtocolPair
	for _, p := range DefaultPairs() {
		if p.Target == target {
			pairs = append(pairs, p)
		}
	}
	return pairs
}

var (
	bothStreamModes = []bool{false, true}
	streamOnly      = []bool{true}

	// Pairs on which the server-tool loop runs today, for the error /
	// round-limit / continuation cases; which pairs have one at all is
	// pinned by TestMCPOwnedToolLoop (see mcpSupportedPair).
	toolLoopSources = []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
	toolLoopTargets = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}

	// Guardrails x MCP targets. Responses targets are left out of the block
	// cases: Anthropic clients on them are never offered server tools (M3).
	guardrailsMCPTargets = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
	guardrailsTargets    = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses}
)

// serverToolCases lists every case of the section, in output order.
func serverToolCases() []serverToolCase {
	var cases []serverToolCase
	add := func(cs ...serverToolCase) { cases = append(cases, cs...) }

	add(pairCases("TestMCPOwnedToolLoop", "owned_tool_loop", DefaultPairs(), bothStreamModes, ownedToolLoopCase)...)
	add(serverToolCase{
		test: "TestMCPOwnedToolNotOfferedWhenDisabled", scenario: "owned_tool_disabled",
		source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta,
		run: ownedToolNotOfferedWhenDisabledCase,
	})
	toolLoop := crossPairs(toolLoopSources, toolLoopTargets)
	add(pairCases("TestMCPServerToolError", "server_tool_error", toolLoop, bothStreamModes, serverToolErrorCase)...)
	add(pairCases("TestMCPToolLoopBounded", "round_limit", toolLoop, bothStreamModes, toolLoopBoundedCase)...)
	add(pairCases("TestMCPMixedToolContinuation", "mixed_tool_continuation", toolLoop, bothStreamModes, mixedToolContinuationCase)...)
	add(pairCases("TestMCPTruncatedToolStream", "truncated_tool_stream", DefaultPairs(), streamOnly, truncatedToolStreamCase)...)
	add(pairCases("TestMCPToolInputOnBlockStart", "tool_input_on_block_start", pairsWithTarget(protocol.TypeAnthropicBeta), streamOnly, toolInputOnBlockStartCase)...)
	add(pairCases("TestMCPNoFailoverAfterServerTool", "no_failover_after_server_tool", DefaultPairs(), bothStreamModes, noFailoverAfterServerToolCase)...)

	add(pairCases("TestGuardrailsBlocksServerTool", "guardrails_blocks_server_tool", crossPairs(anthropicSources, guardrailsMCPTargets), bothStreamModes, guardrailsBlocksServerToolCase)...)
	add(pairCases("TestGuardrailsBlocksClientToolAfterServerRound", "guardrails_blocks_client_tool", crossPairs(anthropicSources, guardrailsMCPTargets), bothStreamModes, guardrailsBlocksClientToolAfterServerRoundCase)...)
	add(pairCases("TestGuardrailsCredentialAliasClientTool", "credential_alias_client_tool", crossPairs(anthropicSources, guardrailsTargets), bothStreamModes, credentialAliasClientToolCase)...)
	add(remoteMCPHarnessCases()...)
	add(routingMCPHarnessCases()...)
	return cases
}

// ─── Entry points ────────────────────────────────────────────────────────────

// ExecuteAllServerTool runs the server-tool section without a testing.T and
// returns one TestResult per case. Name format: "servertool/<case key>".
// Each case boots its own env, so cases run concurrently up to
// sectionParallelism; results keep case order.
func (m *Matrix) ExecuteAllServerTool() []TestResult {
	cases := serverToolCases()
	results := make([]TestResult, len(cases))
	runIndexed(len(cases), m.sectionParallelism(), func(i int) {
		results[i] = runServerToolCaseCLI(cases[i])
	})
	return results
}

// runServerToolCaseCLI executes one case under a flagRecorder and judges it
// against the known-gap registry.
func runServerToolCaseCLI(c serverToolCase) TestResult {
	res := TestResult{
		Name:      "servertool/" + c.key(),
		Scenario:  c.scenario,
		Source:    c.source,
		Target:    c.target,
		Streaming: c.streaming,
	}
	start := time.Now()

	rec := &flagRecorder{}
	var failures []string
	var detail string
	func() {
		defer func() {
			rec.runCleanups()
			if r := recover(); r != nil && r != flagAbort {
				panic(r)
			}
		}()
		failures, detail = c.run(rec)
	}()
	for _, e := range rec.errs {
		failures = append(failures, e.Error)
	}

	errs, gap := judgeCase(c.key(), failures)
	switch {
	case gap != nil:
		res.KnownGap = gap.ID
		for _, f := range failures {
			res.Errors = append(res.Errors, AssertionError{Assertion: "known gap " + gap.ID, Error: f, Context: gap.Reason})
		}
	default:
		for _, e := range errs {
			res.Errors = append(res.Errors, AssertionError{Assertion: "servertool", Error: e, Context: detail})
		}
	}
	res.Passed = len(failures) == 0 && len(errs) == 0
	res.Duration = time.Since(start)
	return res
}

// runServerToolTest is the go test entry point: it runs the cases of one go
// test as parallel subtests named exactly like their registry keys.
func runServerToolTest(t *testing.T, test string) {
	t.Helper()
	t.Parallel()

	for _, c := range serverToolCases() {
		if c.test != test {
			continue
		}
		if c.sub == "" {
			runServerToolCaseT(t, c)
			continue
		}
		t.Run(c.sub, func(t *testing.T) {
			t.Parallel()
			runServerToolCaseT(t, c)
		})
	}
}

func runServerToolCaseT(t *testing.T, c serverToolCase) {
	t.Helper()
	if t.Name() != c.key() {
		t.Fatalf("case key %q does not match test name %q", c.key(), t.Name())
	}
	failures, detail := c.run(t)
	checkCase(t, c.key(), failures, detail)
}

// ─── Shared helpers ──────────────────────────────────────────────────────────

// newCaseEnv boots a fresh env for one case, closed by t's cleanup.
func newCaseEnv(t flagTB, opts ...TestEnvOption) *TestEnv {
	t.Helper()
	env, err := NewTestEnvForCLI(opts...)
	if err != nil {
		t.Fatalf("create test env: %v", err)
	}
	t.Cleanup(env.Close)
	return env
}

// routedRequest wires (source, target, scenario) and returns the gateway path
// and the standard request body for it.
func routedRequest(env *TestEnv, source, target protocol.APIType, scenario Scenario, streaming bool) (string, []byte, string) {
	env.SetupRoute(source, target, scenario)
	model := env.findRouteModel(source, target, scenario.Name)
	path, body := buildRequest(source, model, streaming)
	return path, body, model
}

// mcpSupportedPair reports whether the gateway runs server tools for a pair.
// On the other OpenAI pairs (Chat on Responses, Responses on Chat or
// Responses) there is no tool loop, so MCP is skipped: the request passes
// through without server tools. Every case that runs over such a pair asserts
// that skip policy (mcpSkippedFailures) instead of tool-round behavior.
func mcpSupportedPair(source, target protocol.APIType) bool {
	openAI := func(api protocol.APIType) bool {
		return api == protocol.TypeOpenAIChat || api == protocol.TypeOpenAIResponses
	}
	if openAI(source) && openAI(target) {
		return source == protocol.TypeOpenAIChat && target == protocol.TypeOpenAIChat
	}
	return true
}

// mcpSkippedFailures lists how a pair without MCP violates "the server tool
// is neither offered, executed nor leaked; the provider answers once, and the
// client gets that answer".
func mcpSkippedFailures(t flagTB, env *TestEnv, target protocol.APIType, echo *EchoServertoolProvider, status int, raw string) []string {
	t.Helper()
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	if upstream := string(requireLastRequest(t, env, target, "mcp skipped").Body); strings.Contains(upstream, OwnedToolWireName) {
		failures = append(failures, "server tool offered upstream on a pair without MCP")
	}
	if n := len(echo.Calls()); n != 0 {
		failures = append(failures, fmt.Sprintf("server tool executed %d times, want 0", n))
	}
	if got := env.VirtualCallCount(); got != 1 {
		failures = append(failures, fmt.Sprintf("upstream calls = %d, want 1", got))
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	return failures
}

// clientSawError reports whether the client was told the request failed: a
// non-2xx status, or an error event / error object in the response.
func clientSawError(status int, raw string) bool {
	return status < 200 || status >= 300 ||
		strings.Contains(raw, `"type":"error"`) ||
		strings.Contains(raw, `"error":{`) ||
		strings.Contains(raw, `response.failed`)
}

// ─── Server-tool loop cases ──────────────────────────────────────────────────

// ownedToolLoopCase pins the server-owned tool loop for every protocol pair:
// the gateway offers its tool, executes the model's call exactly once with
// the model's arguments, continues the conversation, and returns only the
// final answer — the client never sees the server tool.
func ownedToolLoopCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, _ := routedRequest(env, source, target, OwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	if !mcpSupportedPair(source, target) {
		return mcpSkippedFailures(t, env, target, echo, status, raw), "client response:\n" + raw
	}
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	failures = append(failures, echoRanOnceWithX(echo)...)
	if got := env.VirtualCallCount(); got != 2 {
		failures = append(failures, fmt.Sprintf("upstream calls = %d, want 2 (tool round + final round)", got))
	}
	if !strings.Contains(raw, OwnedToolFinalText) {
		failures = append(failures, "final answer missing from client response")
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	// While the server tool runs the client hears a keep-alive, so
	// idle-timeout proxies do not drop the stream - on every pair the Stage
	// pipeline serves.
	if streaming && stagePipelinePair(source, target) &&
		!strings.Contains(raw, ": keep-alive") {
		failures = append(failures, "no keep-alive while the server tool ran")
	}
	return failures, "client response:\n" + raw
}

// echoRanOnceWithX checks the echo tool ran exactly once, with q=x.
func echoRanOnceWithX(echo *EchoServertoolProvider) []string {
	calls := echo.Calls()
	switch {
	case len(calls) != 1:
		return []string{fmt.Sprintf("server tool executed %d times, want 1", len(calls))}
	case calls[0]["q"] != "x":
		return []string{fmt.Sprintf("server tool args = %v, want q=x", calls[0])}
	}
	return nil
}

// ownedToolNotOfferedWhenDisabledCase pins that without the MCP extension
// the gateway neither offers nor executes server tools.
func ownedToolNotOfferedWhenDisabledCase(t flagTB) ([]string, string) {
	env := newCaseEnv(t)
	path, body, _ := routedRequest(env, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, OwnedToolScenario(), false)

	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	upstream := ""
	if last := env.virtual.LastRequest(EndpointAnthropic); last != nil {
		upstream = string(last.Body)
	}
	if strings.Contains(upstream, OwnedToolWireName) {
		failures = append(failures, "server tool offered upstream with MCP disabled")
	}
	if got := env.VirtualCallCount(); got != 1 {
		failures = append(failures, fmt.Sprintf("upstream calls = %d, want 1", got))
	}
	return failures, "client response:\n" + raw + "\nupstream request:\n" + upstream
}

// serverToolErrorCase pins that a failing server tool is reported back to
// the model (which then answers) rather than aborting the request, and that
// the failure is not retried.
func serverToolErrorCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := &EchoServertoolProvider{Fail: true}
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, _ := routedRequest(env, source, target, OwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	if n := len(echo.Calls()); n != 1 {
		failures = append(failures, fmt.Sprintf("failing server tool executed %d times, want 1", n))
	}
	if last := env.virtual.LastRequest(cacheControlEndpoint(target)); last == nil || !strings.Contains(string(last.Body), ownedToolErrorText) {
		failures = append(failures, "tool failure was not reported to the model")
	}
	if !strings.Contains(raw, OwnedToolFinalText) {
		failures = append(failures, "final answer missing from client response")
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	return failures, "client response:\n" + raw
}

// maxToolRounds is InterceptorConfig.MaxRounds at every call site.
const maxToolRounds = 3

// toolLoopBoundedCase pins that a model which keeps calling a server tool
// cannot hold the request forever, and that the loop's cut-off never hands
// the server tool call to the client.
func toolLoopBoundedCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, _ := routedRequest(env, source, target, AlwaysOwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	if n := len(echo.Calls()); n > maxToolRounds {
		failures = append(failures, fmt.Sprintf("server tool executed %d times, limit %d", n, maxToolRounds))
	}
	if n := env.VirtualCallCount(); n > maxToolRounds+1 {
		failures = append(failures, fmt.Sprintf("upstream calls = %d, limit %d", n, maxToolRounds+1))
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client at the round limit")
	}
	return failures, fmt.Sprintf("tool calls=%d upstream=%d client response:\n%s", len(echo.Calls()), env.VirtualCallCount(), raw)
}

// followUpWithClientToolResult builds the client's next request in its own
// protocol: the original question, the assistant turn carrying the client
// tool call it received (id toolID), and that tool's result.
func followUpWithClientToolResult(source protocol.APIType, model, toolID string, streaming bool) (string, []byte) {
	path, _ := buildRequest(source, model, streaming)
	switch source {
	case protocol.TypeOpenAIChat:
		return path, mustMarshal(map[string]any{
			"model": model, "stream": streaming,
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": nil, "tool_calls": []map[string]any{{
					"id": toolID, "type": "function", "function": map[string]any{"name": clientToolName, "arguments": `{"location":"Paris"}`},
				}}},
				{"role": "tool", "tool_call_id": toolID, "content": clientToolResultText},
			},
		})
	default: // Anthropic V1 / Beta share the message shape
		return path, mustMarshal(map[string]any{
			"model": model, "max_tokens": 1024, "stream": streaming,
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": []map[string]any{{
					"type": "tool_use", "id": toolID, "name": clientToolName, "input": map[string]any{"location": "Paris"},
				}}},
				{"role": "user", "content": []map[string]any{{
					"type": "tool_result", "tool_use_id": toolID, "content": clientToolResultText,
				}}},
			},
		})
	}
}

// mixedToolContinuationCase pins the two-request mixed round: the server
// tool runs in the first request and is hidden, the client receives only its
// own tool call, and when it returns that result the gateway resumes the
// provider conversation with the stored server-tool result spliced back in.
func mixedToolContinuationCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, model := routedRequest(env, source, target, MixedToolScenario(), streaming)
	headers := map[string]string{"X-Tingly-Session-ID": fmt.Sprintf("mixed-%s-%s-%v", source, target, streaming)}

	status, first := sendRawWithHeaders(t, env, path, body, headers)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("first request status = %d", status))
	}
	if n := len(echo.Calls()); n != 1 {
		failures = append(failures, fmt.Sprintf("server tool executed %d times in the mixed round, want 1", n))
	}
	if strings.Contains(first, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client in the mixed round")
	}
	toolID := ""
	for _, id := range []string{"toolu-client-tool", "call-client-tool"} {
		if strings.Contains(first, id) {
			toolID = id
		}
	}
	if toolID == "" {
		failures = append(failures, "client tool call missing from the mixed round")
		return failures, "first response:\n" + first
	}

	path, body = followUpWithClientToolResult(source, model, toolID, streaming)
	status, second := sendRawWithHeaders(t, env, path, body, headers)
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("follow-up status = %d", status))
	}
	if last := env.virtual.LastRequest(cacheControlEndpoint(target)); last == nil || !strings.Contains(string(last.Body), ownedToolResultText) {
		failures = append(failures, "follow-up did not carry the stored server-tool result upstream")
	}
	if !strings.Contains(second, OwnedToolFinalText) {
		failures = append(failures, "final answer missing from follow-up response")
	}
	if n := len(echo.Calls()); n != 1 {
		failures = append(failures, fmt.Sprintf("server tool executed %d times in total, want 1", n))
	}
	return failures, "first response:\n" + first + "\nfollow-up response:\n" + second
}

// truncatedToolStreamCase pins that a provider stream cut inside a server
// tool call — no terminal event — is a failed round: the half-received call
// is not executed, it does not reach the client, and the client is told the
// request failed instead of getting a clean-looking end of stream.
func truncatedToolStreamCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, _ := routedRequest(env, source, target, TruncatedOwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	if !mcpSupportedPair(source, target) {
		return mcpSkippedFailures(t, env, target, echo, status, raw), "client response:\n" + raw
	}
	var failures []string
	if n := len(echo.Calls()); n != 0 {
		failures = append(failures, fmt.Sprintf("server tool executed %d times from a truncated call, want 0", n))
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "truncated server tool call leaked to client")
	}
	if !clientSawError(status, raw) {
		failures = append(failures, fmt.Sprintf("client got no error for the truncated stream (status = %d)", status))
	}
	return failures, fmt.Sprintf("status=%d upstream=%d client response:\n%s", status, env.VirtualCallCount(), raw)
}

// toolInputOnBlockStartCase pins that a server tool call whose input arrives
// complete on content_block_start (no input_json_delta) runs exactly once
// with that input and the turn completes.
func toolInputOnBlockStartCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	path, body, _ := routedRequest(env, source, target, BlockStartInputOwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	if !mcpSupportedPair(source, target) {
		return mcpSkippedFailures(t, env, target, echo, status, raw), "client response:\n" + raw
	}
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	failures = append(failures, echoRanOnceWithX(echo)...)
	if !strings.Contains(raw, OwnedToolFinalText) {
		failures = append(failures, "final answer missing from client response")
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	return failures, fmt.Sprintf("tool calls=%v client response:\n%s", echo.Calls(), raw)
}

// scriptedProvider is a provider endpoint that serves OwnedToolScenario's
// tool round while the echo tool is offered and not yet called, and fails
// every other request with a retryable 503. It counts the requests it gets.
type scriptedProvider struct {
	srv     *httptest.Server
	calls   atomic.Int64
	offered atomic.Bool // some request offered the echo tool
}

func startOwnedToolThenFailProvider(t flagTB) *scriptedProvider {
	t.Helper()
	owned := OwnedToolScenario().MockResponses
	p := &scriptedProvider{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), OwnedToolWireName) {
			p.offered.Store(true)
		}
		format := FormatOpenAIChat
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			format = FormatAnthropic
		case strings.HasSuffix(r.URL.Path, "/responses"):
			format = FormatOpenAIResponses
		}
		if callsOwnedTool(body) {
			var req struct {
				Stream bool `json:"stream"`
			}
			_ = json.Unmarshal(body, &req)
			if builder := owned[format]; req.Stream {
				sse.WriteSSEResponse(w, builder.StreamFor(body))
			} else {
				status, out := builder.NonStreamFor(body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write(out)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(mustMarshal(map[string]any{"type": "error", "error": map[string]any{"type": "overloaded_error", "message": "service A unavailable"}}))
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// setupServerToolFailoverRoute wires a two-tier rule for (source, target):
// tier 0 is service A (startOwnedToolThenFailProvider), tier 1 is service B,
// env.virtual serving OwnedToolScenario. Returns the request model and A.
func setupServerToolFailoverRoute(t flagTB, env *TestEnv, source, target protocol.APIType) (string, *scriptedProvider) {
	t.Helper()
	a := startOwnedToolThenFailProvider(t)
	scenario := OwnedToolScenario()
	env.virtual.RegisterScenario(scenario)

	apiStyle := targetToAPIStyle(target)
	aBase, bBase := a.srv.URL, env.virtual.URL()
	if apiStyle == protocol.APIStyleOpenAI {
		aBase, bBase = aBase+"/v1", bBase+"/v1"
	}
	requestModel := fmt.Sprintf("st-fo-%s-to-%s", source, target)
	aUUID, bUUID := requestModel+"-a", requestModel+"-b"
	bModel := "virtual-model-" + scenario.Name
	for _, p := range []*typ.Provider{
		{UUID: aUUID, Name: aUUID, APIBase: aBase, Token: "a-token"},
		{UUID: bUUID, Name: bUUID, APIBase: bBase, Token: "b-token"},
	} {
		p.APIStyle = apiStyle
		p.OpenAIEndpointMode = targetToOpenAIEndpointMode(target)
		p.Enabled = true
		p.Timeout = int64(constant.DefaultRequestTimeout)
		if err := env.appConfig.AddProvider(p); err != nil {
			t.Fatalf("add provider %s: %v", p.UUID, err)
		}
	}
	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, bModel,
		tieredService(aUUID, "service-a-model", 0),
		tieredService(bUUID, bModel, 1))
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add failover rule: %v", err)
	}
	return requestModel, a
}

// noFailoverAfterServerToolCase pins that once a server tool has run, a
// retryable failure of a later round does not fail over to another service:
// replaying the request elsewhere would run the tool again (side effects) on
// a conversation the other service never saw. The server tool ran exactly
// once, service B gets nothing, and the client gets the error. On a pair
// without MCP no tool runs, so the ordinary failover to B is expected.
func noFailoverAfterServerToolCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
	model, a := setupServerToolFailoverRoute(t, env, source, target)
	path, body := buildRequest(source, model, streaming)

	status, raw := sendRaw(t, env, path, body)
	detail := fmt.Sprintf("status=%d serviceA=%d serviceB=%d tool calls=%d client response:\n%s",
		status, a.calls.Load(), env.VirtualCallCount(), len(echo.Calls()), raw)
	if !mcpSupportedPair(source, target) {
		// No tool ran, so failing over is correct: A's 503 moves the request
		// to B, which answers once without server tools.
		failures := mcpSkippedFailures(t, env, target, echo, status, raw)
		if a.offered.Load() {
			failures = append(failures, "server tool offered to service A on a pair without MCP")
		}
		return failures, detail
	}
	var failures []string
	if n := len(echo.Calls()); n != 1 {
		failures = append(failures, fmt.Sprintf("server tool executed %d times in total, want 1", n))
	}
	if n := env.VirtualCallCount(); n != 0 {
		failures = append(failures, fmt.Sprintf("service B received %d requests after the server tool ran, want 0", n))
	}
	if !clientSawError(status, raw) {
		failures = append(failures, fmt.Sprintf("client got no error for the failed round (status = %d)", status))
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	return failures, detail
}

// ─── Guardrails x MCP cases ──────────────────────────────────────────────────
//
// The intended order is one decision point per tool call: guardrails first,
// then ownership — a blocked server tool never runs, and a client tool
// produced after server rounds is still checked.

// guardrailsBlocksServerToolCase pins that a server-owned tool call blocked
// by guardrails is not executed and the client gets the block message.
func guardrailsBlocksServerToolCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t,
		NewTestEnvOptionWithServertoolProviders(echo),
		NewTestEnvOptionWithGuardrails(newBlockToolNamedGuardrails("echo")),
	)
	path, body, _ := routedRequest(env, source, target, OwnedToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	if n := len(echo.Calls()); n != 0 {
		failures = append(failures, fmt.Sprintf("blocked server tool executed %d times", n))
	}
	// The block message may name the refused command; only a tool_use block
	// for the server tool is a leak.
	if strings.Contains(raw, ownedToolUseMarker) {
		failures = append(failures, "server tool call leaked to client")
	}
	if !strings.Contains(raw, "Blocked by guardrails") {
		failures = append(failures, "no block message in response")
	}
	return failures, "client response:\n" + raw
}

// guardrailsBlocksClientToolAfterServerRoundCase pins the composition order:
// the server tool runs (allowed), and the client tool the model calls in the
// next round is blocked before it reaches the client.
func guardrailsBlocksClientToolAfterServerRoundCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t,
		NewTestEnvOptionWithServertoolProviders(echo),
		NewTestEnvOptionWithGuardrails(newBlockToolNamedGuardrails(clientToolName)),
	)
	path, body, _ := routedRequest(env, source, target, OwnedThenClientToolScenario(), streaming)

	status, raw := sendRaw(t, env, path, body)
	failures := blockedToolUseFailures(status, raw)
	if n := len(echo.Calls()); n != 1 {
		failures = append(failures, fmt.Sprintf("allowed server tool executed %d times, want 1", n))
	}
	if strings.Contains(raw, clientToolName) && !strings.Contains(raw, "Blocked by guardrails") {
		failures = append(failures, "blocked client tool leaked to client")
	}
	return failures, "client response:\n" + raw
}

const (
	credentialSecret = "sk-protocoltest-secret"
	credentialAlias  = "TINGLY_CRED_TOKEN_PROTOCOLTEST"
)

// credentialAliasClientToolCase pins how credential masking composes with
// response checks and the tool loop. The client sends a protected
// credential, which Guardrails masks before it goes upstream; after a server
// tool round the model calls a client tool with the alias as input; a
// response policy blocks the real secret. The check must see what the model
// produced (the alias), so the response is not blocked, and the client's
// tool input must carry the real value, restored after the check.
func credentialAliasClientToolCase(t flagTB, source, target protocol.APIType, streaming bool) ([]string, string) {
	echo := NewEchoServertoolProvider()
	env := newCaseEnv(t,
		NewTestEnvOptionWithServertoolProviders(echo),
		NewTestEnvOptionWithGuardrails(newBlockResponseContainingGuardrails(credentialSecret)),
	)
	installProtectedCredential(env, credentialSecret, credentialAlias)
	path, body, _ := routedRequest(env, source, target, CredentialAliasClientToolScenario(credentialAlias), streaming)
	// The alias is only registered once the request actually carries the
	// secret and request-side guardrails mask it.
	body = []byte(strings.Replace(string(body), "What is the capital of France?", "use "+credentialSecret+" for the weather", 1))

	status, raw := sendRaw(t, env, path, body)
	var failures []string
	if status != http.StatusOK {
		failures = append(failures, fmt.Sprintf("status = %d", status))
	}
	upstream := ""
	if last := env.virtual.LastRequest(cacheControlEndpoint(target)); last != nil {
		upstream = string(last.Body)
	}
	if strings.Contains(upstream, credentialSecret) {
		failures = append(failures, "secret reached upstream unmasked")
	}
	if strings.Contains(raw, "Blocked by guardrails") {
		failures = append(failures, "response blocked: the check saw the restored secret instead of the model's masked output")
	}
	if !strings.Contains(raw, clientToolName) {
		failures = append(failures, "client tool call missing from client response")
	}
	if !strings.Contains(raw, credentialSecret) {
		failures = append(failures, "client tool input does not carry the real credential")
	}
	if strings.Contains(raw, credentialAlias) {
		failures = append(failures, "credential alias leaked to client")
	}
	if strings.Contains(raw, OwnedToolWireName) {
		failures = append(failures, "server tool call leaked to client")
	}
	return failures, fmt.Sprintf("tool calls=%d client response:\n%s", len(echo.Calls()), raw)
}

// ownedToolUseMarker appears only in a tool_use block for the server tool;
// a block message may name the tool in plain text.
const ownedToolUseMarker = `"name":"` + OwnedToolWireName + `"`

// stagePipelinePair reports whether the Protocol Stage pipeline serves the
// pair: Anthropic clients on any provider, OpenAI clients on Anthropic.
func stagePipelinePair(source, target protocol.APIType) bool {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return true
	}
	return target == protocol.TypeAnthropicBeta || target == protocol.TypeAnthropicV1
}
