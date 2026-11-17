package protocolserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func limitsTestHandler(t *testing.T) (*ProtocolHandler, *routing.LimitsGuard, *config.Config) {
	t.Helper()
	dir, err := os.MkdirTemp("", "limits-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	require.NoError(t, err)
	for _, id := range []string{"acct-a", "acct-b"} {
		require.NoError(t, cfg.AddProvider(&typ.Provider{
			UUID: id, Name: id, Enabled: true, APIStyle: protocol.APIStyleOpenAI, APIBase: "https://" + id + ".example.invalid/v1",
		}))
	}
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	sel := routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)
	guard := routing.NewLimitsGuard(limits.NewRegistry(), cfg)
	sel.SetLimits(guard)
	h := NewHandler(ProtocolHandlerDeps{Config: cfg, LoadBalancer: lb, HealthMonitor: hm, RoutingSelector: routing.NewSimpleSelector(sel)})
	return h, guard, cfg
}

func ginCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	return c
}

func TestLimitsMiddleware_ReleasesOnReturnAndPanic(t *testing.T) {
	h, guard, _ := limitsTestHandler(t)
	svc := &loadbalance.Service{Provider: "acct-a", Model: "m", Active: true, Limits: &ai.Limits{MaxConcurrent: 1}}
	inflight := func() int { return guard.Registry().Stat(routing.ServiceLimitKey("r", svc)).Inflight }

	engine := gin.New()
	engine.Use(gin.Recovery(), h.limitsMiddleware)
	take := func(c *gin.Context) {
		p, ok := guard.Acquire("r", svc, false)
		require.True(t, ok)
		routing.LeaseFromGin(c).Hold(svc.ServiceID(), p)
		require.Equal(t, 1, inflight())
	}
	engine.GET("/ok", func(c *gin.Context) { take(c); c.Status(200) })
	engine.GET("/panic", func(c *gin.Context) { take(c); panic("boom") })

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/ok", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, 0, inflight(), "normal return releases")

	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/panic", nil))
	require.Equal(t, 500, w.Code)
	require.Equal(t, 0, inflight(), "panic must not leak the slot")
}

func TestWriteSelectionError(t *testing.T) {
	c := ginCtx()
	writeSelectionError(c, &routing.LimitError{Reasons: limits.ReasonUsage, RetryAfter: 90 * time.Second})
	w := c.Writer.(interface{ Status() int })
	require.Equal(t, http.StatusTooManyRequests, w.Status())
	require.Equal(t, "90", c.Writer.Header().Get("Retry-After"))

	c = ginCtx()
	writeSelectionError(c, errors.New("no service"))
	require.Equal(t, http.StatusBadRequest, c.Writer.Status())
	require.Empty(t, c.Writer.Header().Get("Retry-After"))
}

func TestFailover_ReleasesFailedAttemptAndRespectsLimits(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	h, guard, _ := limitsTestHandler(t)
	a := &loadbalance.Service{Provider: "acct-a", Model: "m", Active: true, Tier: 0, Limits: &ai.Limits{MaxConcurrent: 1}}
	b := &loadbalance.Service{Provider: "acct-b", Model: "m", Active: true, Tier: 1, Limits: &ai.Limits{MaxConcurrent: 1}}
	rule := &typ.Rule{
		UUID:     "fo-rule",
		LBTactic: typ.Tactic{Type: loadbalance.TacticTier, Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom}},
		Services: []*loadbalance.Service{a, b},
	}
	st := func(s *loadbalance.Service) int { return guard.Registry().Stat(routing.ServiceLimitKey(rule.UUID, s)).Inflight }

	// the initial attempt holds A (as selectService would)
	c := ginCtx()
	pa, ok := guard.Acquire(rule.UUID, a, false)
	require.True(t, ok)
	routing.LeaseFromGin(c).Hold(a.ServiceID(), pa)
	tried := map[string]bool{a.ServiceID(): true}

	// A failed -> its slot is released and B is acquired
	prov, svc, err := h.selectFallbackServiceLimited(c, rule, tried, a.ServiceID())
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.Equal(t, "acct-b", prov.UUID)
	require.Equal(t, 0, st(a), "failed attempt's slot must be freed")
	require.Equal(t, 1, st(b))

	// another request already saturates B: failover has nothing to try
	c2 := ginCtx()
	tried2 := map[string]bool{a.ServiceID(): true}
	prov, svc, err = h.selectFallbackServiceLimited(c2, rule, tried2, a.ServiceID())
	require.NoError(t, err)
	require.Nil(t, svc, "B is at its cap and failover never waits")
	require.Nil(t, prov)
	require.Equal(t, 1, st(b))

	routing.ReleaseLease(c)
	require.Equal(t, 0, st(b))
}
