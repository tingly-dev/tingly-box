package probe

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
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

	svc := &E2EService{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
		TestMode:     E2EModeSimple,
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

	svc := &E2EService{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-anthropic",
		Model:        "claude-3-5-sonnet-20241022",
		TestMode:     E2EModeSimple,
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

	svc := &E2EService{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-google",
		Model:        "gemini-2.0-flash",
		TestMode:     E2EModeSimple,
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

	svc := &E2EService{config: cfg}
	req := &E2ERequest{
		TargetType:   E2ETargetProvider,
		ProviderUUID: "p-openai",
		Model:        "gpt-4",
		TestMode:     E2EModeSimple,
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

	svc := &E2EService{config: cfg}
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

// ---- endpoint override context plumbing ----

func TestWithEndpointOverride_RoundTrip(t *testing.T) {
	ctx := withEndpointOverride(context.Background(), "responses")
	assert.Equal(t, "responses", endpointOverrideFromContext(ctx))

	ctx = withEndpointOverride(context.Background(), "chat")
	assert.Equal(t, "chat", endpointOverrideFromContext(ctx))
}

func TestWithEndpointOverride_InvalidValueIsNoop(t *testing.T) {
	ctx := withEndpointOverride(context.Background(), "bogus")
	assert.Equal(t, "", endpointOverrideFromContext(ctx))

	ctx = withEndpointOverride(context.Background(), "")
	assert.Equal(t, "", endpointOverrideFromContext(ctx))
}

func TestEndpointOverrideFromContext_UnsetReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", endpointOverrideFromContext(context.Background()))
}
