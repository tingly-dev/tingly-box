package protocolserver

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ── the store ───────────────────────────────────────────────────────────────

func TestEndpointMemory_LookupRememberExpiry(t *testing.T) {
	now := time.Now()
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()
	m := newEndpointMemory()

	if got := m.Lookup(endpointMemoryKey("p1", "luna")); got != "" {
		t.Fatalf("empty store returned %q", got)
	}

	m.Remember(endpointMemoryKey("p1", "luna"), protocol.TypeOpenAIResponses)
	if got := m.Lookup(endpointMemoryKey("p1", "luna")); got != protocol.TypeOpenAIResponses {
		t.Errorf("learned value = %q, want responses", got)
	}
	// Keyed per model, not per provider.
	if got := m.Lookup(endpointMemoryKey("p1", "kimi-k3")); got != "" {
		t.Errorf("another model on the same provider returned %q", got)
	}

	now = now.Add(endpointMemoryTTL + time.Second)
	if got := m.Lookup(endpointMemoryKey("p1", "luna")); got != "" {
		t.Errorf("expired entry still returned %q", got)
	}
}

func TestEndpointMemory_FailedRetryCooldown(t *testing.T) {
	now := time.Now()
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()
	m := newEndpointMemory()

	if !m.ShouldRetry(endpointMemoryKey("p1", "luna")) {
		t.Fatal("first retry must be allowed")
	}
	m.MarkRetry(endpointMemoryKey("p1", "luna"))
	if m.ShouldRetry(endpointMemoryKey("p1", "luna")) {
		t.Error("a second retry inside the cooldown must be refused")
	}

	now = now.Add(endpointRetryCooldown + time.Second)
	if !m.ShouldRetry(endpointMemoryKey("p1", "luna")) {
		t.Error("retry must be allowed again after the cooldown")
	}

	// The cooldown gates failures only: a success clears it outright, and the
	// learned mapping then means no failure arises to retry in the first place.
	m.MarkRetry(endpointMemoryKey("p1", "luna"))
	m.Remember(endpointMemoryKey("p1", "luna"), protocol.TypeOpenAIResponses)
	if !m.ShouldRetry(endpointMemoryKey("p1", "luna")) {
		t.Error("remembering an answer must clear the cooldown")
	}
}

// ── the signature ───────────────────────────────────────────────────────────

