package db

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenQuotaIsolationRotationAndPersistence(t *testing.T) {
	s, dir := setupTestAPITokenStore(t)
	limit := int64(100)
	a, err := s.CreateTokenForTeam("a", "key-a", DefaultTeamID, "A", "admin", nil, &limit)
	require.NoError(t, err)
	_, err = s.CreateTokenWithTokenID("b", "key-b", "B", "admin", nil)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.ChargeTokens(a.ID, 5); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, err = s.ValidateToken("key-a")
	var exhausted *TokenQuotaExceeded
	require.ErrorAs(t, err, &exhausted)
	require.Equal(t, int64(100), exhausted.Used)
	_, err = s.ValidateToken("key-b")
	require.NoError(t, err)
	require.NoError(t, s.UpdateTokenString("key-a", "rotated-a"))
	_, err = s.ValidateToken("key-a")
	require.Error(t, err)
	rotated, err := s.ValidateToken("rotated-a")
	require.NoError(t, err)
	require.NotEqual(t, a.ID, rotated.ID)
	require.Zero(t, rotated.WeeklyTokensUsed)
	require.Equal(t, limit, *rotated.WeeklyTokenLimit)
	// A request admitted with the old key cannot consume the replacement's quota.
	require.NoError(t, s.ChargeTokens(a.ID, 10))
	require.NoError(t, s.ChargeTokens(rotated.ID, 7))
	require.NoError(t, s.Close())
	s, err = NewAPITokenStore(dir)
	require.NoError(t, err)
	defer s.Close()
	persisted, err := s.ValidateToken("rotated-a")
	require.NoError(t, err)
	require.Equal(t, int64(7), persisted.WeeklyTokensUsed)
	limit = 200
	require.NoError(t, s.SetWeeklyTokenLimit("rotated-a", &limit))
	r, err := s.ValidateToken("rotated-a")
	require.NoError(t, err)
	require.Equal(t, int64(7), r.WeeklyTokensUsed)
	require.NoError(t, s.SetWeeklyTokenLimit("rotated-a", nil))
	r, err = s.ValidateToken("rotated-a")
	require.NoError(t, err)
	require.Nil(t, r.WeeklyTokenLimit)
}

func TestTokenQuotaWeekBoundary(t *testing.T) {
	sunday := time.Date(2026, 10, 25, 23, 59, 59, 0, time.UTC)
	monday := sunday.Add(time.Second)
	r := APITokenRecord{TokenWeekStart: tokenWeekStart(sunday), WeeklyTokensUsed: 100}
	require.Equal(t, int64(100), r.CurrentTokenWeek(sunday).WeeklyTokensUsed)
	require.Zero(t, r.CurrentTokenWeek(monday).WeeklyTokensUsed)
	require.Equal(t, monday, tokenWeekStart(monday))
	require.Equal(t, monday, tokenWeekStart(monday.In(time.FixedZone("CST", 8*3600))))
}

func TestTokenRegenerationFailureAndDisabledState(t *testing.T) {
	s, _ := setupTestAPITokenStore(t)
	defer s.Close()
	key, err := s.CreateTokenWithTokenID("a", "old", "A", "admin", nil)
	require.NoError(t, err)
	_, err = s.CreateTokenWithTokenID("b", "existing", "B", "admin", nil)
	require.NoError(t, err)
	require.NoError(t, s.ChargeTokens(key.ID, 12))
	require.Error(t, s.UpdateTokenString("old", "existing"))
	require.Error(t, s.UpdateTokenString("old", "old"))
	unchanged, err := s.ValidateToken("old")
	require.NoError(t, err)
	require.Equal(t, int64(12), unchanged.WeeklyTokensUsed)
	require.NoError(t, s.SetTokenEnabled("old", false))
	require.NoError(t, s.UpdateTokenString("old", "new"))
	replacement, err := s.GetToken("new")
	require.NoError(t, err)
	require.False(t, replacement.Enabled)
	require.Zero(t, replacement.WeeklyTokensUsed)
}

func TestTokenQuotaValidationAndReset(t *testing.T) {
	s, _ := setupTestAPITokenStore(t)
	defer s.Close()
	zero := int64(0)
	r, err := s.CreateTokenForTeam("a", "key", DefaultTeamID, "A", "admin", nil, &zero)
	require.NoError(t, err)
	_, err = s.ValidateToken("key")
	var exhausted *TokenQuotaExceeded
	require.ErrorAs(t, err, &exhausted)
	negative := int64(-1)
	require.Error(t, s.SetWeeklyTokenLimit("key", &negative))
	limit := int64(20)
	require.NoError(t, s.SetWeeklyTokenLimit("key", &limit))
	s.mu.Lock()
	s.cache["key"].TokenWeekStart = tokenWeekStart(time.Now()).AddDate(0, 0, -7)
	s.cache["key"].QuotaResetMode = "legacy"
	s.cache["key"].WeeklyTokensUsed = 200
	s.mu.Unlock()
	_, err = s.ValidateToken("key")
	require.NoError(t, err)
	require.NoError(t, s.ChargeTokens(r.ID, 3))
	current, err := s.ValidateToken("key")
	require.NoError(t, err)
	require.Equal(t, int64(3), current.WeeklyTokensUsed)
}
