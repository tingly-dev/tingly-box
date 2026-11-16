package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageMinutesSince(t *testing.T) {
	store, err := NewUsageStore(t.TempDir())
	require.NoError(t, err)
	now := time.Now().UTC()
	rec := func(provider, model, rule string, ago time.Duration, tokens int) *UsageRecord {
		return &UsageRecord{ProviderUUID: provider, ProviderName: provider, Model: model, Scenario: "team", RuleUUID: rule,
			Timestamp: now.Add(-ago), InputTokens: tokens, OutputTokens: 0, TotalTokens: tokens, Status: "success"}
	}
	for _, r := range []*UsageRecord{
		rec("p1", "m", "r1", 2*time.Minute, 100),
		rec("p1", "m", "r1", 2*time.Minute, 50),
		rec("p1", "m", "r2", 3*time.Minute, 7),
		rec("p1", "other", "r1", 4*time.Minute, 5),
		rec("p2", "m", "r1", 2*time.Minute, 999),
		rec("p1", "m", "r1", 3*time.Hour, 1000), // outside the window
	} {
		require.NoError(t, store.RecordUsage(r))
	}

	sum := func(ms []UsageMinute) (reqs, toks int64) {
		for _, m := range ms {
			reqs += m.Requests
			toks += m.Tokens
		}
		return
	}

	acct, err := store.UsageMinutesSince("p1", "", "", now.Add(-time.Hour))
	require.NoError(t, err)
	reqs, toks := sum(acct)
	require.Equal(t, int64(4), reqs)
	require.Equal(t, int64(162), toks)

	share, err := store.UsageMinutesSince("p1", "m", "r1", now.Add(-time.Hour))
	require.NoError(t, err)
	reqs, toks = sum(share)
	require.Equal(t, int64(2), reqs)
	require.Equal(t, int64(150), toks)
	for _, m := range share {
		require.WithinDuration(t, now, m.At, 5*time.Minute)
	}
}
