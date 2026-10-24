package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuotaMigrationPreservesLegacyKeys(t *testing.T) {
	for _, priorQuota := range []bool{false, true} {
		name := "without_quota"
		if priorQuota {
			name = "with_weekly_quota"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			conn, err := openTinglyDB(dir)
			require.NoError(t, err)
			extra := ""
			if priorQuota {
				extra = ", weekly_token_limit integer, weekly_tokens_used integer, token_week_start datetime"
			}
			require.NoError(t, conn.Exec("CREATE TABLE api_tokens (id integer PRIMARY KEY AUTOINCREMENT, token_id text NOT NULL, user_id text NOT NULL, team_id text, display_name text, enabled numeric, created_at datetime"+extra+")").Error)
			require.NoError(t, conn.Exec("INSERT INTO api_tokens (token_id,user_id,team_id,display_name,enabled,created_at) VALUES (?,?,?,?,?,?)", "legacy-key", "legacy-user", DefaultTeamID, "Legacy", true, time.Now()).Error)
			if priorQuota {
				require.NoError(t, conn.Exec("UPDATE api_tokens SET weekly_token_limit=?,weekly_tokens_used=?,token_week_start=?", 100, 17, tokenWeekStart(time.Now())).Error)
			}
			sqlDB, err := conn.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
			s, err := NewAPITokenStore(dir)
			require.NoError(t, err)
			defer s.Close()
			r, err := s.ValidateToken("legacy-key")
			require.NoError(t, err)
			require.Equal(t, "legacy-user", r.UserID)
			require.Equal(t, DefaultTeamID, r.TeamID)
			require.Empty(t, r.QuotaResetMode)
			if priorQuota {
				require.Equal(t, int64(17), r.WeeklyTokensUsed)
				require.Equal(t, int64(100), *r.WeeklyTokenLimit)
			} else {
				require.Nil(t, r.WeeklyTokenLimit)
				require.Zero(t, r.WeeklyTokensUsed)
			}
		})
	}
}
