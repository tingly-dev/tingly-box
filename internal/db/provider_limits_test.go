package db

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProviderLimitsRoundTrip(t *testing.T) {
	store, tmpDir := setupTestProviderStore(t)
	p := &typ.Provider{
		UUID: "lim-1", Name: "lim", APIBase: "https://x.example/v1", APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey, Token: "k", Enabled: true,
		Limits: &ai.Limits{MaxConcurrent: 4, MaxTokens: 1_000_000, WindowSeconds: 18000, SoftPercent: 80, QuotaPercent: 95},
	}
	require.NoError(t, store.Save(p))

	got, err := store.GetByUUID("lim-1")
	require.NoError(t, err)
	require.Equal(t, p.Limits, got.Limits)

	// the returned provider is a private copy: mutating it must not touch the cache
	got.Limits.MaxConcurrent = 99
	again, _ := store.GetByUUID("lim-1")
	require.Equal(t, 4, again.Limits.MaxConcurrent)

	// clearing limits persists as "no limits"; an all-zero struct is stored as NULL
	p.Limits = &ai.Limits{}
	require.NoError(t, store.Save(p))
	got, _ = store.GetByUUID("lim-1")
	require.Nil(t, got.Limits)

	// survives a reopen (column persisted, not just cached)
	p.Limits = &ai.Limits{MaxRequests: 10, WindowSeconds: 60}
	require.NoError(t, store.Save(p))
	require.NoError(t, store.Close())
	reopened, err := NewProviderStore(tmpDir)
	require.NoError(t, err)
	defer reopened.Close()
	got, err = reopened.GetByUUID("lim-1")
	require.NoError(t, err)
	require.Equal(t, &ai.Limits{MaxRequests: 10, WindowSeconds: 60}, got.Limits)
}
