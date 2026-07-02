package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestResolveOpenAIEndpoint(t *testing.T) {
	chatOnly := &typ.Provider{UUID: "p-chat"} // default mode = chat
	responsesOnly := &typ.Provider{UUID: "p-resp", OpenAIEndpointMode: ai.EndpointModeResponses}
	both := &typ.Provider{UUID: "p-both", OpenAIEndpointMode: ai.EndpointModeBoth}

	tests := []struct {
		name     string
		provider *typ.Provider
		flags    typ.RuleFlags
		incoming IncomingAPIType
		want     protocol.APIType
	}{
		// Default mode (chat) — provider ignores client's incoming API
		{
			name:     "default mode forces chat (chat incoming)",
			provider: chatOnly,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "default mode forces chat (responses incoming, downgrade)",
			provider: chatOnly,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},

		// Responses-only (Codex)
		{
			name:     "responses mode forces responses (chat incoming)",
			provider: responsesOnly,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "responses mode forces responses (responses incoming)",
			provider: responsesOnly,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Both (OpenAI proper) — mirror
		{
			name:     "both mode mirrors chat",
			provider: both,
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "both mode mirrors responses",
			provider: both,
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},

		// Rule overrides (override takes priority over provider mode)
		{
			name:     "override=responses on default chat-mode provider forces responses",
			provider: chatOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "responses"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},
		{
			name:     "override=chat on responses-only provider forces chat",
			provider: responsesOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "override=chat on both-mode provider forces chat",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "chat"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "override=responses on both-mode provider forces responses",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "responses"},
			incoming: IncomingAPIChat,
			want:     protocol.TypeOpenAIResponses,
		},

		// Auto / unknown override values
		{
			name:     "auto flag treated as no override",
			provider: chatOnly,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "auto"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIChat,
		},
		{
			name:     "unknown flag value treated as no override",
			provider: both,
			flags:    typ.RuleFlags{OpenAIEndpointOverride: "bogus"},
			incoming: IncomingAPIResponses,
			want:     protocol.TypeOpenAIResponses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOpenAIEndpoint(tt.provider, tt.flags, tt.incoming)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveOpenAIEndpointNilProviderErrors(t *testing.T) {
	if _, err := ResolveOpenAIEndpoint(nil, typ.RuleFlags{}, IncomingAPIChat); err == nil {
		t.Error("expected error for nil provider")
	}
}

// TestResolveOpenAIEndpointCodexOAuthSnapshot documents the design assumption
// that Codex providers carry OpenAIEndpointMode=responses by the time they
// reach routing. The OAuth handler is responsible for setting this on
// instantiation; this test pins the resolver behavior against such providers.
func TestResolveOpenAIEndpointCodexOAuthSnapshot(t *testing.T) {
	codex := &typ.Provider{
		UUID:               "codex-1",
		OAuthDetail:        &typ.OAuthDetail{Issuer: ai.IssuerCodex},
		OpenAIEndpointMode: ai.EndpointModeResponses,
	}
	got, err := ResolveOpenAIEndpoint(codex, typ.RuleFlags{}, IncomingAPIChat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != protocol.TypeOpenAIResponses {
		t.Errorf("Codex with EndpointModeResponses should route to Responses, got %v", got)
	}
}

func TestParseEndpointOverride(t *testing.T) {
	cases := []struct {
		in   string
		want EndpointOverride
	}{
		{"", OverrideAuto},
		{"auto", OverrideAuto},
		{"chat", OverrideChat},
		{"responses", OverrideResponses},
		{"unknown", OverrideAuto},
		{"CHAT", OverrideAuto}, // case-sensitive on purpose; the registry emits lowercase
	}
	for _, c := range cases {
		got := ParseEndpointOverride(c.in)
		if got != c.want {
			t.Errorf("ParseEndpointOverride(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsCodexProvider(t *testing.T) {
	if (&typ.Provider{UUID: "p-1"}).IsCodexProvider() {
		t.Error("provider without OAuthDetail should not be Codex")
	}
	codex := &typ.Provider{
		UUID:        "codex-1",
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex},
	}
	if !codex.IsCodexProvider() {
		t.Error("Codex-issuer OAuth provider should be Codex")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Endpoint cache
// ─────────────────────────────────────────────────────────────────────────

func TestEndpointCache_GetSet(t *testing.T) {
	c := NewEndpointCache(time.Hour)

	// Miss
	if _, ok := c.Get("p1", "m1"); ok {
		t.Fatal("expected miss on empty cache")
	}

	// Set and hit
	c.Set("p1", "m1", protocol.TypeOpenAIChat)
	got, ok := c.Get("p1", "m1")
	if !ok || got != protocol.TypeOpenAIChat {
		t.Fatalf("expected chat, got %v ok=%v", got, ok)
	}

	// Different model is independent
	if _, ok := c.Get("p1", "m2"); ok {
		t.Fatal("expected miss for different model")
	}
}

func TestEndpointCache_TTLExpiry(t *testing.T) {
	c := NewEndpointCache(10 * time.Millisecond)

	c.Set("p1", "m1", protocol.TypeOpenAIResponses)
	time.Sleep(20 * time.Millisecond)

	if _, ok := c.Get("p1", "m1"); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestEndpointCache_Overwrite(t *testing.T) {
	c := NewEndpointCache(time.Hour)

	c.Set("p1", "m1", protocol.TypeOpenAIChat)
	c.Set("p1", "m1", protocol.TypeOpenAIResponses)

	got, ok := c.Get("p1", "m1")
	if !ok || got != protocol.TypeOpenAIResponses {
		t.Fatalf("expected responses after overwrite, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Runtime auto-detection
// ─────────────────────────────────────────────────────────────────────────

func TestAlternateOpenAIProtocol(t *testing.T) {
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIChat); got != protocol.TypeOpenAIResponses {
		t.Errorf("alternate of chat = %v, want responses", got)
	}
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIResponses); got != protocol.TypeOpenAIChat {
		t.Errorf("alternate of responses = %v, want chat", got)
	}
}

func TestIncomingToTarget(t *testing.T) {
	if got := incomingToTarget(IncomingAPIChat); got != protocol.TypeOpenAIChat {
		t.Errorf("incoming chat → %v, want chat", got)
	}
	if got := incomingToTarget(IncomingAPIResponses); got != protocol.TypeOpenAIResponses {
		t.Errorf("incoming responses → %v, want responses", got)
	}
}

func TestScenarioPreferredProtocol(t *testing.T) {
	tests := []struct {
		name     string
		scenario typ.RuleScenario
		incoming IncomingAPIType
		want     protocol.APIType
	}{
		{"codex prefers responses even on chat ingress", typ.ScenarioCodex, IncomingAPIChat, protocol.TypeOpenAIResponses},
		{"codex prefers responses on responses ingress", typ.ScenarioCodex, IncomingAPIResponses, protocol.TypeOpenAIResponses},
		{"codex profile suffix normalized", typ.RuleScenario("codex:p1"), IncomingAPIChat, protocol.TypeOpenAIResponses},
		{"openai mirrors chat ingress", typ.ScenarioOpenAI, IncomingAPIChat, protocol.TypeOpenAIChat},
		{"openai mirrors responses ingress", typ.ScenarioOpenAI, IncomingAPIResponses, protocol.TypeOpenAIResponses},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scenarioPreferredProtocol(tt.scenario, tt.incoming); got != tt.want {
				t.Errorf("scenarioPreferredProtocol(%q, %q) = %v, want %v", tt.scenario, tt.incoming, got, tt.want)
			}
		})
	}
}

// TestDispatchWithAutoFallback_CacheAttributedToServingProvider covers the
// multi-service failover interaction: when the initial provider fails and a
// fallback provider serves the request, the protocol cache entry must be
// written for the serving provider, not the initial one — otherwise the
// initial provider gets pinned to a protocol it never confirmed.
func TestDispatchWithAutoFallback_CacheAttributedToServingProvider(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}

	initial := &typ.Provider{UUID: "prov-initial"}
	serving := &typ.Provider{UUID: "prov-serving"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	// Simulate a dispatch where failover moved past the initial provider:
	// the gate commits (success) and the serving identity differs from initial.
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		gate.WriteHeader(200)
		gate.CommitFirstChunk()
		return serving, "served-model"
	}

	s.dispatchWithAutoFallback(c, initial, "req-model", protocol.TypeOpenAIChat, dispatch)

	if _, ok := s.endpointCache.Get(initial.UUID, "req-model"); ok {
		t.Error("cache must not contain an entry for the initial provider")
	}
	got, ok := s.endpointCache.Get(serving.UUID, "served-model")
	if !ok {
		t.Fatal("cache must contain an entry for the serving provider")
	}
	if got != protocol.TypeOpenAIChat {
		t.Errorf("cached protocol = %v, want chat", got)
	}
}

// TestDispatchWithAutoFallback_NoCacheOnTransformFailure ensures a failed
// transform (served=nil) never writes a cache entry even if the gate state
// looks successful.
func TestDispatchWithAutoFallback_NoCacheOnTransformFailure(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	initial := &typ.Provider{UUID: "prov-initial"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		gate.WriteHeader(200) // transform error path writes a JSON error; simulate benign status
		return nil, ""
	}

	s.dispatchWithAutoFallback(c, initial, "m", protocol.TypeOpenAIChat, dispatch)

	if _, ok := s.endpointCache.Get(initial.UUID, "m"); ok {
		t.Error("cache must stay empty when the transform failed")
	}
}

// TestDispatchWithAutoFallback_FirstAttemptSucceeds verifies the happy path:
// preferred protocol works on the first try and gets cached.
func TestDispatchWithAutoFallback_FirstAttemptSucceeds(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	calls := 0
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		calls++
		gate.WriteHeader(200)
		gate.CommitFirstChunk()
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIResponses, dispatch)

	if calls != 1 {
		t.Errorf("dispatch called %d times, want 1", calls)
	}
	got, ok := s.endpointCache.Get(provider.UUID, "model-a")
	if !ok || got != protocol.TypeOpenAIResponses {
		t.Errorf("cache = (%v, %v), want (responses, true)", got, ok)
	}
}

// TestDispatchWithAutoFallback_FallbackSucceeds verifies: preferred fails
// with retryable status → alternate tried → alternate succeeds → alternate
// protocol cached.
func TestDispatchWithAutoFallback_FallbackSucceeds(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	var targets []protocol.APIType
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		targets = append(targets, target)
		if target == protocol.TypeOpenAIResponses {
			// In practice, upstream 404 is converted to 500 by SendStreamingError
			gate.WriteHeader(http.StatusInternalServerError)
			gate.Write([]byte(`{"error":"endpoint not found"}`))
			c.Error(fmt.Errorf("status 500: endpoint not found"))
			return provider, "model-a"
		}
		gate.WriteHeader(200)
		gate.CommitFirstChunk()
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIResponses, dispatch)

	if len(targets) != 2 {
		t.Fatalf("dispatch called %d times, want 2", len(targets))
	}
	if targets[0] != protocol.TypeOpenAIResponses {
		t.Errorf("first attempt target = %v, want responses", targets[0])
	}
	if targets[1] != protocol.TypeOpenAIChat {
		t.Errorf("fallback target = %v, want chat", targets[1])
	}
	got, ok := s.endpointCache.Get(provider.UUID, "model-a")
	if !ok || got != protocol.TypeOpenAIChat {
		t.Errorf("cache = (%v, %v), want (chat, true)", got, ok)
	}
}

// TestDispatchWithAutoFallback_BothFail verifies: both attempts fail →
// no cache entry written.
func TestDispatchWithAutoFallback_BothFail(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	calls := 0
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		calls++
		gate.WriteHeader(http.StatusBadGateway)
		gate.Write([]byte(`{"error":"upstream failed"}`))
		c.Error(fmt.Errorf("status 502: bad gateway"))
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)

	if calls != 2 {
		t.Errorf("dispatch called %d times, want 2 (initial + fallback)", calls)
	}
	if _, ok := s.endpointCache.Get(provider.UUID, "model-a"); ok {
		t.Error("cache must stay empty when both attempts fail")
	}
}

// TestDispatchWithAutoFallback_NonRetryableSkipsFallback verifies: when
// the first attempt fails with a non-retryable error (e.g. 401), no
// fallback is attempted.
func TestDispatchWithAutoFallback_NonRetryableSkipsFallback(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	calls := 0
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		calls++
		gate.WriteHeader(http.StatusUnauthorized)
		gate.Write([]byte(`{"error":"unauthorized"}`))
		c.Error(fmt.Errorf("status 401: unauthorized"))
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)

	if calls != 1 {
		t.Errorf("dispatch called %d times, want 1 (no fallback for 401)", calls)
	}
	if _, ok := s.endpointCache.Get(provider.UUID, "model-a"); ok {
		t.Error("cache must stay empty on non-retryable error")
	}
}

// TestDispatchWithAutoFallback_Status0NoRetry verifies: when the writer
// is never touched (status 0), no fallback is attempted — the handler
// ran to completion without producing output.
func TestDispatchWithAutoFallback_Status0NoRetry(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	calls := 0
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		calls++
		// Intentionally do nothing — simulate a handler that returns without writing
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)

	if calls != 1 {
		t.Errorf("dispatch called %d times, want 1 (status 0 is non-retryable)", calls)
	}
}

// TestDispatchWithAutoFallback_GinErrorsClearedBetweenAttempts verifies
// that gin context errors from the first attempt are cleared before the
// fallback, so the fallback starts with a clean error slate.
func TestDispatchWithAutoFallback_GinErrorsClearedBetweenAttempts(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		if target == protocol.TypeOpenAIChat {
			gate.WriteHeader(http.StatusInternalServerError)
			gate.Write([]byte(`{"error":"endpoint not found"}`))
			c.Error(fmt.Errorf("status 500: endpoint not found"))
			return provider, "model-a"
		}
		// Fallback: verify errors were cleared
		if len(c.Errors) != 0 {
			t.Errorf("gin errors not cleared before fallback: %v", c.Errors)
		}
		gate.WriteHeader(200)
		gate.CommitFirstChunk()
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)
}

// TestDispatchWithAutoFallback_NonRetryableErrorViaGinContext verifies
// that non-retryable classification uses the gin context error (not just
// status code). A 500 status with a "rate limit" error message should
// NOT trigger fallback.
func TestDispatchWithAutoFallback_NonRetryableErrorViaGinContext(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	calls := 0
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		calls++
		gate.WriteHeader(http.StatusInternalServerError)
		gate.Write([]byte(`{"error":"rate limit exceeded"}`))
		c.Error(fmt.Errorf("rate limit exceeded"))
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)

	if calls != 1 {
		t.Errorf("dispatch called %d times, want 1 (rate limit is non-retryable even with 500 status)", calls)
	}
}

// TestDispatchWithAutoFallback_BufferedSuccessNonStreaming verifies the
// non-streaming success path: gate is NOT committed (no CommitFirstChunk)
// but has a 200 status with body → gateSucceeded returns true → cache
// written.
func TestDispatchWithAutoFallback_BufferedSuccessNonStreaming(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	provider := &typ.Provider{UUID: "prov-1"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
		gate.WriteHeader(200)
		gate.Write([]byte(`{"id":"resp-1"}`))
		// No CommitFirstChunk — simulates non-streaming response
		return provider, "model-a"
	}

	s.dispatchWithAutoFallback(c, provider, "model-a", protocol.TypeOpenAIChat, dispatch)

	got, ok := s.endpointCache.Get(provider.UUID, "model-a")
	if !ok || got != protocol.TypeOpenAIChat {
		t.Errorf("cache = (%v, %v), want (chat, true) — buffered 200 should count as success", got, ok)
	}
}
