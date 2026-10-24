package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestProviderResetClock(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	r := APITokenRecord{QuotaResetMode: "provider", TokenWeekStart: now, WeeklyTokensUsed: 70}
	r = advanceQuota(r, now, &end)
	require.Equal(t, int64(70), r.WeeklyTokensUsed)
	require.Equal(t, &end, r.QuotaResetsAt)
	// A fetch failure retains the learned boundary.
	r = advanceQuota(r, now.Add(time.Minute), nil)
	require.Equal(t, &end, r.QuotaResetsAt)
	r = advanceQuota(r, end, nil)
	require.Zero(t, r.WeeklyTokensUsed)
	require.Nil(t, r.QuotaResetsAt)
	// A stale snapshot cannot repeatedly mint new budget.
	r.WeeklyTokensUsed = 12
	r = advanceQuota(r, end.Add(time.Hour), &end)
	require.Equal(t, int64(12), r.WeeklyTokensUsed)
	r = advanceQuota(r, end.Add(14*24*time.Hour), nil)
	require.Equal(t, int64(12), r.WeeklyTokensUsed)
	// A fresh future deadline does not clear current consumption.
	next := end.Add(21 * 24 * time.Hour)
	r = advanceQuota(r, end.Add(14*24*time.Hour), &next)
	require.Equal(t, int64(12), r.WeeklyTokensUsed)
	require.Equal(t, &next, r.QuotaResetsAt)
}

func TestQuotaResetPolicyPreservesUsageAndPersistence(t *testing.T) {
	s, dir := setupTestAPITokenStore(t)
	limit := int64(100)
	r, err := s.CreateTokenForTeam("user", "key-reset", DefaultTeamID, "test", "admin", nil, &limit)
	require.NoError(t, err)
	deadline := time.Now().Add(time.Hour).UTC()
	s.SetQuotaResetSource(func(id string) (string, *time.Time) { return "provider-a", &deadline })
	require.NoError(t, s.ChargeTokens(r.ID, 80))
	current, err := s.GetToken(r.TokenID)
	require.NoError(t, err)
	require.Equal(t, "provider-a", current.QuotaProviderID)
	require.True(t, current.QuotaResetsAt.Equal(deadline))
	require.NoError(t, s.Close())
	s, err = NewAPITokenStore(dir)
	require.NoError(t, err)
	defer s.Close()
	current, err = s.GetToken(r.TokenID)
	require.NoError(t, err)
	require.True(t, current.QuotaResetsAt.Equal(deadline))
	require.Equal(t, int64(80), current.WeeklyTokensUsed)
	anchor := time.Now().Add(3 * time.Hour)
	require.NoError(t, s.SetWeeklyTokenLimit(r.TokenID, &limit, &QuotaResetPolicy{Mode: "manual", Anchor: &anchor}))
	current, err = s.GetToken(r.TokenID)
	require.NoError(t, err)
	require.Equal(t, int64(80), current.WeeklyTokensUsed)
	require.True(t, current.QuotaResetsAt.Equal(anchor))
	require.Error(t, s.SetWeeklyTokenLimit(r.TokenID, &limit, &QuotaResetPolicy{Mode: "manual"}))
	require.Error(t, s.SetWeeklyTokenLimit(r.TokenID, &limit, &QuotaResetPolicy{Mode: "bogus"}))
}

func TestManualQuotaReset(t *testing.T) {
	anchor := time.Date(2026, 10, 22, 8, 30, 0, 0, time.FixedZone("CST", 8*3600))
	r := APITokenRecord{QuotaResetMode: "manual", QuotaResetAnchor: &anchor, QuotaResetsAt: &anchor, WeeklyTokensUsed: 99}
	r = advanceQuota(r, anchor.Add(15*24*time.Hour), nil)
	require.Zero(t, r.WeeklyTokensUsed)
	require.True(t, r.QuotaResetsAt.Equal(anchor.Add(21*24*time.Hour)))
	r.WeeklyTokensUsed = 5
	r = advanceQuota(r, anchor.Add(16*24*time.Hour), nil)
	require.Equal(t, int64(5), r.WeeklyTokensUsed)
	ancient := time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC)
	next := nextWeeklyReset(ancient, anchor)
	require.True(t, next.After(anchor))
	require.LessOrEqual(t, next.Sub(anchor), 7*24*time.Hour)
}

func TestProviderWeeklyResetSelection(t *testing.T) {
	end := time.Now().Add(time.Hour)
	weekly := &quota.UsageWindow{Type: quota.WindowTypeWeekly, Kind: quota.WindowKindLimit, ResetsAt: &end}
	usage := &quota.ProviderUsage{Windows: []*quota.UsageWindow{
		{Type: quota.WindowTypeSession, Kind: quota.WindowKindLimit, ResetsAt: &end}, weekly,
	}}
	require.True(t, ProviderWeeklyReset(usage).Equal(end))
	weekly.UsedPercent = 95 // Outside traffic does not affect the clock.
	require.True(t, ProviderWeeklyReset(usage).Equal(end))
	other := end.Add(time.Hour)
	usage.Windows = append(usage.Windows, &quota.UsageWindow{Type: quota.WindowTypeWeekly, Kind: quota.WindowKindLimit, ResetsAt: &other})
	require.Nil(t, ProviderWeeklyReset(usage))
	require.Nil(t, ProviderWeeklyReset(nil))
}
