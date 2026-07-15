package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newResolveTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	tm := data.NewEmbeddedOnlyTemplateManager()
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)
	return cfg
}

func codexResolveProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Codex OAuth",
		APIBase:  protocol.CodexAPIBase,
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-codex-token",
			Issuer:      ai.IssuerCodex,
		},
	}
}

// Codex's /models endpoint is unsupported, so the resolver must fall through
// to the embedded template (never persisted) and report source=template.
func TestResolveProviderModels_Codex_TemplateFallback(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))

	got, err := cfg.ResolveProviderModels(true, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceTemplate, got.Source)
	assert.Contains(t, got.Models, "gpt-5.5")
}

// A non-empty DB cache is served verbatim (source=db) and the upstream API is
// NOT re-queried when forceRefresh is false.
func TestResolveProviderModels_CacheHit_NotRefetched(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"cached-only"}, db.ModelSourceAPI))

	got, err := cfg.ResolveProviderModels(false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceCache, got.Source)
	assert.Equal(t, []string{"cached-only"}, got.Models)
}

// forceRefresh bypasses the cache: even with a cached list present, codex
// re-resolves to the template (its /models endpoint is unsupported, so no real
// network call happens, but the cache is intentionally skipped).
func TestResolveProviderModels_ForceRefresh_BypassesCache(t *testing.T) {
	cfg := newResolveTestConfig(t)
	p := codexResolveProvider()
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"stale-cached"}, db.ModelSourceAPI))

	got, err := cfg.ResolveProviderModels(true, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceTemplate, got.Source)
	assert.NotContains(t, got.Models, "stale-cached")
	assert.Contains(t, got.Models, "gpt-5.5")
}

// A provider that does not exist yields an error (used by the refresh path to
// surface a 500) rather than silently resolving to an empty list.
func TestResolveProviderModels_UnknownProvider_Errors(t *testing.T) {
	cfg := newResolveTestConfig(t)

	_, err := cfg.ResolveProviderModels(true, "does-not-exist")
	require.Error(t, err)
}
