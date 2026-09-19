package routing_test

// Benchmarks for the production ServiceSelector pipeline (health → smart →
// affinity → load_balancer), wired exactly as internal/server/server.go does
// (protocolserver.LoadBalancer + routing.ServiceSelector), minus the HTTP
// layer. They exist to put real numbers behind "is the multi-stage pipeline
// (and its known double active/health filtering — see
// internal/protocolserver/load_balance.go's selectService, which re-filters
// candidates HealthStage already filtered) worth optimizing", rather than
// reasoning about it from first principles.
//
// Run: go test ./internal/routing/... -bench . -benchmem -run '^$'

import (
	"testing"

	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/appconfig"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocolserver"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// benchService and benchRequest are thin aliases over routing's exported
// test fixtures (ServiceForTest/OpenAIRequestForTest) — kept as local
// wrappers only to fix the benchmark's request model/weight/active shape
// once, not to reimplement the fixtures themselves.
func benchService(provider, model string) *loadbalance.Service {
	return routing.ServiceForTest(provider, model, true)
}

func benchRequest() *openai.ChatCompletionNewParams {
	return routing.OpenAIRequestForTest("bench-model")
}

// benchSelector wires the real production pipeline. cfg's rule list is left
// empty on purpose — Select() takes the *typ.Rule directly via
// SelectionContext, so benchmarks build rules in-memory without paying for
// AddRequestConfig's persisted-config write path.
func benchSelector(b *testing.B) (*routing.ServiceSelector, *appconfig.AppConfig) {
	b.Helper()
	appConfig, err := appconfig.NewAppConfig(appconfig.WithConfigDir(b.TempDir()))
	if err != nil {
		b.Fatal(err)
	}
	cfg := appConfig.GetGlobalConfig()

	healthMonitor := loadbalance.NewHealthMonitor(cfg.HealthMonitor)
	healthFilter := routing.NewHealthFilter(healthMonitor)
	lb := protocolserver.NewLoadBalancer(cfg, healthFilter)
	affinityStore := protocolserver.NewAffinityStore(0)
	sel := routing.NewServiceSelector(cfg, affinityStore, lb)
	return sel, appConfig
}

func benchProvider(b *testing.B, appConfig *appconfig.AppConfig, uuid string) {
	b.Helper()
	p := &typ.Provider{UUID: uuid, Name: uuid, APIBase: "http://bench.invalid", Enabled: true}
	if err := appConfig.GetGlobalConfig().AddProvider(p); err != nil {
		b.Fatal(err)
	}
}

func runSelectBench(b *testing.B, sel *routing.ServiceSelector, ctx *routing.SelectionContext) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sel.Select(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSelect_Plain is the simplest shape: no smart routing, no
// affinity, N services under the (default) random tactic. This isolates the
// pipeline's fixed per-request overhead — including the double active/health
// filter (once in HealthStage, once again inside LoadBalancer.SelectService)
// — from smart-routing/affinity-specific work.
func BenchmarkSelect_Plain(b *testing.B) {
	sel, appConfig := benchSelector(b)
	benchProvider(b, appConfig, "bench-a")
	benchProvider(b, appConfig, "bench-b")
	benchProvider(b, appConfig, "bench-c")

	rule := &typ.Rule{
		UUID:         "bench-rule-plain",
		RequestModel: "bench-model",
		Scenario:     typ.ScenarioOpenAI,
		Active:       true,
		Services: []*loadbalance.Service{
			benchService("bench-a", "m"),
			benchService("bench-b", "m"),
			benchService("bench-c", "m"),
		},
	}
	ctx := &routing.SelectionContext{
		Rule:                  rule,
		Request:               benchRequest(),
		SessionID:             typ.SessionID{Source: typ.SessionSourceIP, Value: "127.0.0.1"},
		Scenario:              typ.ScenarioOpenAI,
		MatchedSmartRuleIndex: -1,
	}
	runSelectBench(b, sel, ctx)
}

// BenchmarkSelect_SmartMatch narrows via a matching smart-routing partition
// on every request — the common "content routing hit" case.
func BenchmarkSelect_SmartMatch(b *testing.B) {
	sel, appConfig := benchSelector(b)
	benchProvider(b, appConfig, "bench-base")
	benchProvider(b, appConfig, "bench-smart")

	rule := &typ.Rule{
		UUID:         "bench-rule-smart-match",
		RequestModel: "bench-model",
		Scenario:     typ.ScenarioOpenAI,
		Active:       true,
		SmartEnabled: true,
		Services:     []*loadbalance.Service{benchService("bench-base", "m")},
		SmartRouting: []smartrouting.SmartRouting{{
			Description: "always",
			Ops: []smartrouting.SmartOp{{
				Position:  smartrouting.PositionModel,
				Operation: smartrouting.OpModelContains,
				Value:     "bench",
			}},
			Services: []*loadbalance.Service{benchService("bench-smart", "m")},
		}},
	}
	ctx := &routing.SelectionContext{
		Rule:                  rule,
		Request:               benchRequest(),
		SessionID:             typ.SessionID{Source: typ.SessionSourceIP, Value: "127.0.0.1"},
		Scenario:              typ.ScenarioOpenAI,
		MatchedSmartRuleIndex: -1,
	}
	runSelectBench(b, sel, ctx)
}

// BenchmarkSelect_SmartNoMatch exercises the lazy basePool fallback path in
// SmartRoutingStage: the partition never matches, so every request falls
// back to IntersectServices(candidates, rule.Services).
func BenchmarkSelect_SmartNoMatch(b *testing.B) {
	sel, appConfig := benchSelector(b)
	benchProvider(b, appConfig, "bench-base")
	benchProvider(b, appConfig, "bench-smart")

	rule := &typ.Rule{
		UUID:         "bench-rule-smart-nomatch",
		RequestModel: "bench-model",
		Scenario:     typ.ScenarioOpenAI,
		Active:       true,
		SmartEnabled: true,
		Services:     []*loadbalance.Service{benchService("bench-base", "m")},
		SmartRouting: []smartrouting.SmartRouting{{
			Description: "never",
			Ops: []smartrouting.SmartOp{{
				Position:  smartrouting.PositionModel,
				Operation: smartrouting.OpModelContains,
				Value:     "never-matches-this-request",
			}},
			Services: []*loadbalance.Service{benchService("bench-smart", "m")},
		}},
	}
	ctx := &routing.SelectionContext{
		Rule:                  rule,
		Request:               benchRequest(),
		SessionID:             typ.SessionID{Source: typ.SessionSourceIP, Value: "127.0.0.1"},
		Scenario:              typ.ScenarioOpenAI,
		MatchedSmartRuleIndex: -1,
	}
	runSelectBench(b, sel, ctx)
}

// BenchmarkSelect_Affinity exercises the affinity-hit short-circuit: the pin
// is written once (via a real Select() call, so it's locked exactly as
// production would), then every subsequent iteration returns from
// AffinityStage before HealthStage's sibling filter in LoadBalancer runs.
func BenchmarkSelect_Affinity(b *testing.B) {
	sel, appConfig := benchSelector(b)
	benchProvider(b, appConfig, "bench-pinned")
	benchProvider(b, appConfig, "bench-other")

	rule := &typ.Rule{
		UUID:         "bench-rule-affinity",
		RequestModel: "bench-model",
		Scenario:     typ.ScenarioOpenAI,
		Active:       true,
		Services:     []*loadbalance.Service{benchService("bench-pinned", "m"), benchService("bench-other", "m")},
	}
	rule.Flags.SessionAffinity = 3600

	ctx := &routing.SelectionContext{
		Rule:                  rule,
		Request:               benchRequest(),
		SessionID:             typ.SessionID{Source: typ.SessionSourceHeader, Value: "bench-session"},
		Scenario:              typ.ScenarioOpenAI,
		MatchedSmartRuleIndex: -1,
	}
	if _, err := sel.Select(ctx); err != nil {
		b.Fatal(err)
	}
	runSelectBench(b, sel, ctx)
}

// BenchmarkHealthFilter_Filter isolates a single health-filter pass so the
// double-filtering cost visible in BenchmarkSelect_Plain (HealthStage, then
// again inside LoadBalancer.SelectService) can be read off directly: each
// BenchmarkSelect_Plain op should cost roughly the pipeline's fixed overhead
// plus ~2x this number's ns/op and allocs/op.
func BenchmarkHealthFilter_Filter(b *testing.B) {
	monitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	filter := routing.NewHealthFilter(monitor)
	services := []*loadbalance.Service{
		benchService("bench-a", "m"),
		benchService("bench-b", "m"),
		benchService("bench-c", "m"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Filter(services)
	}
}
