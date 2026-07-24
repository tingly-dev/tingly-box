package protocoltest

import (
	"fmt"
	"slices"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// This file is the request-content-shape regression suite, shared by both the
// go-test entry point (TestContentShapes) and the CLI
// (`harness matrix --mode=content_shapes`). It is deliberately separate from
// the protocol matrix: every matrix request carries the same fixed single-turn
// prompt (see client.go's harnessPrompt) and the Scenario only controls the
// mocked upstream *response* shape — so the matrix, by design, cannot catch
// bugs in how the gateway forwards unusual *request* content shapes upstream.
//
// This suite closes that gap for the array-of-text-blocks content form
// (`content: [{"type":"text","text":"..."}]` — valid per the OpenAI spec and
// emitted by several agent frameworks instead of a plain string) on tool,
// assistant, and system messages: the shape that shipped upstream as an empty
// string in issue #1427. Each case drives one bespoke multi-turn request
// through the real gateway and asserts on the request actually forwarded
// upstream (VirtualServer.LastRequest) — not the parsed response — the same
// technique the rule-flag suite (flags.go) uses. See
// .design/harness-matrix.md section 9.

// contentShapeScenario is the shared fixture every case routes through. Its
// mock response is irrelevant to what these cases assert (the forwarded
// *request*, not the response), so any text-capable scenario works; a
// dedicated name just keeps its routes from colliding with other suites'.
func contentShapeScenario() Scenario {
	s := TextScenario()
	s.Name = "content_shapes"
	s.Description = "Shared fixture for request content-shape regression tests"
	s.Assertions = nil
	return s
}

// contentShapeCase is one request-content-shape regression case. It reuses
// flagTB/flagRecorder/flagAbort from flags.go so cases run under both
// *testing.T and the CLI without duplicating that plumbing.
type contentShapeCase struct {
	name string
	run  func(t flagTB, env *TestEnv)
}

// sendContentShape wires a route for (source, target), stamps the resolved
// request model onto body, and sends it through the gateway. body must not
// already set "model". Fails the case on transport errors.
func sendContentShape(t flagTB, env *TestEnv, source, target protocol.APIType, body map[string]any) *RoundTripResult {
	t.Helper()
	s := contentShapeScenario()
	env.SetupRoute(source, target, s)
	model := env.findRouteModel(source, target, s.Name)
	if model == "" {
		t.Fatalf("no route configured for source=%s target=%s", source, target)
	}
	body["model"] = model

	path, _ := buildRequest(source, model, false) // only the path is reused; body is bespoke
	res, err := env.dispatch(source, target, s.Name, path, mustMarshal(body), nil, false)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return res
}

// assertUpstreamText sends body through the gateway, extracts a field from
// the captured upstream request via extract, and asserts it equals want.
// CapturedRequest.JSON() is nil-safe (vmodel/benchmark/capture.go), so no
// separate "was anything captured" check is needed — an absent request
// simply makes every extractor return ok=false, which fails the assertion
// with an informative message on its own.
func assertUpstreamText(t flagTB, env *TestEnv, source, target protocol.APIType, endpoint EndpointKind, body map[string]any, extract func(map[string]any) (string, bool), want string) {
	t.Helper()
	sendContentShape(t, env, source, target, body)
	out, ok := extract(env.virtual.LastRequest(endpoint).JSON())
	if !ok || out != want {
		t.Errorf("upstream %s text = %q (ok=%v), want %q", endpoint, out, ok, want)
	}
}

// ─── OpenAI Responses body helpers ────────────────────────────────────────

// responsesFunctionCallOutput returns the "output" string of the first
// function_call_output item in a captured OpenAI Responses request body.
func responsesFunctionCallOutput(body map[string]any) (string, bool) {
	input, _ := body["input"].([]any)
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call_output" {
			out, ok := item["output"].(string)
			return out, ok
		}
	}
	return "", false
}

// responsesMessageContent returns the "content" string of the first input
// item with the given role in a captured OpenAI Responses request body.
func responsesMessageContent(body map[string]any, role string) (string, bool) {
	input, _ := body["input"].([]any)
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		if item["type"] == "message" && item["role"] == role {
			content, ok := item["content"].(string)
			return content, ok
		}
	}
	return "", false
}

// responsesReasoningEffort returns the "reasoning.effort" string in a
// captured OpenAI Responses request body.
func responsesReasoningEffort(body map[string]any) (string, bool) {
	reasoning, _ := body["reasoning"].(map[string]any)
	effort, ok := reasoning["effort"].(string)
	return effort, ok
}

// ─── Anthropic Beta body helpers ──────────────────────────────────────────

