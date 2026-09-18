package protocoltest

import (
	"fmt"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// vendor_transforms is the vendor-dispatch regression section shared by go
// test and `harness matrix --mode=vendor`.
//
// The rest of the harness (cache_controls, content_shapes, flags, the
// protocol matrix itself) dispatches every case to a generic httptest URL
// standing in for "some OpenAI-compatible provider." That's the right fixture
// for protocol-shape fidelity, but it is blind to ApplyProviderTransforms /
// VendorTransform (internal/protocol/ops, internal/protocol/transform),
// which key their behavior off the destination's real APIBase — api.openai.com,
// api.deepseek.com, generativelanguage.googleapis.com, and so on. cache_controls
// discovered this the hard way: its generic destination is correctly off the
// explicit-prompt-cache allowlist (see supportsExplicitPromptCache), so it can
// only prove markers are stripped for an unrecognized vendor, never that they
// survive for a recognized one.
//
// This section closes that gap: it builds virtual providers whose APIBase
// genuinely matches a vendor discriminator, routes traffic to them through a
// local relay standing in for the vendor host (newVendorHostProxy) so the
// bytes still land on the real VirtualServer, and asserts the transform
// outcome end-to-end — exercising the real dispatch table in
// ApplyProviderTransforms, not a hand-built request struct.
//
// Today this only covers the explicit-prompt-cache allowlist (the case that
// motivated the section). Extending it to other vendor-specific transforms
// (DeepSeek's reasoning_content flip, Gemini's thinking_config mapping, tool
// schema filtering, ...) means adding fields to vendorFixture and assertions
// in runVendorTransformCase — the routing/relay plumbing is already generic.

// vendorFixture is one destination identity ApplyProviderTransforms
// recognizes (or deliberately doesn't), plus the outcomes this suite checks
// for that identity.
type vendorFixture struct {
	name    string // subtest name
	apiBase string // fake but realistic — must (or must not) match ApplyProviderTransforms's dispatch

	// wantsExplicitPromptCache is whether prompt_cache_options /
	// prompt_cache_breakpoint should survive to this vendor's wire request.
	// Only api.openai.com is allowlisted today — see supportsExplicitPromptCache
	// in internal/protocol/ops/request_openai_extensions.go.
	wantsExplicitPromptCache bool
}

var vendorFixtures = []vendorFixture{
	{name: "openai_official", apiBase: "http://api.openai.com", wantsExplicitPromptCache: true},
	{name: "generic_openai_compatible", apiBase: "http://example-llm-provider.test", wantsExplicitPromptCache: false},
	{name: "deepseek", apiBase: "http://api.deepseek.com", wantsExplicitPromptCache: false},
	// NVIDIA NIM rejects the whole request over top-level prompt-cache
	// fields (400 "Unsupported parameter(s): `prompt_cache_options`",
	// #1548) — the vendor that motivated default-deny in the first place.
	{name: "nvidia_nim", apiBase: "http://integrate.api.nvidia.com", wantsExplicitPromptCache: false},
}

func vendorTransformScenario() Scenario {
	s := TextScenario()
	s.Name = "vendor_transforms"
	s.Description = "Shared fixture for vendor-dispatch (ApplyProviderTransforms) end-to-end checks"
	s.Assertions = nil
	return s
}

// newVendorHostProxy starts a local forward proxy that relays every request
// through it to target, preserving method/path/query/headers/body/streaming.
// Pointing a provider's ProxyURL at this relay while its APIBase is a real
// vendor host (e.g. "http://api.openai.com/v1") makes the vendor-identity
// string genuinely reach ApplyProviderTransforms's matcher — the same
// Provider.APIBase field production dials — while the bytes actually go to
// the local VirtualServer. No production code changes, no real network
// egress to the vendor's real host.
func newVendorHostProxy(t flagTB, target string) string {
	t.Helper()
	targetURL, err := url.Parse(target)
	if err != nil {
		t.Fatalf("newVendorHostProxy: parse target %q: %v", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.FlushInterval = -1 // flush every write immediately — the cache_controls SSE cases need it

	relay := httptest.NewServer(proxy)
	t.Cleanup(relay.Close)
	return relay.URL
}

// setupVendorRoute wires an Anthropic v1 -> OpenAI Chat route whose
// destination provider carries fx's real APIBase (dialed, via the relay in
// newVendorHostProxy, to the VirtualServer), and returns the request model to
// send to.
func setupVendorRoute(t flagTB, env *TestEnv, s Scenario, fx vendorFixture, streaming bool) string {
	source := protocol.TypeAnthropicV1
	target := protocol.TypeOpenAIChat
	proxyURL := newVendorHostProxy(t, env.virtual.URL())
	// UUID must be unique per (fixture, streaming): GetGlobalTransportPool is
	// a process-global cache keyed on provider UUID + model, not APIBase/
	// ProxyURL. Two concurrently-running cases that happened to share a UUID
	// (e.g. this fixture's stream and nonstream cases both landing on
	// "vendor-<fx>-vendor_transforms") could race on AddProvider's pool
	// invalidation and end up dialing through each other's (possibly
	// already-closed) relay — exactly the "final provider received no chat
	// request" flake this caused before the streaming suffix was added.
	uuid := fmt.Sprintf("vendor-%s-%s-%s", fx.name, s.Name, streamMode(streaming))
	env.setupRouteCore(source, target, s, nil, func(p *typ.Provider) {
		p.UUID = uuid
		p.Name = uuid
		p.APIBase = fx.apiBase + "/v1"
		p.ProxyURL = proxyURL
	})
	return env.findRouteModel(source, target, s.Name)
}

// runVendorTransformCase sends a cached and a no-cache request through fx's
// route and asserts the explicit-prompt-cache allowlist behaved as fx
// declares.
func runVendorTransformCase(t flagTB, env *TestEnv, fx vendorFixture, streaming bool) {
	t.Helper()
	s := vendorTransformScenario()
	source := protocol.TypeAnthropicV1
	target := protocol.TypeOpenAIChat
	model := setupVendorRoute(t, env, s, fx, streaming)
	if model == "" {
		t.Fatalf("vendor/%s: route model not configured", fx.name)
	}

	for _, cached := range []bool{true, false} {
		label := fmt.Sprintf("vendor/%s/%s/%s", fx.name, cacheStateName(cached), streamMode(streaming))
		sendCacheControlBody(t, env, source, target, s.Name, model, streaming, cached)
		wantCached := cached && fx.wantsExplicitPromptCache
		assertCapturedCacheState(t, env, target, wantCached, label)
		// The wire shape of text content follows the same allowlist: a vendor
		// that cannot take the breakpoints has no use for the content-part
		// array either, and some reject it outright. See
		// compactOpenAIChatTextContent.
		assertCapturedChatTextShape(t, env, !fx.wantsExplicitPromptCache, label)
	}
}

// assertCapturedChatTextShape checks how text-only message content reached the
// vendor: the compact string for everyone off the explicit-prompt-cache
// allowlist, the content-part array for those on it. Either way the shape is
// fixed per vendor and never depends on where a cache breakpoint sat — the
// property the cache_prefix section checks end-to-end.
func assertCapturedChatTextShape(t flagTB, env *TestEnv, wantCompact bool, label string) {
	t.Helper()
	captured := env.virtual.LastRequest(EndpointChat)
	if captured == nil {
		t.Fatalf("%s: final provider received no chat request", label)
	}
	messages, _ := captured.JSON()["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("%s: captured chat request has no messages; body=%s", label, truncate(string(captured.Body), 1200))
	}
	for i, raw := range messages {
		msg, _ := raw.(map[string]any)
		switch content := msg["content"].(type) {
		case string:
			if !wantCompact {
				t.Errorf("%s: messages[%d].content is a plain string, want content parts; body=%s",
					label, i, truncate(string(captured.Body), 1200))
			}
		case []any:
			if wantCompact {
				t.Errorf("%s: messages[%d].content is a %d-part array, want a plain string — "+
					"vendors off the prompt-cache allowlist may reject the array form; body=%s",
					label, i, len(content), truncate(string(captured.Body), 1200))
			}
		}
	}
}

// ExecuteAllVendorTransforms runs the vendor-dispatch fixture table across
// both streaming modes. Name format: vendor/<fixture>/{stream|nonstream}.
func (m *Matrix) ExecuteAllVendorTransforms() []TestResult {
	var cases []recorderCase
	for _, fx := range vendorFixtures {
		for _, streaming := range m.Streaming {
			name := fmt.Sprintf("vendor/%s/%s", fx.name, streamMode(streaming))
			cases = append(cases, recorderCase{
				name:      name,
				scenario:  "vendor",
				source:    protocol.TypeAnthropicV1,
				target:    protocol.TypeOpenAIChat,
				streaming: streaming,
				run: func(t flagTB, env *TestEnv) {
					runVendorTransformCase(t, env, fx, streaming)
				},
			})
		}
	}
	return m.runRecorderCases(cases)
}
