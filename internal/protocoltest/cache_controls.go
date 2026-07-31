package protocoltest

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// cache_controls is the request-side prompt-cache regression section shared by
// go test and `harness matrix --mode=cache_controls`.
//
// The normal scenario matrix deliberately sends one fixed plain-text request,
// so it cannot prove that cache markers survive conversion. These cases send
// bespoke cache/no-cache requests through the real gateway and inspect the
// request captured by the final VirtualServer provider.
//
// Two path classes are covered:
//
//   - single: every supported A -> B pair in Matrix.Pairs;
//   - ABA: every idempotent A -> B -> A path, using setupChainHopRoute so the
//     first conversion re-enters the gateway through B's real HTTP endpoint.

const cacheControlExpectedMarkers = 2

func cacheControlScenario() Scenario {
	s := TextScenario()
	s.Name = "cache_controls"
	s.Description = "Shared fixture for request-side prompt-cache conversion checks"
	s.Assertions = nil
	return s
}

func cacheControlIdempotentCases() []IdempotentCase {
	cases := append([]IdempotentCase(nil), DefaultIdempotentCases()...)
	cases = append(cases,
		IdempotentCase{
			Name:     "anthropic_beta_via_openai_chat",
			Source:   protocol.TypeAnthropicBeta,
			Mid:      protocol.TypeOpenAIChat,
			Baseline: protocol.TypeAnthropicBeta,
		},
		IdempotentCase{
			Name:     "anthropic_beta_via_openai_responses",
			Source:   protocol.TypeAnthropicBeta,
			Mid:      protocol.TypeOpenAIResponses,
			Baseline: protocol.TypeAnthropicBeta,
		},
	)
	return cases
}

// cacheControlBody returns the same stable system + user prefix in each source
// protocol. cached controls both markers and OpenAI's explicit cache mode.
func cacheControlBody(source protocol.APIType, model string, streaming, cached bool) map[string]any {
	cacheControl := map[string]any{"type": "ephemeral"}
	breakpoint := map[string]any{"mode": "explicit"}

	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		system := map[string]any{"type": "text", "text": "stable system prefix"}
		user := map[string]any{"type": "text", "text": "stable conversation prefix"}
		if cached {
			system["cache_control"] = cacheControl
			user["cache_control"] = cacheControl
		}
		return map[string]any{
			"model":      model,
			"max_tokens": 64,
			"stream":     streaming,
			"system":     []map[string]any{system},
			"messages": []map[string]any{{
				"role":    "user",
				"content": []map[string]any{user},
			}},
		}

	case protocol.TypeOpenAIChat:
		system := map[string]any{"type": "text", "text": "stable system prefix"}
		user := map[string]any{"type": "text", "text": "stable conversation prefix"}
		body := map[string]any{
			"model":  model,
			"stream": streaming,
			"messages": []map[string]any{
				{"role": "system", "content": []map[string]any{system}},
				{"role": "user", "content": []map[string]any{user}},
			},
		}
		if cached {
			system["prompt_cache_breakpoint"] = breakpoint
			user["prompt_cache_breakpoint"] = breakpoint
			body["prompt_cache_options"] = map[string]any{"mode": "explicit"}
		}
		return body

	case protocol.TypeOpenAIResponses:
		system := map[string]any{"type": "input_text", "text": "stable system prefix"}
		user := map[string]any{"type": "input_text", "text": "stable conversation prefix"}
		body := map[string]any{
			"model":  model,
			"stream": streaming,
			"input": []map[string]any{
				{"type": "message", "role": "system", "content": []map[string]any{system}},
				{"type": "message", "role": "user", "content": []map[string]any{user}},
			},
		}
		if cached {
			system["prompt_cache_breakpoint"] = breakpoint
			user["prompt_cache_breakpoint"] = breakpoint
			body["prompt_cache_options"] = map[string]any{"mode": "explicit"}
		}
		return body
	}
	panic(fmt.Sprintf("unsupported cache-control source protocol %s", source))
}