// anthropicToolResultText returns the text of the first tool_result block
// found across a captured Anthropic Beta request body's messages.
func anthropicToolResultText(body map[string]any) (string, bool) {
	msgs, _ := body["messages"].([]any)
	for _, raw := range msgs {
		msg, _ := raw.(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, rawBlock := range blocks {
			block, _ := rawBlock.(map[string]any)
			if block["type"] != "tool_result" {
				continue
			}
			parts, _ := block["content"].([]any)
			if len(parts) == 0 {
				return "", false
			}
			first, _ := parts[0].(map[string]any)
			text, ok := first["text"].(string)
			return text, ok
		}
	}
	return "", false
}

// anthropicMessageText returns the text of the first content block of the
// first message with the given role in a captured Anthropic Beta request body.
func anthropicMessageText(body map[string]any, role string) (string, bool) {
	msgs, _ := body["messages"].([]any)
	for _, raw := range msgs {
		msg, _ := raw.(map[string]any)
		if msg["role"] != role {
			continue
		}
		blocks, _ := msg["content"].([]any)
		if len(blocks) == 0 {
			return "", false
		}
		first, _ := blocks[0].(map[string]any)
		text, ok := first["text"].(string)
		return text, ok
	}
	return "", false
}

// anthropicSystemText returns the joined text of the "system" field in a
// captured Anthropic Beta request body, whether it arrived as a plain string
// or as an array of text blocks.
func anthropicSystemText(body map[string]any) (string, bool) {
	switch v := body["system"].(type) {
	case string:
		return v, v != ""
	case []any:
		if len(v) == 0 {
			return "", false
		}
		first, _ := v[0].(map[string]any)
		text, ok := first["text"].(string)
		return text, ok
	}
	return "", false
}

// ─── Cases ─────────────────────────────────────────────────────────────────

func contentShapeCases() []contentShapeCase {
	const secretWord = "The secret word is ZANZIBAR"
	const parisAnswer = "The capital of France is Paris."
	const systemPrompt = "You are a helpful assistant."

	toolCallTurns := []map[string]any{
		{"role": "user", "content": "What's the secret word?"},
		{"role": "assistant", "tool_calls": []map[string]any{
			{"id": "call_1", "type": "function", "function": map[string]any{"name": "get_secret", "arguments": "{}"}},
		}},
	}
	// slices.Concat copies rather than mutating toolCallTurns' backing array,
	// which matters because both cases below share it under t.Parallel().
	toolResultBody := func() map[string]any {
		return map[string]any{
			"messages": slices.Concat(toolCallTurns, []map[string]any{
				{"role": "tool", "tool_call_id": "call_1",
					"content": []map[string]any{{"type": "text", "text": secretWord}}},
			}),
		}
	}
	// assistantArrayBody/systemArrayBody are funcs, not shared maps: each of
	// the two cases below runs under t.Parallel() and sendContentShape
	// mutates its body (stamps "model" on it), so a shared map instance would
	// race between the Chat→Responses and Chat→Anthropic cases that both use
	// this fixture.
	assistantArrayBody := func() map[string]any {
		return map[string]any{
			"messages": []map[string]any{
				{"role": "user", "content": "What is the capital of France?"},
				{"role": "assistant", "content": []map[string]any{{"type": "text", "text": parisAnswer}}},
			},
		}
	}
	systemArrayBody := func() map[string]any {
		return map[string]any{
			"messages": []map[string]any{
				{"role": "system", "content": []map[string]any{{"type": "text", "text": systemPrompt}}},
				{"role": "user", "content": "Hello"},
			},
		}
	}

	assistantContent := func(body map[string]any) (string, bool) { return responsesMessageContent(body, "assistant") }
	instructions := func(body map[string]any) (string, bool) {
		v, ok := body["instructions"].(string)
		return v, ok
	}
	anthropicAssistantText := func(body map[string]any) (string, bool) { return anthropicMessageText(body, "assistant") }

	return []contentShapeCase{
		// ── Chat → Responses ────────────────────────────────────────────
		{name: "chat_to_responses/tool_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, EndpointResponses,
				toolResultBody(), responsesFunctionCallOutput, secretWord)
		}},

		{name: "chat_to_responses/assistant_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, EndpointResponses,
				assistantArrayBody(), assistantContent, parisAnswer)
		}},

		{name: "chat_to_responses/system_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, EndpointResponses,
				systemArrayBody(), instructions, systemPrompt)
		}},

		{name: "chat_to_responses/reasoning_effort_forwarded", run: func(t flagTB, env *TestEnv) {
			body := map[string]any{
				"messages":         []map[string]any{{"role": "user", "content": "Hello"}},
				"reasoning_effort": "high",
			}
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, EndpointResponses,
				body, responsesReasoningEffort, "high")
		}},

		// ── Chat → Anthropic Beta ───────────────────────────────────────
		{name: "chat_to_anthropic/tool_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, EndpointAnthropic,
				toolResultBody(), anthropicToolResultText, secretWord)
		}},

		{name: "chat_to_anthropic/assistant_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, EndpointAnthropic,
				assistantArrayBody(), anthropicAssistantText, parisAnswer)
		}},

		{name: "chat_to_anthropic/system_array_content", run: func(t flagTB, env *TestEnv) {
			assertUpstreamText(t, env, protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, EndpointAnthropic,
				systemArrayBody(), anthropicSystemText, systemPrompt)
		}},
	}
}

// ─── CLI execution (harness matrix --mode=content_shapes) ────────────────

// ExecuteAllContentShapes runs the request-content-shape regression suite
// without requiring testing.T. It is the CLI-compatible counterpart of
// TestContentShapes, returning []TestResult. Name format:
// "content_shapes/<case name>".
func (m *Matrix) ExecuteAllContentShapes() []TestResult {
	results := make([]TestResult, 0, len(contentShapeCases()))
	for _, c := range contentShapeCases() {
		results = append(results, runContentShapeCaseCLI(c))
	}
	return results
}

func runContentShapeCaseCLI(c contentShapeCase) TestResult {
	res := TestResult{Name: "content_shapes/" + c.name, Scenario: c.name}
	start := time.Now()

	env, err := NewTestEnvForCLI()
	if err != nil {
		res.Errors = []AssertionError{{Assertion: "setup", Error: fmt.Sprintf("create test env: %v", err)}}
		res.Duration = time.Since(start)
		return res
	}
	defer env.Close()

	// Reuses flagRecorder/flagAbort from flags.go: same flagTB contract, same
	// "run under a testing.T-free recorder" trick, no need to duplicate it.
	rec := &flagRecorder{}
	func() {
		defer func() {
			rec.runCleanups()
			if r := recover(); r != nil && r != flagAbort {
				panic(r)
			}
		}()
		c.run(rec, env)
	}()

	res.Errors = rec.errs
	res.Passed = len(rec.errs) == 0
	res.Duration = time.Since(start)
	return res
}
