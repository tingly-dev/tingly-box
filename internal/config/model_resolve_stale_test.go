package config

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// When the cached list has expired and the upstream refetch fails (e.g. an
// upstream tingly-box restarting), the read path serves the last real list
// instead of an empty one; a manual refresh still reports the failure.
func TestResolveProviderModels_ExpiredCache_FetchFails_ServesStale(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg, err := NewConfig(WithConfigDir(dir))
	require.NoError(t, err)

	p := &typ.Provider{
		Name:     "upstream-tb",
		APIBase:  upstream.URL + "/tingly/openai/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "tb-token",
		Enabled:  true,
	}
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"tb-model"}, db.ModelSourceAPI))

	// Age the record past the TTL.
	conn, err := db.OpenSQLite(constant.GetDBFile(dir), 5000)
	require.NoError(t, err)
	require.NoError(t, conn.Model(&db.ProviderModelRecord{}).
		Where("provider_uuid = ?", p.UUID).
		Update("last_updated", time.Now().Add(-2*time.Hour)).Error)
	require.Empty(t, cfg.GetModelManager().GetModels(p.UUID), "record should be expired")

	got, err := cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceCache, got.Source)
	assert.Equal(t, []string{"tb-model"}, got.Models)

	refreshed, err := cfg.ResolveProviderModels(true, false, p.UUID)
	require.NoError(t, err)
	assert.Empty(t, refreshed.Models, "manual refresh must not mask the failure with the stale list")
}

// A manual refresh invalidates the cache even when the upstream call fails:
// the next normal read re-queries upstream rather than serving the
// pre-refresh list until its TTL runs out, and still falls back to that list
// while upstream stays down. A later success clears the invalidation.
func TestResolveProviderModels_ManualRefreshInvalidatesCache(t *testing.T) {
	up := false
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if !up {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"tb-new","object":"model"}]}`))
	}))
	defer upstream.Close()

	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	p := &typ.Provider{
		Name:     "upstream-tb",
		APIBase:  upstream.URL + "/tingly/openai/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "tb-token",
		Enabled:  true,
	}
	require.NoError(t, cfg.AddProvider(p))
	require.NoError(t, cfg.GetModelManager().SaveModels(p, []string{"tb-old"}, db.ModelSourceAPI))

	// Fresh cache: a normal read does not touch upstream.
	got, err := cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"tb-old"}, got.Models)
	assert.Equal(t, 0, hits)

	// Manual refresh fails, but the cache is now invalidated.
	_, err = cfg.ResolveProviderModels(true, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, 1, hits)

	got, err = cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, 2, hits, "invalidated cache must re-query upstream")
	assert.Equal(t, []string{"tb-old"}, got.Models, "stale list still served while upstream is down")

	// Upstream recovers: the next read picks up the change and re-caches it.
	up = true
	got, err = cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceAPI, got.Source)
	assert.Equal(t, []string{"tb-new"}, got.Models)
	assert.False(t, got.LastUpdatedAt.IsZero())

	got, err = cfg.ResolveProviderModels(false, false, p.UUID)
	require.NoError(t, err)
	assert.Equal(t, ModelListSourceCache, got.Source)
	assert.Equal(t, 3, hits, "successful save clears the invalidation")
}
