package probe

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestConfig builds a minimal *config.Config backed by a temp directory.
func newTestConfig(t *testing.T) *serverconfig.Config {
	t.Helper()
	dir, err := os.MkdirTemp("", "probe-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	cfg, err := serverconfig.NewConfigWithDir(dir)
	require.NoError(t, err)
	return cfg
}

// addProvider registers a provider in cfg and returns it.
func addProvider(t *testing.T, cfg *serverconfig.Config, p *typ.Provider) {
	t.Helper()
	require.NoError(t, cfg.AddProvider(p))
}

// ---- resolveProviderTarget loopback routing ----

func TestResolveProviderTarget_OpenAI_RoutesLoopback(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ServerPort = 18080
	cfg.ModelToken = "test-token"

	p := &typ.Provider{
		UUID:     "p-openai",
		Name:     "OpenAI",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
		Models:   []string{"gpt-4"},
	}
	addProvider(t, cfg, p)

	svc := &E2EProber{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
	}

	loopback, model, headers, err := svc.resolveProviderTarget(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4", model)
	assert.Equal(t, protocol.APIStyleOpenAI, loopback.APIStyle)
	assert.Equal(t, "http://localhost:18080/tingly/openai", loopback.APIBase,
		"apiBase should point at TB loopback (no /v1 suffix), got %s", loopback.APIBase)
	require.Contains(t, headers, "X-Tingly-Probe-Service")
	assert.Equal(t, "p-openai:gpt-4", headers["X-Tingly-Probe-Service"])
}

func TestResolveProviderTarget_Anthropic_RoutesLoopback(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ServerPort = 18080
	cfg.ModelToken = "test-token"

	p := &typ.Provider{
		UUID:     "p-anthropic",
		Name:     "Anthropic",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		Enabled:  true,
		Models:   []string{"claude-3-5-sonnet-20241022"},
	}
	addProvider(t, cfg, p)

	svc := &E2EProber{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-anthropic",
		Model:        "claude-3-5-sonnet-20241022",
	}

	loopback, model, headers, err := svc.resolveProviderTarget(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "claude-3-5-sonnet-20241022", model)
	assert.Equal(t, protocol.APIStyleAnthropic, loopback.APIStyle)
	assert.Equal(t, "http://localhost:18080/tingly/anthropic", loopback.APIBase,
		"apiBase should point at TB loopback (no /v1 suffix), got %s", loopback.APIBase)
	assert.Equal(t, "p-anthropic:claude-3-5-sonnet-20241022", headers["X-Tingly-Probe-Service"])
}

func TestResolveProviderTarget_Google_DirectSDK(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ServerPort = 18080

	p := &typ.Provider{
		UUID:     "p-google",
		Name:     "Google",
		APIBase:  "https://generativelanguage.googleapis.com",
		APIStyle: protocol.APIStyleGoogle,
		Enabled:  true,
		Models:   []string{"gemini-2.0-flash"},
	}
	addProvider(t, cfg, p)

	svc := &E2EProber{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-google",
		Model:        "gemini-2.0-flash",
	}

	got, model, headers, err := svc.resolveProviderTarget(context.Background(), req)
	require.NoError(t, err)

	// Google must go direct (same provider record returned).
	assert.Equal(t, "p-google", got.UUID)
	assert.Equal(t, "gemini-2.0-flash", model)
	assert.Empty(t, headers, "Google probe must have no probe headers (direct SDK path)")
}

func TestResolveProviderTarget_NoPort_FallsBackDirect(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ServerPort = 0 // unknown port

	p := &typ.Provider{
		UUID:     "p-openai",
		Name:     "OpenAI",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
		Models:   []string{"gpt-4"},
	}
	addProvider(t, cfg, p)

	svc := &E2EProber{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
	}

	got, _, headers, err := svc.resolveProviderTarget(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "p-openai", got.UUID, "must fall back to direct provider when port unknown")
	assert.Empty(t, headers)
}

func TestResolveProviderTarget_DisabledProvider_Errors(t *testing.T) {
	cfg := newTestConfig(t)

	// AddProvider always sets Enabled=true; disable it afterwards via Update.
	p := &typ.Provider{
		UUID:     "p-disabled",
		Name:     "Disabled",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
	}
	addProvider(t, cfg, p)

	p.Enabled = false
	require.NoError(t, cfg.UpdateProvider("p-disabled", p))

	svc := &E2EProber{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-disabled",
		Model:        "gpt-4",
	}

	_, _, _, err := svc.resolveProviderTarget(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

// ---- WithProbeHeaders context round-trip ----

func TestWithProbeHeaders_ContextRoundTrip(t *testing.T) {
	headers := map[string]string{"X-Tingly-Probe-Service": "p:m"}
	ctx := client.WithProbeHeaders(context.Background(), headers)

	got, ok := client.GetProbeHeaders(ctx)
	assert.True(t, ok)
	assert.Equal(t, "p:m", got["X-Tingly-Probe-Service"])

	_, ok = client.GetProbeHeaders(context.Background())
	assert.False(t, ok)
}

// ---- resolveOpenAIProbeEndpoint ----

func TestResolveOpenAIProbeEndpoint_OverrideWins(t *testing.T) {
	chatOnly := &typ.Provider{AuthType: typ.AuthTypeAPIKey}
	codexOAuth := &typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex},
	}

	assert.Equal(t, "chat", resolveOpenAIProbeEndpoint("chat", chatOnly))
	assert.Equal(t, "chat", resolveOpenAIProbeEndpoint("chat", codexOAuth),
		"an explicit override must win even over a Codex OAuth provider")
	assert.Equal(t, "responses", resolveOpenAIProbeEndpoint("responses", chatOnly))
	assert.Equal(t, "responses", resolveOpenAIProbeEndpoint("responses", codexOAuth))
}

func TestResolveOpenAIProbeEndpoint_DefaultsWithoutOverride(t *testing.T) {
	chatOnly := &typ.Provider{AuthType: typ.AuthTypeAPIKey}
	codexOAuth := &typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex},
	}

	assert.Equal(t, "chat", resolveOpenAIProbeEndpoint("", chatOnly))
	assert.Equal(t, "responses", resolveOpenAIProbeEndpoint("", codexOAuth))
}

