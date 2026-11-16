package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newTestAgentConfig(t *testing.T) *serverconfig.Config {
	t.Helper()
	cfg, err := serverconfig.NewConfig(
		serverconfig.WithConfigDir(t.TempDir()),
		serverconfig.WithDisableBuiltIn(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return cfg
}

func TestErrUnsupportedAgentType_Error(t *testing.T) {
	err := ErrUnsupportedAgentType{AgentType: agent.AgentType("bogus")}
	if got := err.Error(); !strings.Contains(got, "bogus") {
		t.Errorf("Error() = %q, expected it to contain %q", got, "bogus")
	}
}

func TestAgentUseCase_RoutingKey(t *testing.T) {
	uc := NewAgentUseCase(newTestAgentConfig(t), "localhost")

	tests := []struct {
		name             string
		agentType        agent.AgentType
		wantRequestModel string
		wantScenario     typ.RuleScenario
		wantErr          bool
	}{
		{
			name:             "claude code",
			agentType:        agent.AgentTypeClaudeCode,
			wantRequestModel: "cc",
			wantScenario:     typ.ScenarioClaudeCode,
		},
		{
			name:             "opencode",
			agentType:        agent.AgentTypeOpenCode,
			wantRequestModel: "tingly-opencode",
			wantScenario:     typ.ScenarioOpenCode,
		},
		{
			name:             "codex",
			agentType:        agent.AgentTypeCodex,
			wantRequestModel: "tingly-codex",
			wantScenario:     typ.ScenarioCodex,
		},
		{
			name:      "unsupported type errors rather than falling back",
			agentType: agent.AgentType("not-a-real-agent"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestModel, scenario, err := uc.RoutingKey(tt.agentType)
			if tt.wantErr {
				var target ErrUnsupportedAgentType
				if !errors.As(err, &target) {
					t.Fatalf("expected ErrUnsupportedAgentType, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RoutingKey: %v", err)
			}
			if requestModel != tt.wantRequestModel {
				t.Errorf("requestModel = %q, want %q", requestModel, tt.wantRequestModel)
			}
			if scenario != tt.wantScenario {
				t.Errorf("scenario = %q, want %q", scenario, tt.wantScenario)
			}
		})
	}
}

func TestAgentUseCase_ResolveRouting(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	t.Run("no rule configured yet", func(t *testing.T) {
		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if res.RuleFound {
			t.Error("expected RuleFound=false with no rule configured")
		}
		if res.RequestModel != "cc" {
			t.Errorf("RequestModel = %q, want %q", res.RequestModel, "cc")
		}
	})

	t.Run("rule exists with a usable provider", func(t *testing.T) {
		provider := &typ.Provider{
			UUID: serverconfig.GenerateUUID(), Name: "test-provider",
			APIBase: "https://api.example.com", APIStyle: "openai",
			AuthType: typ.AuthTypeAPIKey, Token: "sk-test", Enabled: true,
		}
		if err := cfg.AddProvider(provider); err != nil {
			t.Fatalf("AddProvider: %v", err)
		}
		ruleUC := NewRuleUseCase(cfg)
		if _, err := ruleUC.Create(CreateRuleRequest{
			Scenario:     typ.ScenarioClaudeCode,
			RequestModel: "cc",
			Services: []*loadbalance.Service{
				{Provider: provider.UUID, Model: "claude-sonnet", Weight: 1, Active: true},
			},
		}); err != nil {
			t.Fatalf("RuleUseCase.Create: %v", err)
		}

		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if !res.RuleFound {
			t.Fatal("expected RuleFound=true")
		}
		if !res.ServiceUsable {
			t.Fatal("expected ServiceUsable=true with a valid provider")
		}
		if res.ProviderUUID != provider.UUID {
			t.Errorf("ProviderUUID = %q, want %q", res.ProviderUUID, provider.UUID)
		}
		if res.Model != "claude-sonnet" {
			t.Errorf("Model = %q, want %q", res.Model, "claude-sonnet")
		}
	})

	t.Run("unsupported agent type propagates the error", func(t *testing.T) {
		_, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentType("bogus")})
		var target ErrUnsupportedAgentType
		if !errors.As(err, &target) {
			t.Fatalf("expected ErrUnsupportedAgentType, got %v", err)
		}
	})

	t.Run("rule exists but its service has no model set", func(t *testing.T) {
		provider := &typ.Provider{
			UUID: serverconfig.GenerateUUID(), Name: "opencode-provider",
			APIBase: "https://api.example.com", APIStyle: "openai",
			AuthType: typ.AuthTypeAPIKey, Token: "sk-test", Enabled: true,
		}
		if err := cfg.AddProvider(provider); err != nil {
			t.Fatalf("AddProvider: %v", err)
		}
		ruleUC := NewRuleUseCase(cfg)
		if _, err := ruleUC.Create(CreateRuleRequest{
			Scenario:     typ.ScenarioOpenCode,
			RequestModel: "tingly-opencode",
			Services:     []*loadbalance.Service{{Provider: provider.UUID, Weight: 1, Active: true}}, // no Model
		}); err != nil {
			t.Fatalf("RuleUseCase.Create: %v", err)
		}

		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeOpenCode})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if !res.RuleFound {
			t.Fatal("expected RuleFound=true")
		}
		if res.ServiceUsable {
			t.Error("expected ServiceUsable=false when the service has no model")
		}
		if res.ProviderUUID != provider.UUID {
			t.Errorf("ProviderUUID = %q, want unusable service identity %q", res.ProviderUUID, provider.UUID)
		}
	})

	t.Run("rule without services is still found", func(t *testing.T) {
		// Codex has no built-in default rule (unlike Claude Code/OpenCode
		// above), so create one explicitly here rather than relying on seeding.
		provider := &typ.Provider{
			UUID: serverconfig.GenerateUUID(), Name: "codex-provider",
			APIBase: "https://api.example.com", APIStyle: "openai",
			AuthType: typ.AuthTypeAPIKey, Token: "sk-test", Enabled: true,
		}
		if err := cfg.AddProvider(provider); err != nil {
			t.Fatalf("AddProvider: %v", err)
		}
		ruleUC := NewRuleUseCase(cfg)
		if _, err := ruleUC.Create(CreateRuleRequest{
			Scenario:     typ.ScenarioCodex,
			RequestModel: "tingly-codex",
			Services:     []*loadbalance.Service{{Provider: provider.UUID, Model: "codex-model", Weight: 1, Active: true}},
		}); err != nil {
			t.Fatalf("RuleUseCase.Create: %v", err)
		}

		rule := cfg.GetRuleByRequestModelAndScenario("tingly-codex", typ.ScenarioCodex)
		if rule == nil {
			t.Fatal("expected the codex rule just created")
		}
		rule.Services = nil
		if err := cfg.UpdateRule(rule.UUID, *rule); err != nil {
			t.Fatalf("UpdateRule: %v", err)
		}

		res, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: agent.AgentTypeCodex})
		if err != nil {
			t.Fatalf("ResolveRouting: %v", err)
		}
		if !res.RuleFound {
			t.Fatal("expected RuleFound=true for an existing empty rule")
		}
		if res.ServiceUsable {
			t.Error("expected ServiceUsable=false for an empty rule")
		}
	})
}

