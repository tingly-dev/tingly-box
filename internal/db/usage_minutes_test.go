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

func TestUsageMinutesWhereAndByUser(t *testing.T) {
	store, err := NewUsageStore(t.TempDir())
	require.NoError(t, err)
	now := time.Now().UTC()
	add := func(user, team, model string, ago time.Duration, tokens int) {
		require.NoError(t, store.RecordUsage(&UsageRecord{ProviderUUID: "p", ProviderName: "p", Model: model, Scenario: "team",
			UserID: user, TeamID: team, Timestamp: now.Add(-ago), TotalTokens: tokens, InputTokens: tokens, Status: "success"}))
	}
	add("alice", "t1", "m1", time.Minute, 10)
	add("alice", "t1", "m2", 2*time.Minute, 20)
	add("bob", "t1", "m1", time.Minute, 5)
	add("carol", "t2", "m1", time.Minute, 99)
	add("alice", "t1", "m1", 5*time.Hour, 1000) // outside

	sum := func(ms []UsageMinute) (r, tk int64) {
		for _, m := range ms {
			r += m.Requests
			tk += m.Tokens
		}
		return
	}
	since := now.Add(-time.Hour)

	alice, err := store.UsageMinutesWhere(map[string]string{"user_id": "alice"}, since)
	require.NoError(t, err)
	r, tk := sum(alice)
	require.Equal(t, int64(2), r)
	require.Equal(t, int64(30), tk)

	team, err := store.UsageMinutesWhere(map[string]string{"team_id": "t1"}, since)
	require.NoError(t, err)
	r, tk = sum(team)
	require.Equal(t, int64(3), r)
	require.Equal(t, int64(35), tk)

	model, err := store.UsageMinutesWhere(map[string]string{"model": "m1"}, since)
	require.NoError(t, err)
	r, _ = sum(model)
	require.Equal(t, int64(3), r)

	_, err = store.UsageMinutesWhere(map[string]string{"token": "x"}, since)
	require.Error(t, err, "only whitelisted columns may be filtered on")

	byUser, err := store.UsageMinutesByUser("t1", since)
	require.NoError(t, err)
	require.Len(t, byUser, 2, "carol belongs to another team")
	r, tk = sum(byUser["alice"])
	require.Equal(t, int64(2), r)
	require.Equal(t, int64(30), tk)
	r, _ = sum(byUser["bob"])
	require.Equal(t, int64(1), r)
}