// automaticCacheControlBody exercises the request-level/implicit cache mode:
// Anthropic names it top-level cache_control, while OpenAI names it
// prompt_cache_options.mode="implicit". Unlike cacheControlBody, it contains
// no explicit content breakpoint.
func automaticCacheControlBody(source protocol.APIType, model string, streaming bool) map[string]any {
	body := cacheControlBody(source, model, streaming, false)
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		body["cache_control"] = map[string]any{"type": "ephemeral"}
	case protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		body["prompt_cache_options"] = map[string]any{"mode": "implicit"}
	default:
		panic(fmt.Sprintf("unsupported automatic cache-control source protocol %s", source))
	}
	return body
}

func cacheControlEndpoint(target protocol.APIType) EndpointKind {
	switch target {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return EndpointAnthropic
	case protocol.TypeOpenAIChat:
		return EndpointChat
	case protocol.TypeOpenAIResponses:
		return EndpointResponses
	default:
		return ""
	}
}

func sendCacheControlBody(t flagTB, env *TestEnv, source, target protocol.APIType, scenarioName, model string, streaming, cached bool) {
	t.Helper()
	path, _ := buildRequest(source, model, streaming)
	_, err := env.dispatch(source, target, scenarioName, path,
		mustMarshal(cacheControlBody(source, model, streaming, cached)), nil, streaming)
	if err != nil {
		t.Fatalf("dispatch %s -> %s (cached=%v, streaming=%v): %v",
			source, target, cached, streaming, err)
	}
}

func sendAutomaticCacheControlBody(t flagTB, env *TestEnv, source, target protocol.APIType, scenarioName, model string, streaming bool) {
	t.Helper()
	path, _ := buildRequest(source, model, streaming)
	_, err := env.dispatch(source, target, scenarioName, path,
		mustMarshal(automaticCacheControlBody(source, model, streaming)), nil, streaming)
	if err != nil {
		t.Fatalf("dispatch automatic cache %s -> %s (streaming=%v): %v",
			source, target, streaming, err)
	}
}

