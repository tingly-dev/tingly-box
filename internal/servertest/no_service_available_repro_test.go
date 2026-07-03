package servertest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/server/affinity"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestRepro_NoServiceAvailable_SingleServiceRateLimited reproduces the reported
// intermittent "no service available for rule ..." failure end-to-end through
// the real ServiceSelector pipeline.
//
// Scenario: a rule (like the built-in cc-default) has a single, correctly
// configured, active service. The service hits a transient 429. The health
// monitor marks it unhealthy for the recovery window. From that moment every
// request to the rule fails with "no service available for rule" even though
// the rule and service are configured correctly and the upstream may already
// have recovered.
//
// Before the fix this test fails at the rate-limited step with that exact
// error. After the fix (fall back to the active set) selection still returns
// the service.
func TestRepro_NoServiceAvailable_SingleServiceRateLimited(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	cfg := appConfig.GetGlobalConfig()

	// A correctly configured, enabled provider.
	providerUUID := uuid.New().String()
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID:    providerUUID,
		Name:    "cc-default-provider",
		APIBase: "https://example.invalid",
		Token:   "sk-test",
		Enabled: true,
	}))

	// Build the real selection stack exactly as server.go wires it.
	healthMonitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	healthFilter := typ.NewHealthFilter(healthMonitor)
	lb := server.NewLoadBalancer(cfg, healthFilter)
	defer lb.Stop()
	affinityStore := affinity.NewAffinityStore(0)
	selector := routing.NewServiceSelector(cfg, affinityStore, lb)

	// A single-service rule, no smart routing / affinity -> no-affinity pipeline.
	svc := &loadbalance.Service{
		Provider:   providerUUID,
		Model:      "tingly/cc-default",
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}
	rule := &typ.Rule{
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: "tingly/cc-default",
		UUID:         "built-in-cc-default",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{svc},
		Active:   true,
	}

	ctx := &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}

	// Sanity: while healthy, selection works.
	res, err := selector.Select(ctx)
	require.NoError(t, err, "healthy single service should select fine")
	require.NotNil(t, res)
	require.Equal(t, "tingly/cc-default", res.Service.Model)

	// Now the only service hits a transient 429 -> marked unhealthy.
	healthMonitor.ReportRateLimit(svc.ServiceID())
	require.False(t, healthMonitor.IsHealthy(svc.ServiceID()),
		"service should be unhealthy right after a 429")

	// This is the reported failure point. Pre-fix: selector.Select returns
	// "no service available for rule built-in-cc-default ...". Post-fix: it
	// falls back to the active service and succeeds.
	res, err = selector.Select(ctx)
	require.NoError(t, err,
		"single rate-limited service must not blow up the whole rule")
	require.NotNil(t, res)
	require.Equal(t, "tingly/cc-default", res.Service.Model)
}
