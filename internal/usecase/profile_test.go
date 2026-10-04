package usecase

import (
	"errors"
	"testing"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newTestProfileConfig(t *testing.T) (*serverconfig.Config, typ.ProfileMeta, string) {
	t.Helper()
	cfg, err := serverconfig.NewConfig(
		serverconfig.WithConfigDir(t.TempDir()),
		serverconfig.WithDisableBuiltIn(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	provider := &typ.Provider{
		UUID:     serverconfig.GenerateUUID(),
		Name:     "profile-provider",
		APIBase:  "https://api.example.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "test-token",
		Enabled:  true,
	}
	if err := cfg.AddProvider(provider); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	profile, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work", false)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	profiledScenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID)
	rule := cfg.GetRuleByRequestModelAndScenario("default", profiledScenario)
	if rule == nil {
		t.Fatal("expected seeded profile rule")
	}
	rule.Services = []*loadbalance.Service{{
		Provider: provider.UUID,
		Model:    "claude-test",
		Weight:   1,
		Active:   true,
	}}
	if err := cfg.UpdateRule(rule.UUID, *rule); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	return cfg, profile, provider.UUID
}

func TestProfileUseCase_List(t *testing.T) {
	cfg, profile, _ := newTestProfileConfig(t)
	result := NewProfileUseCase(cfg).List(ListProfilesRequest{Scenario: typ.ScenarioClaudeCode})
	if len(result.Profiles) != 1 || result.Profiles[0].ID != profile.ID {
		t.Fatalf("List() = %#v, want profile %s", result.Profiles, profile.ID)
	}
}

func TestProfileUseCase_GetByIDOrName(t *testing.T) {
	cfg, profile, providerUUID := newTestProfileConfig(t)
	uc := NewProfileUseCase(cfg)

	for _, identifier := range []string{profile.ID, profile.Name} {
		t.Run(identifier, func(t *testing.T) {
			result, err := uc.Get(GetProfileRequest{
				Scenario:   typ.ScenarioClaudeCode,
				Identifier: identifier,
			})
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if result.Profile.ID != profile.ID {
				t.Errorf("profile ID = %q, want %q", result.Profile.ID, profile.ID)
			}
			if result.Scenario != typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID) {
				t.Errorf("scenario = %q", result.Scenario)
			}
			if len(result.Rules) != 6 {
				t.Fatalf("rules = %d, want 6", len(result.Rules))
			}
			// Assert the full ordering is non-decreasing by RequestModel, not
			// just Rules[0]. "default" sorts first lexicographically anyway, so
			// checking only the first element cannot catch a sort regression.
			for i := 1; i < len(result.Rules); i++ {
				if result.Rules[i-1].RequestModel > result.Rules[i].RequestModel {
					t.Errorf("rules not sorted by RequestModel: %q > %q at index %d",
						result.Rules[i-1].RequestModel, result.Rules[i].RequestModel, i)
				}
			}
			if !result.Rules[0].Configured || result.Rules[0].ProviderUUID != providerUUID || result.Rules[0].ProviderName != "profile-provider" {
				t.Errorf("configured rule = %#v", result.Rules[0])
			}
		})
	}
}

func TestProfileUseCase_ResolveDoesNotRequireRuleDetails(t *testing.T) {
	cfg, profile, _ := newTestProfileConfig(t)
	result, err := NewProfileUseCase(cfg).Resolve(GetProfileRequest{
		Scenario:   typ.ScenarioClaudeCode,
		Identifier: profile.Name,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Profile.ID != profile.ID || result.Scenario != typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID) {
		t.Errorf("Resolve() = %#v", result)
	}
}

func TestProfileUseCase_GetNotFound(t *testing.T) {
	cfg, _, _ := newTestProfileConfig(t)
	_, err := NewProfileUseCase(cfg).Get(GetProfileRequest{
		Scenario:   typ.ScenarioClaudeCode,
		Identifier: "missing",
	})
	var notFound ErrProfileNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("Get error = %v, want ErrProfileNotFound", err)
	}
	if notFound.Identifier != "missing" || notFound.Scenario != typ.ScenarioClaudeCode {
		t.Errorf("error = %#v", notFound)
	}
}
