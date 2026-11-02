package protocolserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newDualProviderRouter builds a real gateway stack (config, routing
// selector, client pool, full route registration) around one dual provider
// whose primary APIBase is a dead address and whose OpenAI-side dual URL is
// upstreamURL: a request that reaches the upstream proves the handler
// resolved the dual endpoint via ResolveStyle. One rule binds requestModel
// under scenario to the provider's upstreamModel.
func newDualProviderRouter(t *testing.T, upstreamURL string, apiStyle protocol.APIStyle, scenario typ.RuleScenario, requestModel, upstreamModel string) *gin.Engine {
	return newDualProviderRouterWithProxy(t, upstreamURL, "", apiStyle, scenario, requestModel, upstreamModel)
}

func newDualProviderRouterWithProxy(t *testing.T, upstreamURL, proxyURL string, apiStyle protocol.APIStyle, scenario typ.RuleScenario, requestModel, upstreamModel string) *gin.Engine {
	t.Helper()
	loadbalance.DefaultBreakerStore().Reset()

	cfg, err := config.NewConfig(
		config.WithConfigDir(t.TempDir()),
		config.WithDisableMigration(),
		config.WithDisableBuiltIn(),
	)
	require.NoError(t, err)
	providerID := "prov-dual"
	if proxyURL != "" {
		providerID += "-" + t.Name()
	}
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: providerID, Name: "prov-dual", Enabled: true,
		AuthType: typ.AuthTypeAPIKey, Token: "test-key", ProxyURL: proxyURL,
		APIStyle:         apiStyle,
		APIBase:          "http://127.0.0.1:1/v1",
		APIBaseOpenAI:    upstreamURL + "/v1",
		APIBaseAnthropic: "http://127.0.0.1:1/anthropic",
	}))
	require.NoError(t, cfg.AddRule(typ.Rule{
		Scenario: scenario, RequestModel: requestModel, Active: true,
		Services: []*loadbalance.Service{{Provider: providerID, Model: upstreamModel, Active: true}},
	}))

	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{
		Config:          cfg,
		ClientPool:      client.NewClientPool(),
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)),
		LoadBalancer:    lb,
		HealthMonitor:   hm,
	})

	engine := gin.New()
	ph.RegisterRoutes(engine, func(c *gin.Context) { c.Next() })
	return engine
}

// newPathRecordingUpstream serves payload as JSON for every request and
// returns a getter for the most recent request path (mutex-guarded: the
// server handler runs on its own goroutine).
func newPathRecordingUpstream(t *testing.T, payload map[string]any) (*httptest.Server, func() string) {
	t.Helper()
	var mu sync.Mutex
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return lastPath
	}
}