// countCacheMarkers recursively counts markerKey and validates every marker's
// protocol-specific discriminator. Invalid marker values are reported as
// errors but still counted, keeping missing-vs-malformed diagnostics separate.
func countCacheMarkers(t flagTB, value any, markerKey, discriminator, wantValue string) int {
	t.Helper()
	count := 0
	var walk func(any)
	walk = func(v any) {
		switch node := v.(type) {
		case map[string]any:
			for key, child := range node {
				if key == markerKey {
					count++
					marker, ok := child.(map[string]any)
					if !ok {
						t.Errorf("%s marker is %T, want object", markerKey, child)
					} else if got, _ := marker[discriminator].(string); got != wantValue {
						t.Errorf("%s.%s = %q, want %q", markerKey, discriminator, got, wantValue)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(value)
	return count
}

func assertCapturedCacheState(t flagTB, env *TestEnv, target protocol.APIType, cached bool, label string) {
	t.Helper()
	endpoint := cacheControlEndpoint(target)
	if endpoint == "" {
		t.Fatalf("%s: unsupported target protocol %s", label, target)
	}

	captured := env.virtual.LastRequest(endpoint)
	if captured == nil {
		t.Fatalf("%s: final provider received no %s request", label, endpoint)
	}
	body := captured.JSON()

	markerKey := "prompt_cache_breakpoint"
	discriminator := "mode"
	wantValue := "explicit"
	if endpoint == EndpointAnthropic {
		markerKey = "cache_control"
		discriminator = "type"
		wantValue = "ephemeral"
	}

	gotMarkers := countCacheMarkers(t, body, markerKey, discriminator, wantValue)
	wantMarkers := 0
	if cached {
		wantMarkers = cacheControlExpectedMarkers
	}
	if gotMarkers != wantMarkers {
		t.Errorf("%s: final %s marker count = %d, want %d; body=%s",
			label, markerKey, gotMarkers, wantMarkers, truncate(string(captured.Body), 1200))
	}

	unexpectedKey := "cache_control"
	unexpectedDiscriminator := "type"
	unexpectedValue := "ephemeral"
	if endpoint == EndpointAnthropic {
		unexpectedKey = "prompt_cache_breakpoint"
		unexpectedDiscriminator = "mode"
		unexpectedValue = "explicit"
	}
	if unexpected := countCacheMarkers(t, body, unexpectedKey, unexpectedDiscriminator, unexpectedValue); unexpected != 0 {
		t.Errorf("%s: final provider body contains %d non-native %s marker(s); body=%s",
			label, unexpected, unexpectedKey, truncate(string(captured.Body), 1200))
	}

	if endpoint == EndpointAnthropic {
		return
	}
	rawOptions, hasOptions := body["prompt_cache_options"]
	if hasOptions != cached {
		t.Errorf("%s: final prompt_cache_options presence = %v, want %v; body=%s",
			label, hasOptions, cached, truncate(string(captured.Body), 1200))
	}
	options, _ := rawOptions.(map[string]any)
	mode, _ := options["mode"].(string)
	wantMode := ""
	if cached {
		wantMode = "explicit"
	}
	if mode != wantMode {
		t.Errorf("%s: final prompt_cache_options.mode = %q, want %q; body=%s",
			label, mode, wantMode, truncate(string(captured.Body), 1200))
	}
}

func assertCapturedAutomaticCacheState(t flagTB, env *TestEnv, target protocol.APIType, label string) {
	t.Helper()
	endpoint := cacheControlEndpoint(target)
	if endpoint == "" {
		t.Fatalf("%s: unsupported target protocol %s", label, target)
	}
	captured := env.virtual.LastRequest(endpoint)
	if captured == nil {
		t.Fatalf("%s: final provider received no %s request", label, endpoint)
	}
	body := captured.JSON()

	if endpoint == EndpointAnthropic {
		raw, ok := body["cache_control"]
		if !ok {
			t.Errorf("%s: final Anthropic body has no top-level cache_control; body=%s",
				label, truncate(string(captured.Body), 1200))
			return
		}
		control, _ := raw.(map[string]any)
		if got, _ := control["type"].(string); got != "ephemeral" {
			t.Errorf("%s: final cache_control.type = %q, want ephemeral; body=%s",
				label, got, truncate(string(captured.Body), 1200))
		}
		// Automatic caching must not invent explicit content breakpoints.
		delete(body, "cache_control")
		if explicit := countCacheMarkers(t, body, "cache_control", "type", "ephemeral"); explicit != 0 {
			t.Errorf("%s: final Anthropic body contains %d explicit cache marker(s); body=%s",
				label, explicit, truncate(string(captured.Body), 1200))
		}
		return
	}

	options, _ := body["prompt_cache_options"].(map[string]any)
	if got, _ := options["mode"].(string); got != "implicit" {
		t.Errorf("%s: final prompt_cache_options.mode = %q, want implicit; body=%s",
			label, got, truncate(string(captured.Body), 1200))
	}
	if explicit := countCacheMarkers(t, body, "prompt_cache_breakpoint", "mode", "explicit"); explicit != 0 {
		t.Errorf("%s: final OpenAI body contains %d explicit cache marker(s); body=%s",
			label, explicit, truncate(string(captured.Body), 1200))
	}
}

func runSingleCacheControlCase(t flagTB, env *TestEnv, pair ProtocolPair, streaming bool) {
	t.Helper()
	s := cacheControlScenario()
	env.SetupRoute(pair.Source, pair.Target, s)
	model := env.findRouteModel(pair.Source, pair.Target, s.Name)
	if model == "" {
		t.Fatalf("single %s -> %s: route model not configured", pair.Source, pair.Target)
	}

	for _, cached := range []bool{true, false} {
		label := fmt.Sprintf("single/%s→%s/%s/%s",
			pair.Source, pair.Target, cacheStateName(cached), streamMode(streaming))
		sendCacheControlBody(t, env, pair.Source, pair.Target, s.Name, model, streaming, cached)
		assertCapturedCacheState(t, env, pair.Target, cached, label)
	}
	label := fmt.Sprintf("single/%s→%s/automatic/%s",
		pair.Source, pair.Target, streamMode(streaming))
	sendAutomaticCacheControlBody(t, env, pair.Source, pair.Target, s.Name, model, streaming)
	assertCapturedAutomaticCacheState(t, env, pair.Target, label)
}

func runABACacheControlCase(t flagTB, env *TestEnv, ic IdempotentCase, streaming bool) {
	t.Helper()
	s := cacheControlScenario()

	// Tail: B -> A through the final VirtualServer provider.
	env.SetupRoute(ic.Mid, ic.Baseline, s)
	tailModel := env.findRouteModel(ic.Mid, ic.Baseline, s.Name)
	if tailModel == "" {
		t.Fatalf("ABA %s -> %s -> %s: tail route model not configured",
			ic.Source, ic.Mid, ic.Baseline)
	}

	// Head: A -> B and re-enter this gateway carrying tailModel.
	headModel := fmt.Sprintf("cache-aba-%s", ic.Name)
	env.setupChainHopRoute(ic.Source, ic.Mid, s, headModel, tailModel)

	for _, cached := range []bool{true, false} {
		label := fmt.Sprintf("aba/%s→%s→%s/%s/%s",
			ic.Source, ic.Mid, ic.Baseline, cacheStateName(cached), streamMode(streaming))
		sendCacheControlBody(t, env, ic.Source, ic.Mid, s.Name, headModel, streaming, cached)
		assertCapturedCacheState(t, env, ic.Baseline, cached, label)
	}
	label := fmt.Sprintf("aba/%s→%s→%s/automatic/%s",
		ic.Source, ic.Mid, ic.Baseline, streamMode(streaming))
	sendAutomaticCacheControlBody(t, env, ic.Source, ic.Mid, s.Name, headModel, streaming)
	assertCapturedAutomaticCacheState(t, env, ic.Baseline, label)
}

func cacheStateName(cached bool) string {
	if cached {
		return "cache"
	}
	return "no_cache"
}

// ExecuteAllCacheControls runs single-hop and ABA prompt-cache request checks.
// Each result validates both the positive cache case and the negative no-cache
// case. Name formats:
//
//   - cache_controls/single/A→B/{stream|nonstream}
//   - cache_controls/aba/A→B→A/{stream|nonstream}
func (m *Matrix) ExecuteAllCacheControls() []TestResult {
	var cases []recorderCase
	for _, pair := range m.Pairs {
		for _, streaming := range m.Streaming {
			pair := pair
			streaming := streaming
			name := fmt.Sprintf("cache_controls/single/%s→%s/%s",
				pair.Source, pair.Target, streamMode(streaming))
			cases = append(cases, recorderCase{
				name:      name,
				scenario:  "cache_controls/single",
				source:    pair.Source,
				target:    pair.Target,
				streaming: streaming,
				run: func(t flagTB, env *TestEnv) {
					runSingleCacheControlCase(t, env, pair, streaming)
				},
			})
		}
	}

	for _, ic := range cacheControlIdempotentCases() {
		for _, streaming := range m.Streaming {
			ic := ic
			streaming := streaming
			name := fmt.Sprintf("cache_controls/aba/%s→%s→%s/%s",
				ic.Source, ic.Mid, ic.Baseline, streamMode(streaming))
			cases = append(cases, recorderCase{
				name:      name,
				scenario:  "cache_controls/aba",
				source:    ic.Source,
				target:    ic.Mid,
				streaming: streaming,
				run: func(t flagTB, env *TestEnv) {
					runABACacheControlCase(t, env, ic, streaming)
				},
			})
		}
	}
	return m.runRecorderCases(cases)
}
