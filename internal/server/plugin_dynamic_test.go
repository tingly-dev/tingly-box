package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newDynamicPluginServer(t *testing.T) *Server {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	reg := NewPluginRegistry()
	cfg.SetEphemeralProviderResolver(reg)
	return &Server{config: cfg, pluginRegistry: reg}
}

func TestRegisterPluginDynamic_RoutesAndExpires(t *testing.T) {
	s := newDynamicPluginServer(t)

	w, resp := postJSON(t, s.RegisterPluginDynamic, RegisterPluginDynamicRequest{
		Name:     "my-rag",
		Endpoint: "http://127.0.0.1:8765/v1",
		ModelID:  "plugin/my-rag",
		Scenario: string(typ.ScenarioExperiment),
	})
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	data, _ := resp["data"].(map[string]any)
	pluginID, _ := data["plugin_id"].(string)
	leaseID, _ := data["lease_id"].(string)
	if pluginID == "" || leaseID == "" {
		t.Fatalf("missing ids: %v", data)
	}

	// No persistent provider was created.
	for _, p := range s.config.ListProviders() {
		if p.UUID == pluginID {
			t.Fatalf("dynamic registration must NOT persist a provider")
		}
	}

	// But routing resolution (the real dispatch chokepoint) finds the live instance.
	prov, err := s.config.GetProviderByUUID(pluginID)
	if err != nil || !prov.IsPlugin() {
		t.Fatalf("expected live ephemeral resolution, err=%v prov=%+v", err, prov)
	}

	// The durable rule (the name) was bound to the plugin id.
	var bound bool
	for _, rule := range s.config.GetRequestConfigs() {
		if rule.GetScenario() == typ.ScenarioExperiment && rule.RequestModel == "plugin/my-rag" {
			bound = true
			if rule.Services[0].Provider != pluginID {
				t.Fatalf("rule service should reference plugin id, got %s", rule.Services[0].Provider)
			}
		}
	}
	if !bound {
		t.Fatalf("expected a durable rule bound to the plugin")
	}

	// After deregister, the instance is gone → routing can no longer resolve it
	// (→ tier failover in a real request).
	postJSON(t, s.DeregisterPlugin, PluginLeaseRequest{LeaseID: leaseID})
	if _, err := s.config.GetProviderByUUID(pluginID); err == nil {
		t.Fatalf("provider must be unresolved after deregister")
	}
}

func TestHeartbeatPlugin_UnknownLease(t *testing.T) {
	s := newDynamicPluginServer(t)
	w, _ := postJSON(t, s.HeartbeatPlugin, PluginLeaseRequest{LeaseID: "nope"})
	if w.Code != 404 {
		t.Fatalf("expected 404 for unknown lease, got %d", w.Code)
	}
}

func TestReRegisterIsIdempotentForRule(t *testing.T) {
	s := newDynamicPluginServer(t)
	postJSON(t, s.RegisterPluginDynamic, RegisterPluginDynamicRequest{
		Name: "p", Endpoint: "http://a/v1", Scenario: string(typ.ScenarioExperiment),
	})
	postJSON(t, s.RegisterPluginDynamic, RegisterPluginDynamicRequest{
		Name: "p", Endpoint: "http://b/v1", Scenario: string(typ.ScenarioExperiment),
	})
	// exactly one rule for plugin/p
	count := 0
	for _, rule := range s.config.GetRequestConfigs() {
		if rule.RequestModel == "plugin/p" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("re-register must not duplicate the rule, got %d", count)
	}
}
