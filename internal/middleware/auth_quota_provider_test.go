package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
)

// Uses the real persisted provider snapshot store, token store and HTTP auth
// middleware; only the model response's reported usage is simulated.
func TestProviderSnapshotQuotaHTTPAndRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	s, err := db.NewAPITokenStore(dir)
	require.NoError(t, err)
	limit := int64(10)
	_, err = s.CreateTokenForTeam("user", "tb-share-clock", db.DefaultTeamID, "clock", "test", nil, &limit)
	require.NoError(t, err)
	q, err := quota.NewGormStoreOverDB(s.GetDB(), logrus.New())
	require.NoError(t, err)
	deadline := time.Now().Add(600 * time.Millisecond).UTC()
	snapshot := &quota.ProviderUsage{ProviderUUID: "local-clock", ProviderType: quota.ProviderTypeCodex, FetchedAt: time.Now(), Windows: []*quota.UsageWindow{{Type: quota.WindowTypeWeekly, Kind: quota.WindowKindLimit, Unit: quota.UsageUnitPercent, ResetsAt: &deadline, WindowMinutes: 10080, UsedPercent: 10}}}
	require.NoError(t, q.Save(context.Background(), snapshot))
	unavailable := false
	bind := func() {
		s.SetQuotaResetSource(func(id string) (string, *time.Time) {
			if unavailable {
				return "local-clock", nil
			}
			u, err := q.Get(context.Background(), "local-clock")
			require.NoError(t, err)
			return "local-clock", db.ProviderWeeklyReset(u)
		})
	}
	bind()
	cfg := &config.Config{ModelToken: "global"}
	cfg.MultiTenantConfig.Enabled = true
	router := func() *gin.Engine {
		r := gin.New()
		r.POST("/tingly/:scenario/v1/messages", NewAuthMiddleware(cfg, nil, nil, s).ModelAuthMiddleware(), func(c *gin.Context) {
			value, _ := c.Get(constant.CtxKeyChargeTokens)
			require.NoError(t, value.(func(int64) error)(20))
			c.Status(200)
		})
		return r
	}
	r := router()
	call := func() int { return performModelAuthRequest(r, "/tingly/team/v1/messages", "tb-share-clock").Code }
	require.Equal(t, 200, call())
	require.Equal(t, 429, call())
	snapshot.Windows[0].UsedPercent = 99 // External consumption must not change local usage.
	require.NoError(t, q.Save(context.Background(), snapshot))
	require.Equal(t, 429, call())
	unavailable = true
	record, err := s.GetToken("tb-share-clock")
	require.NoError(t, err)
	require.True(t, record.QuotaResetsAt.Equal(deadline))
	require.Eventually(t, func() bool { return time.Now().After(deadline) }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 200, call()) // Learned deadline works even if fetching fails.
	require.Equal(t, 429, call())
	unavailable = false
	for i := 0; i < 3; i++ {
		require.Equal(t, 429, call())
	} // Stale snapshot cannot reset twice.
	record, err = s.GetToken("tb-share-clock")
	require.NoError(t, err)
	require.Equal(t, int64(20), record.WeeklyTokensUsed)
	require.NoError(t, s.Close())
	s, err = db.NewAPITokenStore(dir)
	require.NoError(t, err)
	defer s.Close()
	q, err = quota.NewGormStoreOverDB(s.GetDB(), logrus.New())
	require.NoError(t, err)
	bind()
	r = router()
	require.Equal(t, 429, call())
	next := time.Now().Add(time.Hour)
	snapshot.Windows[0].ResetsAt = &next
	require.NoError(t, q.Save(context.Background(), snapshot))
	require.Equal(t, 429, call()) // A new future clock does not erase this period's usage.
	record, err = s.GetToken("tb-share-clock")
	require.NoError(t, err)
	require.True(t, record.QuotaResetsAt.Equal(next))
}