func TestAgentUseCase_Show(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	t.Run("known agent type", func(t *testing.T) {
		res, err := uc.Show(ShowRequest{AgentType: agent.AgentTypeClaudeCode})
		if err != nil {
			t.Fatalf("Show: %v", err)
		}
		if res.Info.Type != agent.AgentTypeClaudeCode {
			t.Errorf("Info.Type = %q, want %q", res.Info.Type, agent.AgentTypeClaudeCode)
		}
		if res.Routing.RequestModel != "cc" {
			t.Errorf("Routing.RequestModel = %q, want %q", res.Routing.RequestModel, "cc")
		}
	})

	t.Run("unknown agent type", func(t *testing.T) {
		_, err := uc.Show(ShowRequest{AgentType: agent.AgentType("bogus")})
		if err == nil {
			t.Fatal("expected an error for an unknown agent type")
		}
	})
}

// TestAgentUseCase_Apply_WritesConfigFilesOnly exercises Apply as a thin
// wrapper over agent.AgentApply.ApplyAgent (internal/agent/rule_bridge.go),
// which already has its own file-writing test coverage in ai/agent — this
// only confirms the wrapper delegates and returns a usable result, using
// the config-files-only path (no Provider/Model) so it doesn't also need a
// seeded provider.
func TestAgentUseCase_Apply_WritesConfigFilesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	result, err := uc.Apply(&agent.ApplyAgentRequest{AgentType: agent.AgentTypeClaudeCode})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true, got %+v", result)
	}
	if len(result.ConfigFiles) == 0 {
		t.Error("expected at least one config file to be written")
	}
}

func TestAgentUseCase_Apply_InvalidAgentType(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	if _, err := uc.Apply(&agent.ApplyAgentRequest{AgentType: agent.AgentType("bogus")}); err == nil {
		t.Fatal("expected an error for an invalid agent type")
	}
}

// TestAgentUseCase_Restore_RoundTrip confirms Restore delegates to
// agent.AgentApply.RestoreAgent and can round-trip a file Apply just wrote.
func TestAgentUseCase_Restore_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	// Apply twice so there's a backup to restore from. Backups use
	// second-granularity timestamps (see ai/agent/restore_test.go), so sleep
	// past a timestamp boundary between the two calls, and again before
	// Restore — its own pre-restore backup would otherwise collide with the
	// backup the second Apply just created.
	if _, err := uc.Apply(&agent.ApplyAgentRequest{AgentType: agent.AgentTypeClaudeCode}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := uc.Apply(&agent.ApplyAgentRequest{AgentType: agent.AgentTypeClaudeCode}); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)

	result, err := uc.Restore(&agent.RestoreAgentRequest{AgentType: agent.AgentTypeClaudeCode})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.RestoredFiles) == 0 {
		t.Error("expected at least one file to be restored")
	}
}

func TestAgentUseCase_Restore_InvalidAgentType(t *testing.T) {
	cfg := newTestAgentConfig(t)
	uc := NewAgentUseCase(cfg, "localhost")

	if _, err := uc.Restore(&agent.RestoreAgentRequest{AgentType: agent.AgentType("bogus")}); err == nil {
		t.Fatal("expected an error for an invalid agent type")
	}
}
