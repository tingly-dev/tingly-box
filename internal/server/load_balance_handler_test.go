package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/clock"
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

// TestPreviewService_DoesNotClaimProbe covers the horizontal (flat) path:
// PreviewService may pick a half-open service but must leave its single
// recovery probe slot claimable for real traffic.
func TestPreviewService_DoesNotClaimProbe(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	base := time.Unix(2_000_000_000, 0)
	now := base
	restore := clock.SetClock(func() time.Time { return now })
	defer restore()

	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	lb := NewLoadBalancer(cfg, typ.NewHealthFilter(nil))

	a := &loadbalance.Service{Provider: "prev-a", Model: "m", Active: true}
	b := &loadbalance.Service{Provider: "prev-b", Model: "m", Active: true}
	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "preview-test",
		UUID:         uuid.New().String(),
		Services:     []*loadbalance.Service{a, b},
		Active:       true,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
	}

	// Trip A and pass the open window so it is half-open-eligible.
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(rule.UUID, a.ServiceID())
	}
	now = now.Add(loadbalance.DefaultBreakerOpenDuration + time.Second)

	// Preview many times (random may pick either service); the half-open
	// probe slot must remain claimable throughout.
	for i := 0; i < 20; i++ {
		svc, err := lb.PreviewService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
	}
	require.True(t, store.Allow(rule.UUID, a.ServiceID()),
		"preview must not consume the half-open probe slot")
}

// TestUpdateRuleTactic_CanonicalParsing pins the partial-update contract:
// the tactic name is validated strictly (typos → 400, aliases resolve), and
// params decode through Tactic.UnmarshalJSON — the same path as a full rule
// save — so the two write surfaces cannot drift.
func TestUpdateRuleTactic_CanonicalParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	rule := typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "tactic-test",
		UUID:         uuid.New().String(),
		Services: []*loadbalance.Service{
			{Provider: "prov-a", Model: "m", Active: true},
			{Provider: "prov-b", Model: "m", Active: true, Tier: 1},
		},
		Active: true,
	}
	require.NoError(t, cfg.AddOrUpdateRequestConfigByRequestModel(rule))

	api := NewLoadBalancerAPI(NewLoadBalancer(cfg, typ.NewHealthFilter(nil)), cfg)

	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Params = gin.Params{{Key: "ruleId", Value: rule.UUID}}
		c.Request = httptest.NewRequest(http.MethodPut,
			"/api/v1/load-balancer/rules/"+rule.UUID+"/tactic",
			bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		api.UpdateRuleTactic(c)
		return rec
	}

	// Params decode via the canonical polymorphic path.
	rec := do(`{"tactic":"tier","params":{"within_tier_tactic":"token_based"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := cfg.GetRuleByUUID(rule.UUID)
	require.Equal(t, loadbalance.TacticTier, got.LBTactic.Type)
	tp, ok := got.LBTactic.Params.(*typ.TierParams)
	require.True(t, ok, "params must decode to *TierParams, got %T", got.LBTactic.Params)
	require.Equal(t, loadbalance.TacticTokenBased, tp.WithinTierTactic)

	// Deprecated alias resolves to the canonical tactic and is echoed as such.
	rec = do(`{"tactic":"priority"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"tactic":"tier"`)

	// No params → the tactic's defaults, not zero values.
	rec = do(`{"tactic":"token_based"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	got = cfg.GetRuleByUUID(rule.UUID)
	require.Equal(t, loadbalance.TacticTokenBased, got.LBTactic.Type)

	// Unknown names are rejected instead of silently degrading to random.
	rec = do(`{"tactic":"latency_basd"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Case-insensitive like the old validator, but now actually applied.
	rec = do(`{"tactic":"RANDOM"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"tactic":"random"`)
}
