package server

import (
	"os"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestSelectFallbackService_SpansAPIStyles proves the core of the lifted-failover
// change: with requireAPIStyle="" the fallback candidate pool spans heterogeneous
// API styles, so failover can rotate from an Anthropic-style provider to an
// OpenAI-style one. The same call pinned to "anthropic" returns no candidate,
// which is exactly the behaviour the old code was stuck with.
func TestSelectFallbackService_SpansAPIStyles(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()

	dir, err := os.MkdirTemp("", "crossstyle-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	if err != nil {
		t.Fatal(err)
	}

	const provAnthropic = "prov-anthropic"
	const provOpenAI = "prov-openai"
	if err := cfg.AddProvider(&typ.Provider{
		UUID: provAnthropic, Name: provAnthropic, Enabled: true,
		APIStyle: protocol.APIStyleAnthropic, APIBase: "https://anthropic.example.invalid",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.AddProvider(&typ.Provider{
		UUID: provOpenAI, Name: provOpenAI, Enabled: true,
		APIStyle: protocol.APIStyleOpenAI, APIBase: "https://openai.example.invalid/v1",
	}); err != nil {
		t.Fatal(err)
	}

	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	hf := typ.NewHealthFilter(hm)
	lb := NewLoadBalancer(cfg, hf)
	s := &Server{config: cfg, loadBalancer: lb, healthMonitor: hm}

	rule := &typ.Rule{
		UUID: "cross-style-rule",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Services: []*loadbalance.Service{
			{Provider: provAnthropic, Model: "claude", Active: true, Tier: 0},
			{Provider: provOpenAI, Model: "gpt-4", Active: true, Tier: 1},
		},
	}

	// The T0 Anthropic service has already been tried and failed.
	tried := map[string]bool{
		loadbalance.FormatServiceID(provAnthropic, "claude"): true,
	}

	// Heterogeneous pool ("") must fall over to the OpenAI provider.
	p, svc, err := s.selectFallbackService(rule, tried, "")
	if err != nil {
		t.Fatalf("selectFallbackService(\"\") error: %v", err)
	}
	if p == nil || svc == nil {
		t.Fatal("selectFallbackService(\"\") returned no candidate; cross-style failover blocked")
	}
	if p.UUID != provOpenAI || svc.Model != "gpt-4" {
		t.Fatalf("selectFallbackService(\"\") picked %s/%s, want %s/gpt-4", p.UUID, svc.Model, provOpenAI)
	}

	// Pinned to the original Anthropic style, the OpenAI candidate is filtered
	// out and the only remaining service is excluded → no candidate. This is the
	// limitation the lift removes.
	p2, svc2, err := s.selectFallbackService(rule, tried, protocol.APIStyleAnthropic)
	if err != nil {
		t.Fatalf("selectFallbackService(anthropic) error: %v", err)
	}
	if p2 != nil || svc2 != nil {
		t.Fatalf("selectFallbackService(anthropic) picked %v, want nil (style-pinned pool exhausted)", p2)
	}
}
