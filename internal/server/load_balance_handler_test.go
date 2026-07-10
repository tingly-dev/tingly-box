package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestGetServicesHealth_ExposesBreakerState pins the API contract that makes
// tier failover visible: every service entry carries its rule-scoped breaker
// state and, while open, the seconds until the next recovery probe.
func TestGetServicesHealth_ExposesBreakerState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadbalance.DefaultBreakerStore().Reset()

	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	primary := &loadbalance.Service{Provider: "prov-a", Model: "model-a", Active: true, Tier: 0}
	backup := &loadbalance.Service{Provider: "prov-b", Model: "model-b", Active: true, Tier: 1}
	rule := typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "health-test",
		UUID:         uuid.New().String(),
		Services:     []*loadbalance.Service{primary, backup},
		Active:       true,
	}
	require.NoError(t, cfg.AddOrUpdateRequestConfigByRequestModel(rule))

	api := NewLoadBalancerAPI(NewLoadBalancer(cfg, typ.NewHealthFilter(nil)), cfg)

	// Trip the primary's breaker exactly as the failover loop does.
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		loadbalance.RecordServiceFailure(rule.UUID, primary.ServiceID())
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "ruleId", Value: rule.UUID}}
	api.GetServicesHealth(c)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp ServiceHealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	entry := func(id string) map[string]interface{} {
		raw, ok := resp.Health[id]
		require.True(t, ok, "health entry for %s missing", id)
		m, ok := raw.(map[string]interface{})
		require.True(t, ok)
		return m
	}

	tripped := entry(primary.ServiceID())
	require.Equal(t, "open", tripped["breaker_state"], "tripped primary must report an open breaker")
	require.Greater(t, tripped["breaker_retry_in_seconds"].(float64), 0.0,
		"an open breaker must report time until its next recovery probe")
	require.Equal(t, 0.0, tripped["tier"].(float64))

	healthy := entry(backup.ServiceID())
	require.Equal(t, "closed", healthy["breaker_state"])
	require.Equal(t, 0.0, healthy["breaker_retry_in_seconds"], "closed breaker reports no retry window")
	require.Equal(t, 1.0, healthy["tier"].(float64))
}
