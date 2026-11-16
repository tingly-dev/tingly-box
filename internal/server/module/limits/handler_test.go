package limits

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestStatusReportsLiveCounters(t *testing.T) {
	dir, err := os.MkdirTemp("", "limits-status-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "acct", Name: "acct", Enabled: true, APIStyle: protocol.APIStyleOpenAI, APIBase: "https://x.example/v1",
		Limits: &ai.Limits{MaxConcurrent: 2},
	}))
	svc := &loadbalance.Service{Provider: "acct", Model: "m", Active: true, Limits: &ai.Limits{MaxRequests: 10, WindowSeconds: 60}}
	require.NoError(t, cfg.AddRequestConfig(typ.Rule{UUID: "r1", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt", Services: []*loadbalance.Service{svc}, Active: true}))

	guard := routing.NewLimitsGuard(limits.NewRegistry(), cfg)
	p, ok := guard.Acquire("r1", svc, false)
	require.True(t, ok)
	defer p.Release()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/limits/status", nil)
	NewHandler(cfg, guard).Status(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp LimitsStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	byScope := map[string]LimitStatusEntry{}
	for _, e := range resp.Data {
		byScope[e.Scope] = e
	}
	require.Equal(t, 1, byScope["account"].Inflight)
	require.InDelta(t, 0.5, byScope["account"].Pressure, 1e-9)
	require.Equal(t, "ok", byScope["account"].State)
	require.Equal(t, int64(1), byScope["service"].Requests)
	require.Equal(t, "r1", byScope["service"].RuleUUID)

	// limits off -> empty list, not an error
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/limits/status", nil)
	NewHandler(cfg, nil).Status(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`)
}
