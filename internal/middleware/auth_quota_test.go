package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
)

func TestSharingKeyQuotaRejectsNextRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := db.NewAPITokenStore(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	limit := int64(10)
	_, err = s.CreateTokenForTeam("user", "tb-share-quota", db.DefaultTeamID, "test", "admin", nil, &limit)
	require.NoError(t, err)
	cfg := &config.Config{ModelToken: "global"}
	reset := time.Now().Add(time.Hour)
	s.SetQuotaResetSource(func(id string) (string, *time.Time) { return "provider", &reset })
	cfg.MultiTenantConfig.Enabled = true
	am := NewAuthMiddleware(cfg, nil, nil, s)
	router := gin.New()
	calls := 0
	router.POST("/tingly/:scenario/v1/messages", am.ModelAuthMiddleware(), func(c *gin.Context) {
		calls++
		if charge, ok := c.Get(constant.CtxKeyChargeTokens); ok {
			require.NoError(t, charge.(func(int64) error)(12))
		}
		c.Status(http.StatusOK)
	})
	path := "/tingly/team/v1/messages"
	require.Equal(t, 200, performModelAuthRequest(router, path, "tb-share-quota").Code)
	blocked := performModelAuthRequest(router, path, "tb-share-quota")
	require.Equal(t, 429, blocked.Code)
	require.Contains(t, blocked.Body.String(), "token_quota_exceeded")
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	require.Equal(t, 1, calls)
	require.Equal(t, 200, performModelAuthRequest(router, path, "global").Code)
	require.Equal(t, 401, performModelAuthRequest(router, path, "tb-share-invalid").Code)
	s.SetQuotaResetSource(nil)
	require.NoError(t, s.SetWeeklyTokenLimit("tb-share-quota", &limit, &db.QuotaResetPolicy{Mode: "provider", ProviderID: "unknown"}))
	unknown := performModelAuthRequest(router, path, "tb-share-quota")
	require.Equal(t, 429, unknown.Code)
	require.Empty(t, unknown.Header().Get("Retry-After"))
	require.Contains(t, unknown.Body.String(), `"resets_at":null`)
}