func TestResolveOpenAIProbeEndpoint_InvalidOverrideFallsBackToDefault(t *testing.T) {
	chatOnly := &typ.Provider{AuthType: typ.AuthTypeAPIKey}
	assert.Equal(t, "chat", resolveOpenAIProbeEndpoint("bogus", chatOnly))
}

// ---- endpointProbeCache ----

func TestEndpointProbeCache_HitAfterRemember(t *testing.T) {
	c := newEndpointProbeCache()
	assert.False(t, c.hit("p1", "gpt-4o", "responses", "simple"), "unseeded key should miss")

	c.remember("p1", "gpt-4o", "responses", "simple")
	assert.True(t, c.hit("p1", "gpt-4o", "responses", "simple"))
}

func TestEndpointProbeCache_KeyIsFullyQualified(t *testing.T) {
	c := newEndpointProbeCache()
	c.remember("p1", "gpt-4o", "responses", "simple")

	assert.False(t, c.hit("p2", "gpt-4o", "responses", "simple"), "different provider must not hit")
	assert.False(t, c.hit("p1", "gpt-4o-mini", "responses", "simple"), "different model must not hit")
	assert.False(t, c.hit("p1", "gpt-4o", "chat", "simple"), "different endpoint must not hit")
	assert.False(t, c.hit("p1", "gpt-4o", "responses", "streaming"), "different test mode must not hit")
}

func TestEndpointProbeCache_ExpiresAfterTTL(t *testing.T) {
	c := newEndpointProbeCache()
	key := endpointProbeCacheKey("p1", "gpt-4o", "responses", "simple")
	// Backdate the entry past the TTL instead of sleeping in the test.
	c.entries[key] = time.Now().Add(-endpointProbeCacheTTL - time.Second)

	assert.False(t, c.hit("p1", "gpt-4o", "responses", "simple"), "stale entry must be treated as a miss")

	c.mu.Lock()
	_, stillPresent := c.entries[key]
	c.mu.Unlock()
	assert.False(t, stillPresent, "hit() should evict the stale entry")
}

// ---- E2EService.Probe cache short-circuit ----

