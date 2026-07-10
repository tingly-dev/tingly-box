//go:build e2e
// +build e2e

package servertest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// newTwoProviderRule builds a rule with two EQUAL-weight, equally healthy
// providers. This is the canonical "spread my traffic 50/50" setup users
// expect to load balance.
func newTwoProviderRule(tactic typ.Tactic) *typ.Rule {
	return &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test",
		UUID:         uuid.New().String(),
		Services: []*loadbalance.Service{
			{Provider: "provider1", Model: "model1", Weight: 1, Active: true, TimeWindow: 300},
			{Provider: "provider2", Model: "model2", Weight: 1, Active: true, TimeWindow: 300},
		},
		LBTactic: tactic,
		Active:   true,
	}
}

// simulate runs N requests through the load balancer, recording realistic
// per-request feedback (tokens, latency, speed) on whichever service was
// chosen — exactly like the real handlers do after a response completes.
// It returns the selection count per provider.
func simulate(t *testing.T, lb *server.LoadBalancer, rule *typ.Rule, n int) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
		counts[svc.Provider]++

		// Feedback: both providers behave identically (same latency, speed,
		// tokens). A correct balancer should therefore split ~50/50.
		svc.RecordUsage(100, 100)             // 200 tokens/request
		svc.Stats.RecordLatency(500, 100)     // 500ms, identical
		svc.Stats.RecordTokenSpeed(50.0, 100) // 50 tps, identical

		// The handler updates CurrentServiceID after each pick.
		lb.UpdateServiceIndex(rule, svc)
	}
	return counts
}

func report(t *testing.T, name string, counts map[string]int, total int) {
	p1 := counts["provider1"]
	p2 := counts["provider2"]
	share1 := 100.0 * float64(p1) / float64(total)
	share2 := 100.0 * float64(p2) / float64(total)
	t.Logf("[%s] provider1=%d (%.1f%%)  provider2=%d (%.1f%%)", name, p1, share1, p2, share2)
}

// TestLB_VirtualValidation_EqualProviders is a virtual validation that
// reproduces the reported bug: with two equal providers, traffic should split
// ~50/50, but several tactics concentrate almost everything on one provider.
func TestLB_VirtualValidation_EqualProviders(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	healthFilter := typ.NewHealthFilter(nil)

	const total = 1000

	cases := []struct {
		name   string
		tactic typ.Tactic
	}{
		{
			name:   "DEFAULT (unset type=0)",
			tactic: typ.Tactic{}, // Type==0: what a rule gets when no tactic is configured
		},
		{
			name:   "TokenBased(default 10000)",
			tactic: typ.Tactic{Type: loadbalance.TacticTokenBased, Params: typ.DefaultTokenBasedParams()},
		},
		{
			name:   "Random",
			tactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
		},
	}

	type result struct {
		name        string
		p1, p2      int
		maxSharePct float64
	}
	var results []result

	// shares keyed by case name, so per-tactic assertions below stay readable.
	shares := map[string]float64{}

	for _, c := range cases {
		// Fresh load balancer + fresh rule so stats don't leak between cases.
		lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
		rule := newTwoProviderRule(c.tactic)
		counts := simulate(t, lb, rule, total)
		report(t, c.name, counts, total)

		p1, p2 := counts["provider1"], counts["provider2"]
		maxShare := 100.0 * float64(max(p1, p2)) / float64(total)
		shares[c.name] = maxShare
		results = append(results, result{c.name, p1, p2, maxShare})
	}

	// Regression guard: with two equal providers EVERY tactic must spread load.
	// Before the fix, the unset default (which resolved to the since-removed
	// adaptive scorer) concentrated ~95% on the first provider once token
	// scores saturated.
	for name, share := range shares {
		if share >= 65.0 {
			t.Errorf("%s concentrates %.1f%% on one provider (want <65%% for equal providers)", name, share)
		}
	}

	// Log a verdict table (visible under `go test -v`). A healthy balancer keeps
	// the dominant provider well under ~70% for equal providers; flag anything
	// that concentrates.
	t.Logf("================ LOAD BALANCING VIRTUAL VALIDATION ================")
	t.Logf("%-30s %8s %8s %14s", "TACTIC", "prov1", "prov2", "max-share")
	for _, r := range results {
		verdict := "OK"
		if r.maxSharePct >= 70.0 {
			verdict = "BAD (concentrated)"
		}
		t.Logf("%-30s %8d %8d %12.1f%%  %s", r.name, r.p1, r.p2, r.maxSharePct, verdict)
	}
	t.Logf("==================================================================")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