// TestLooksLikeEndpointMismatch pins the three shapes OpenCode Zen actually
// returns for one condition, and the failures that must NOT be mistaken for it.
func TestLooksLikeEndpointMismatch(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"grok-4.6: explicit format rejection", 401,
			`{"error":{"type":"ModelError","message":"Model grok-4.6 is not supported for format oa-compat"}}`, true},
		{"grok-4.5: marker inside a 500", 500,
			`{"error":{"message":"Upstream request failed: Endpoint is unavailable."}}`, true},
		{"gpt-5.6-luna: bare 500", 500,
			`{"type":"error","error":{"message":"Internal server error"}}`, true},
		{"bad credential", 401,
			`{"error":{"type":"AuthError","message":"Missing API key."}}`, false},
		{"malformed request", 400,
			`{"error":{"message":"messages: field required"}}`, false},
		{"rate limited", 429,
			`{"error":{"message":"slow down"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeEndpointMismatch(tt.status, []byte(tt.body)); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlternateEndpoint(t *testing.T) {
	if got := alternateEndpoint(protocol.TypeOpenAIChat); got != protocol.TypeOpenAIResponses {
		t.Errorf("chat → %q", got)
	}
	if got := alternateEndpoint(protocol.TypeOpenAIResponses); got != protocol.TypeOpenAIChat {
		t.Errorf("responses → %q", got)
	}
	if got := alternateEndpoint(protocol.TypeAnthropicV1); got != "" {
		t.Errorf("non-OpenAI target → %q, want empty", got)
	}
}

// ── the loop ────────────────────────────────────────────────────────────────

// resetEndpointMemory swaps in a fresh store for one test and restores the
// process-wide one afterwards, so these tests neither inherit nor leak
// learned state.
func resetEndpointMemory(t *testing.T) *endpointMemory {
	t.Helper()
	previous := defaultEndpointMemory
	defaultEndpointMemory = newEndpointMemory()
	t.Cleanup(func() { defaultEndpointMemory = previous })
	return defaultEndpointMemory
}

// perModelFailoverHandler builds a handler with the deps failover selection
// needs (config-backed provider lookup, load balancer, health monitor), plus a
// two-tier rule: the per-model Zen service first, a sibling behind it.
func perModelFailoverHandler(t *testing.T, zen *typ.Provider, sibling *typ.Provider, zenModel, siblingModel string) (*ProtocolHandler, *typ.Rule) {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	for _, p := range []*typ.Provider{zen, sibling} {
		p.Enabled = true
		if err := cfg.AddProvider(p); err != nil {
			t.Fatalf("AddProvider(%s): %v", p.UUID, err)
		}
	}
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{Config: cfg, LoadBalancer: lb, HealthMonitor: hm})

	rule := &typ.Rule{
		UUID: "r-permodel", Scenario: typ.ScenarioOpenAI, Active: true,
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Services: []*loadbalance.Service{
			{Provider: zen.UUID, Model: zenModel, Active: true, Tier: 0},
			{Provider: sibling.UUID, Model: siblingModel, Active: true, Tier: 1},
		},
	}
	return ph, rule
}

func perModelProvider() *typ.Provider {
	return &typ.Provider{
		UUID:               "p-opencode",
		Name:               "OpenCode Go",
		APIStyle:           protocol.APIStyleOpenAI,
		APIBase:            "https://opencode.ai/zen/go/v1",
		OpenAIEndpointMode: ai.EndpointModePerModel,
	}
}

func singleServiceRule(provider *typ.Provider, model string) *typ.Rule {
	return &typ.Rule{
		UUID:     "r1",
		Scenario: typ.ScenarioOpenAI,
		Active:   true,
		Services: []*loadbalance.Service{{Provider: provider.UUID, Model: model, Active: true}},
	}
}

func testGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, rec
}

// TestDispatch_PerModel_LearnsResponsesOnlyModel is the #1595 shape: a
// single-service rule on a per-model provider whose model only answers on
// Responses. The first attempt guesses Chat and is rejected; the loop must
// retry the same service on Responses, serve that answer, and remember it so
// the next request pays no extra round-trip.
func TestDispatch_PerModel_LearnsResponsesOnlyModel(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := singleServiceRule(provider, "gpt-5.6-luna")
	c, rec := testGinContext()
	ph := &ProtocolHandler{}

	var used []protocol.APIType
	attempt := func(p *typ.Provider, model string) {
		target, err := ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		used = append(used, target)
		if target == protocol.TypeOpenAIChat {
			// what Zen answers for luna on /chat/completions
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": "resp_1"})
	}

	ph.DispatchWithPriorityFailover(c, rule, provider, "gpt-5.6-luna", attempt)

	if len(used) != 2 {
		t.Fatalf("attempts = %v, want one guess plus one learning retry", used)
	}
	if used[0] != protocol.TypeOpenAIChat || used[1] != protocol.TypeOpenAIResponses {
		t.Errorf("attempt endpoints = %v, want [chat responses]", used)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("client saw %d, want the retry's 200", rec.Code)
	}
	if got := defaultEndpointMemory.Lookup(endpointMemoryKey(provider.UUID, "gpt-5.6-luna")); got != protocol.TypeOpenAIResponses {
		t.Errorf("learned = %q, want responses", got)
	}
}

// TestDispatch_PerModel_UsesLearnedEndpointWithoutRetry proves the learning is
// worth having: once known, the right endpoint is used on the first attempt.
func TestDispatch_PerModel_UsesLearnedEndpointWithoutRetry(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	defaultEndpointMemory.Remember(endpointMemoryKey(provider.UUID, "gpt-5.6-luna"), protocol.TypeOpenAIResponses)
	rule := singleServiceRule(provider, "gpt-5.6-luna")
	c, rec := testGinContext()
	ph := &ProtocolHandler{}

	var used []protocol.APIType
	ph.DispatchWithPriorityFailover(c, rule, provider, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		target, _ := ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		used = append(used, target)
		c.JSON(http.StatusOK, gin.H{"id": "resp_1"})
	})

	if len(used) != 1 || used[0] != protocol.TypeOpenAIResponses {
		t.Errorf("attempt endpoints = %v, want a single responses attempt", used)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("client saw %d", rec.Code)
	}
}

// TestDispatch_PerModel_DoesNotRetryCredentialFailures keeps the fallback
// narrow: a 401 that is a credential problem carries no format marker and
// must not spend a round-trip on the other endpoint.
func TestDispatch_PerModel_DoesNotRetryCredentialFailures(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := singleServiceRule(provider, "kimi-k3")
	c, _ := testGinContext()
	ph := &ProtocolHandler{}

	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, provider, "kimi-k3", func(p *typ.Provider, model string) {
		attempts++
		_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "AuthError", "message": "Missing API key."}})
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no endpoint retry for a credential failure)", attempts)
	}
	if got := defaultEndpointMemory.Lookup(endpointMemoryKey(provider.UUID, "kimi-k3")); got != "" {
		t.Errorf("nothing should have been learned, got %q", got)
	}
}

// TestDispatch_OtherModes_NeverRetryEndpoints is the containment guarantee:
// a provider that has not declared per-model variance behaves exactly as
// before — one attempt, no buffering for a single-service rule, no learning.
func TestDispatch_OtherModes_NeverRetryEndpoints(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	provider.OpenAIEndpointMode = ai.EndpointModeChat
	rule := singleServiceRule(provider, "gpt-5.6-luna")
	c, _ := testGinContext()
	ph := &ProtocolHandler{}

	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, provider, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		attempts++
		_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: learning is gated on the declared mode", attempts)
	}
	if got := defaultEndpointMemory.Lookup(endpointMemoryKey(provider.UUID, "gpt-5.6-luna")); got != "" {
		t.Errorf("learned %q on a mode that must never learn", got)
	}
}

// TestDispatch_PerModel_CooldownStopsRepeatedRetries keeps a sick upstream
// from doubling its own traffic: the second request inside the cooldown gets
// one attempt, not two.
func TestDispatch_PerModel_CooldownStopsRepeatedRetries(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := singleServiceRule(provider, "sick-model")
	ph := &ProtocolHandler{}

	run := func() int {
		c, _ := testGinContext()
		attempts := 0
		ph.DispatchWithPriorityFailover(c, rule, provider, "sick-model", func(p *typ.Provider, model string) {
			attempts++
			_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
		})
		return attempts
	}

	if got := run(); got != 2 {
		t.Errorf("first request attempts = %d, want 2 (guess + learning retry)", got)
	}
	if got := run(); got != 1 {
		t.Errorf("second request attempts = %d, want 1 (cooldown)", got)
	}
}

// ── the fixes from review ───────────────────────────────────────────────────

// TestEffectiveEndpointMode_HostFallback covers the reason this is derived
// rather than declared: ProviderTemplate.OpenAIEndpointMode never reaches a
// provider created through the normal API (only the OAuth path copies it), so
// a Zen provider in a real install carries no mode at all.
func TestEffectiveEndpointMode_HostFallback(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		want     ai.OpenAIEndpointMode
	}{
		{"zen with no declaration", &typ.Provider{APIBase: "https://opencode.ai/zen/go/v1"}, ai.EndpointModePerModel},
		{"zen on the anthropic base", &typ.Provider{APIBase: "https://relay.example.com", APIBaseAnthropic: "https://opencode.ai/zen/go"}, ai.EndpointModePerModel},
		{"declaration wins over the fallback", &typ.Provider{APIBase: "https://opencode.ai/zen/go/v1", OpenAIEndpointMode: ai.EndpointModeChat}, ai.EndpointModeChat},
		{"unrelated provider", &typ.Provider{APIBase: "https://api.openai.com/v1"}, ai.EndpointModeUnknown},
		{"nil", nil, ai.EndpointModeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveEndpointMode(tt.provider); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDispatch_PerModel_FailedRetryStillFailsOver is the review's top finding:
// a terminal status from the retry (404 on the other endpoint) must not mask
// the retryable original, or the request dies on one service while a healthy
// sibling sits idle.
func TestDispatch_PerModel_FailedRetryStillFailsOver(t *testing.T) {
	resetEndpointMemory(t)
	zen := perModelProvider()
	sibling := &typ.Provider{UUID: "p-sibling", Name: "sibling", APIStyle: protocol.APIStyleOpenAI, APIBase: "https://sibling.example.invalid/v1"}
	ph, rule := perModelFailoverHandler(t, zen, sibling, "gpt-5.6-luna", "gpt-5.6-luna")
	c, rec := testGinContext()

	var seen []string
	ph.DispatchWithPriorityFailover(c, rule, zen, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		target, _ := ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		seen = append(seen, p.UUID+":"+string(target))
		switch {
		case p.UUID != zen.UUID:
			c.JSON(http.StatusOK, gin.H{"id": "from-sibling"})
		case target == protocol.TypeOpenAIChat:
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
		default:
			// Terminal on the retry: without the fix this ends the request.
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "not found"}})
		}
	})

	if len(seen) != 3 {
		t.Fatalf("attempts = %v, want guess + retry + failover to the sibling", seen)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("client saw %d, want the sibling's 200", rec.Code)
	}
}

// TestDispatch_PerModel_PinnedEndpointIsNotSecondGuessed: a rule that sets
// openai_endpoint_override made the choice; learning must not overrule it.
func TestDispatch_PerModel_PinnedEndpointIsNotSecondGuessed(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := singleServiceRule(provider, "gpt-5.6-luna")
	c, _ := testGinContext()
	ph := &ProtocolHandler{}

	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, provider, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		attempts++
		flags := typ.RuleFlags{OpenAIEndpointOverride: "chat"}
		target, _ := ResolveOpenAIEndpointForRequest(c, p, model, flags, IncomingAPIChat)
		if target != protocol.TypeOpenAIChat {
			t.Errorf("attempt used %q despite the override", target)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: a pinned endpoint is not retried elsewhere", attempts)
	}
}

// TestDispatch_PerModel_SetupFailureIsNotAnUpstreamVerdict: a transform or
// resolution failure is the gateway's own 500 and says nothing about which
// endpoint the model speaks.
func TestDispatch_PerModel_SetupFailureIsNotAnUpstreamVerdict(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := singleServiceRule(provider, "gpt-5.6-luna")
	c, _ := testGinContext()
	ph := &ProtocolHandler{}

	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, provider, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		attempts++
		_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		ph.FailAttemptSetup(c, errors.New("transform failed"))
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: our own 500 is not an endpoint verdict", attempts)
	}
}

// TestDispatch_PerModel_StaleDecisionDoesNotLeakAcrossAttempts: a later
// attempt on a provider of another API style resolves no OpenAI endpoint, and
// must not inherit the previous attempt's.
func TestDispatch_PerModel_StaleDecisionDoesNotLeakAcrossAttempts(t *testing.T) {
	resetEndpointMemory(t)
	zen := perModelProvider()
	sibling := &typ.Provider{UUID: "p-anthropic", Name: "claude", APIStyle: protocol.APIStyleAnthropic, APIBase: "https://anthropic.example.invalid"}
	ph, rule := perModelFailoverHandler(t, zen, sibling, "gpt-5.6-luna", "claude-sonnet-5")
	c, _ := testGinContext()

	var seen []string
	ph.DispatchWithPriorityFailover(c, rule, zen, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		seen = append(seen, p.UUID)
		if p.APIStyle == protocol.APIStyleAnthropic {
			// An Anthropic-style attempt never resolves an OpenAI endpoint.
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "upstream down"}})
			return
		}
		_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
	})

	// zen: guess + retry, then the sibling exactly once.
	want := []string{zen.UUID, zen.UUID, sibling.UUID}
	if len(seen) != len(want) {
		t.Fatalf("attempts = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("attempts = %v, want %v", seen, want)
		}
	}
}

// TestDispatch_PerModel_EmptyServiceListStillAttempts guards the hole the gate
// installation opened: a rule with no active services (a probe pinning one by
// header) must still dispatch, not answer an empty 200.
func TestDispatch_PerModel_EmptyServiceListStillAttempts(t *testing.T) {
	resetEndpointMemory(t)
	provider := perModelProvider()
	rule := &typ.Rule{UUID: "r1", Scenario: typ.ScenarioOpenAI, Active: true}
	c, rec := testGinContext()
	ph := &ProtocolHandler{}

	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, provider, "kimi-k3", func(p *typ.Provider, model string) {
		attempts++
		c.JSON(http.StatusOK, gin.H{"id": "ok"})
	})

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Errorf("client saw %d with %d bytes, want the attempt's answer", rec.Code, rec.Body.Len())
	}
}

// TestDispatchMayRetry_StopsBufferingOnceLearned: the response buffer and the
// pristine-request snapshot exist to make a second attempt possible. Once the
// endpoint is known there is no second attempt, and a per-model provider
// should stop paying for one on every request.
func TestDispatchMayRetry_StopsBufferingOnceLearned(t *testing.T) {
	memory := resetEndpointMemory(t)
	zen := perModelProvider()
	rule := singleServiceRule(zen, "gpt-5.6-luna")

	if !DispatchMayRetry(rule, zen, "gpt-5.6-luna") {
		t.Error("with nothing learned a retry is still possible")
	}

	memory.Remember(endpointMemoryKey(zen.UUID, "gpt-5.6-luna"), protocol.TypeOpenAIResponses)
	if DispatchMayRetry(rule, zen, "gpt-5.6-luna") {
		t.Error("once learned there is nothing to retry, so nothing to buffer")
	}
	// Another model on the same provider is still unknown.
	if !DispatchMayRetry(rule, zen, "kimi-k3") {
		t.Error("learning one model must not silence another")
	}
	// A multi-service rule always retries, learned or not.
	multi := &typ.Rule{UUID: "r-multi", Scenario: typ.ScenarioOpenAI, Active: true,
		Services: []*loadbalance.Service{
			{Provider: zen.UUID, Model: "gpt-5.6-luna", Active: true},
			{Provider: "p-other", Model: "gpt-5.6-luna", Active: true},
		}}
	if !DispatchMayRetry(multi, zen, "gpt-5.6-luna") {
		t.Error("a fallback tier is reason enough to buffer")
	}
}

// TestDispatch_PerModel_ForgetsAnEndpointThatStopsWorking closes the loop the
// previous test opens: with no buffer installed, a learned mapping that goes
// stale must not pin the model to a dead endpoint for the rest of the TTL.
func TestDispatch_PerModel_ForgetsAnEndpointThatStopsWorking(t *testing.T) {
	memory := resetEndpointMemory(t)
	zen := perModelProvider()
	serviceID := endpointMemoryKey(zen.UUID, "gpt-5.6-luna")
	memory.Remember(serviceID, protocol.TypeOpenAIResponses)
	rule := singleServiceRule(zen, "gpt-5.6-luna")
	ph := &ProtocolHandler{}

	c, _ := testGinContext()
	attempts := 0
	ph.DispatchWithPriorityFailover(c, rule, zen, "gpt-5.6-luna", func(p *typ.Provider, model string) {
		attempts++
		_, _ = ResolveOpenAIEndpointForRequest(c, p, model, typ.RuleFlags{}, IncomingAPIChat)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Internal server error"}})
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: a known endpoint is not retried, only forgotten", attempts)
	}
	if got := memory.Lookup(serviceID); got != "" {
		t.Errorf("stale mapping survived as %q; the next request would fail the same way", got)
	}
	// And the next request may learn again.
	if !DispatchMayRetry(rule, zen, "gpt-5.6-luna") {
		t.Error("after forgetting, the next request must be able to retry")
	}
}

// TestEndpointMemory_SweepsDeadEntries guards the one map against unbounded
// growth: model names come from the request.
func TestEndpointMemory_SweepsDeadEntries(t *testing.T) {
	now := time.Now()
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()
	m := newEndpointMemory()

	for i := range endpointMemorySweepAt + 1 {
		m.MarkRetry(endpointMemoryKey("p1", fmt.Sprintf("model-%d", i)))
	}
	if got := len(m.entries); got <= endpointMemorySweepAt {
		t.Fatalf("entries = %d, expected the map to have grown past the sweep threshold first", got)
	}

	// Past the cooldown every one of those entries is dead; the next write
	// drops them.
	now = now.Add(endpointRetryCooldown + time.Second)
	m.MarkRetry(endpointMemoryKey("p1", "fresh"))
	if got := len(m.entries); got != 1 {
		t.Errorf("entries after sweep = %d, want only the fresh one", got)
	}
}