// TestProbe_CachedEndpointCheck_SkipsDispatch proves a cache hit on the
// direct provider+model+endpoint shape returns success without reaching the
// SDK dispatch path — clientPool is left nil, which would panic if dispatch
// were attempted, so a clean success return is itself the assertion.
func TestProbe_CachedEndpointCheck_SkipsDispatch(t *testing.T) {
	cfg := newTestConfig(t)
	p := &typ.Provider{
		UUID: "p-cache", Name: "OpenAI", APIBase: "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI, Enabled: true, Models: []string{"gpt-4o"},
	}
	addProvider(t, cfg, p)

	svc := NewE2EProber(cfg, nil) // nil clientPool: dispatch would panic
	svc.endpointCache.remember("p-cache", "gpt-4o", "responses", "false-false")

	result, err := svc.Probe(context.Background(), &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-cache",
		Model:        "gpt-4o",
		Direct:       true,
		Protocol:     ProtocolOpenAIResponses,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
}

// TestProbe_CachedEndpointCheck_DoesNotCrossTestModes proves a cache entry
// recorded for one probe shape does not short-circuit a probe requested under
// a different shape — streaming/tool checks must always dispatch for real,
// even if a non-stream/non-tool check for the same provider+model+endpoint
// recently succeeded. Regression test for the Probe/ProbeStream merge
// silently widening the cache's reach across probe shapes.
func TestProbe_CachedEndpointCheck_DoesNotCrossTestModes(t *testing.T) {
	cfg := newTestConfig(t)
	p := &typ.Provider{
		UUID: "p-cache", Name: "OpenAI", APIBase: "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI, Enabled: true, Models: []string{"gpt-4o"},
	}
	addProvider(t, cfg, p)

	svc := NewE2EProber(cfg, nil) // nil clientPool: dispatch would panic if reached
	svc.endpointCache.remember("p-cache", "gpt-4o", "responses", "false-false")

	assert.False(t, svc.endpointCache.hit("p-cache", "gpt-4o", "responses", "true-false"),
		"a non-stream cache entry must not satisfy a streaming check")
	assert.False(t, svc.endpointCache.hit("p-cache", "gpt-4o", "responses", "false-true"),
		"a non-stream cache entry must not satisfy a tool check")
}

// Through-TB probes hand the SDK a loopback provider, so the Claude Code
// preamble decision must come from the real target: the pinned provider of a
// provider target, or any service provider of a rule target.
func TestTargetIsClaudeCode(t *testing.T) {
	cfg := newTestConfig(t)
	oauth := &typ.Provider{
		UUID: "p-cc", Name: "Claude Code", APIBase: "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic, Enabled: true, AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{Issuer: ai.IssuerClaudeCode, AccessToken: "tok"},
		Models:      []string{"claude-sonnet-4-5"},
	}
	apiKey := &typ.Provider{
		UUID: "p-key", Name: "Anthropic", APIBase: "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic, Enabled: true, Token: "sk-ant", Models: []string{"claude-sonnet-4-5"},
	}
	addProvider(t, cfg, oauth)
	addProvider(t, cfg, apiKey)
	require.NoError(t, cfg.AddRule(typ.Rule{
		UUID: "r-cc", Scenario: typ.ScenarioClaudeCode, RequestModel: "probe-test/cc", Active: true,
		Services: []*loadbalance.Service{{Provider: "p-key", Model: "claude-sonnet-4-5", Active: true}, {Provider: "p-cc", Model: "claude-sonnet-4-5", Active: true}},
	}))
	require.NoError(t, cfg.AddRule(typ.Rule{
		UUID: "r-key", Scenario: typ.ScenarioAnthropic, RequestModel: "m", Active: true,
		Services: []*loadbalance.Service{{Provider: "p-key", Model: "claude-sonnet-4-5", Active: true}},
	}))
	svc := &E2EProber{config: cfg}

	assert.True(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-cc"}))
	assert.False(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-key"}))
	assert.True(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetRule, RuleUUID: "r-cc"}), "any Claude Code service on the rule")
	assert.False(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetRule, RuleUUID: "r-key"}))
	assert.False(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetRule, RuleUUID: "missing"}))
	assert.False(t, svc.targetIsClaudeCode(&E2ERequest{TargetType: E2ETargetProviderConfig}))

	// The flag reaches the Anthropic builder even when the SDK client's own
	// provider (the loopback) is not Claude Code.
	p := (&E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p-cc", Model: "claude-sonnet-4-5"}).probeParams("claude-sonnet-4-5")
	p.ClaudeCodePreamble = true
	params := buildAnthropicMessageParams(p, false)
	require.NotEmpty(t, params.System)
	assert.Equal(t, client.ClaudeCodeSystemHeader, params.System[0].Text)
	params = buildAnthropicMessageParams(p, true)
	assert.Equal(t, 1, countPreambles(params.System), "provider and target flags must not double the preamble")
}

func countPreambles(system []anthropic.TextBlockParam) int {
	n := 0
	for _, b := range system {
		if b.Text == client.ClaudeCodeSystemHeader {
			n++
		}
	}
	return n
}
