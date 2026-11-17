package db

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
)

func TestLimitPolicyStoreCRUDAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLimitPolicyStore(dir)
	require.NoError(t, err)

	_, ok := s.GetLimitPolicy(ai.LimitPolicyUser, "u1")
	require.False(t, ok)

	p, exists, err := s.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "u1",
		Limits:   ai.Limits{MaxConcurrent: 2, MaxTokens: 1000, WindowSeconds: 600, SoftPercent: 80, QuotaPercent: 90},
		Overflow: "bogus", QueueTimeoutSeconds: -5})
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 0, p.Limits.SoftPercent, "soft steering is meaningless for a person")
	require.Equal(t, 0, p.Limits.QuotaPercent)
	require.Equal(t, ai.LimitOverflowQueue, p.Overflow, "unknown overflow falls back to queue")
	require.Equal(t, 0, p.QueueTimeoutSeconds)

	got, ok := s.GetLimitPolicy(ai.LimitPolicyUser, "u1")
	require.True(t, ok)
	require.Equal(t, p, got)

	// model policies keep soft steering and carry no overflow of their own
	m, _, err := s.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyModel, Ref: "claude-opus", Overflow: "reject",
		Limits: ai.Limits{MaxConcurrent: 3, SoftPercent: 70}})
	require.NoError(t, err)
	require.Equal(t, 70, m.Limits.SoftPercent)
	require.Equal(t, "", m.Overflow)

	_, _, err = s.Upsert(ai.LimitPolicy{Kind: "nope", Ref: "x", Limits: ai.Limits{MaxConcurrent: 1}})
	require.Error(t, err)
	_, _, err = s.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyTeam, Ref: "", Limits: ai.Limits{MaxConcurrent: 1}})
	require.Error(t, err)

	require.Len(t, s.List("", ""), 2)
	require.Len(t, s.List(ai.LimitPolicyModel, ""), 1)

	// survives a reopen (written through, not just cached)
	s2, err := NewLimitPolicyStore(dir)
	require.NoError(t, err)
	got, ok = s2.GetLimitPolicy(ai.LimitPolicyUser, "u1")
	require.True(t, ok)
	require.Equal(t, 2, got.Limits.MaxConcurrent)

	// an emptied policy is deleted, from cache and table
	_, exists, err = s.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "u1", Limits: ai.Limits{}})
	require.NoError(t, err)
	require.False(t, exists)
	_, ok = s.GetLimitPolicy(ai.LimitPolicyUser, "u1")
	require.False(t, ok)
	s3, err := NewLimitPolicyStore(dir)
	require.NoError(t, err)
	_, ok = s3.GetLimitPolicy(ai.LimitPolicyUser, "u1")
	require.False(t, ok)

	require.NoError(t, s.Delete(ai.LimitPolicyModel, "never-existed"), "deleting a missing policy is not an error")
}
