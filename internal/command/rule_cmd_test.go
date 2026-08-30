package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// TestResolveProviderRef covers the three branches that `rule add`'s
// --provider flag depends on: UUID hit, unique name hit, ambiguous name
// rejection. The third branch is the load-bearing one — silently picking
// "the first match" would mis-route rules.
func TestResolveProviderRef(t *testing.T) {
	am := newTestAppManager(t)

	uniqUUID, err := addProviderForTest(am, "only-mine", "https://a.example", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider unique: %v", err)
	}
	dupAUUID, err := addProviderForTest(am, "dup", "https://b.example", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider dup-a: %v", err)
	}
	dupBUUID, err := addProviderForTest(am, "dup", "https://c.example", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider dup-b: %v", err)
	}

	t.Run("by uuid", func(t *testing.T) {
		got, err := resolveProviderRef(am, uniqUUID)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if got != uniqUUID {
			t.Fatalf("want %s got %s", uniqUUID, got)
		}
	})

	t.Run("by unique name", func(t *testing.T) {
		got, err := resolveProviderRef(am, "only-mine")
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if got != uniqUUID {
			t.Fatalf("want %s got %s", uniqUUID, got)
		}
	})

	t.Run("case insensitive name", func(t *testing.T) {
		got, err := resolveProviderRef(am, "ONLY-MINE")
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if got != uniqUUID {
			t.Fatalf("want %s got %s", uniqUUID, got)
		}
	})

	t.Run("ambiguous name rejected", func(t *testing.T) {
		_, err := resolveProviderRef(am, "dup")
		if err == nil {
			t.Fatalf("expected error for ambiguous name")
		}
		// The error must surface both UUIDs so the operator can pick.
		msg := err.Error()
		if !strings.Contains(msg, dupAUUID) || !strings.Contains(msg, dupBUUID) {
			t.Fatalf("ambiguity error missing candidate UUIDs: %q", msg)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := resolveProviderRef(am, "does-not-exist")
		if err == nil {
			t.Fatalf("expected error for missing provider")
		}
	})
}

// TestRunRuleAddCI walks the CI rule-add path end-to-end: provider name →
// resolved UUID → rule persisted with the right scenario/request-model and a
// single Service pointing at the provider. Also asserts the duplicate-rule
// guard fires on a second add with the same scenario+request-model.
func TestRunRuleAddCI(t *testing.T) {
	am := newTestAppManager(t)

	providerUUID, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	withSilencedStdout(t, func() {
		if err := runRuleAddCI(am, "openai", "gpt-4o", "openai", "gpt-4o"); err != nil {
			t.Fatalf("runRuleAddCI: %v", err)
		}
	})

	rule := am.GetGlobalConfig().GetRuleByRequestModelAndScenario("gpt-4o", typ.ScenarioOpenAI)
	if rule == nil {
		t.Fatalf("rule not persisted")
	}
	if rule.Scenario != typ.ScenarioOpenAI {
		t.Errorf("scenario: want %q got %q", typ.ScenarioOpenAI, rule.Scenario)
	}
	if !rule.Active {
		t.Errorf("rule should be active")
	}
	if len(rule.Services) != 1 {
		t.Fatalf("want 1 service, got %d", len(rule.Services))
	}
	svc := rule.Services[0]
	if svc.Provider != providerUUID {
		t.Errorf("service.Provider: want %s got %s", providerUUID, svc.Provider)
	}
	if svc.Model != "gpt-4o" {
		t.Errorf("service.Model: want gpt-4o got %s", svc.Model)
	}
	if !svc.Active {
		t.Errorf("service should be active")
	}

	t.Run("duplicate rejected", func(t *testing.T) {
		err := runRuleAddCI(am, "openai", "gpt-4o", "openai", "gpt-4o")
		if err == nil {
			t.Fatalf("expected duplicate-rule error")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("error should mention duplicate; got: %v", err)
		}
	})

	t.Run("unknown provider rejected", func(t *testing.T) {
		err := runRuleAddCI(am, "openai", "gpt-4-turbo", "ghost", "gpt-4")
		if err == nil {
			t.Fatalf("expected provider-not-found error")
		}
	})
}

// TestRuleAddCmdRequiresAllFlags guards the "all or nothing" flag contract —
// rule add has no partial or interactive fallback of any kind; every flag
// is required, full stop.
func TestRuleAddCmdRequiresAllFlags(t *testing.T) {
	am := newTestAppManager(t)
	if _, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	cases := []struct {
		name string
		cmd  RuleAddCmdKong
	}{
		{"only scenario", RuleAddCmdKong{Scenario: "openai"}},
		{"missing model", RuleAddCmdKong{Scenario: "openai", RequestModel: "gpt-4o", Provider: "openai"}},
		{"missing provider", RuleAddCmdKong{Scenario: "openai", RequestModel: "gpt-4o", Model: "gpt-4o"}},
		{"none given", RuleAddCmdKong{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(am)
			if err == nil {
				t.Fatalf("expected a required-flags error")
			}
			if !strings.Contains(err.Error(), "required") {
				t.Errorf("error should mention the flags are required; got: %v", err)
			}
		})
	}
}

// rule update/delete/export are a pure CI surface, same as provider: every
// operation is a single, fully-parameterized invocation with no
// interactive fallback of any kind — picking something is `tingly-box tui`
// or Web UI work. rule add's all-required-flags contract is covered by
// TestRuleAddCmdRequiresAllFlags above.

func TestRuleUpdate_RequiresProviderAndModel_ClearError(t *testing.T) {
	cases := []RuleUpdateCmdKong{
		{UUID: "some-uuid"},
		{UUID: "some-uuid", Provider: "openai"},
		{UUID: "some-uuid", Model: "gpt-4o"},
	}
	for _, cmd := range cases {
		err := cmd.Run(nil)
		if err == nil {
			t.Fatalf("%+v: expected an error, got nil", cmd)
		}
		if !strings.Contains(err.Error(), "required") {
			t.Errorf("%+v: error = %q, want it to say --provider and --model are required", cmd, err.Error())
		}
	}
}

func TestRuleUpdate_ChangesService(t *testing.T) {
	am := newTestAppManager(t)
	if _, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI); err != nil {
		t.Fatalf("addProviderForTest: %v", err)
	}
	newProviderUUID, err := addProviderForTest(am, "openai-2", "https://api2.openai.com", "tok2", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("addProviderForTest: %v", err)
	}

	withSilencedStdout(t, func() {
		err = runRuleAddCI(am, "openai", "gpt-4o", "openai", "gpt-4o")
	})
	if err != nil {
		t.Fatalf("runRuleAddCI: %v", err)
	}
	rule := am.GetGlobalConfig().GetRuleByRequestModelAndScenario("gpt-4o", "openai")
	if rule == nil {
		t.Fatalf("rule not persisted")
	}

	withSilencedStdout(t, func() {
		err = (&RuleUpdateCmdKong{UUID: rule.UUID, Provider: "openai-2", Model: "gpt-4o-mini"}).Run(am)
	})
	if err != nil {
		t.Fatalf("RuleUpdateCmdKong.Run: %v", err)
	}

	updated, getErr := usecase.NewRuleUseCase(am.GetGlobalConfig()).Get(usecase.GetRuleRequest{UUID: rule.UUID})
	if getErr != nil {
		t.Fatalf("get after update: %v", getErr)
	}
	if len(updated.Rule.Services) != 1 {
		t.Fatalf("want 1 service, got %d", len(updated.Rule.Services))
	}
	svc := updated.Rule.Services[0]
	if svc.Provider != newProviderUUID {
		t.Errorf("service.Provider = %q, want %q", svc.Provider, newProviderUUID)
	}
	if svc.Model != "gpt-4o-mini" {
		t.Errorf("service.Model = %q, want gpt-4o-mini", svc.Model)
	}
}

func TestRuleDelete_RequiresYes_ClearError(t *testing.T) {
	err := (&RuleDeleteCmdKong{UUID: "some-uuid"}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "-y/--yes") {
		t.Errorf("error = %q, want it to mention -y/--yes", err.Error())
	}
}

func TestRuleDelete_WithYes_Deletes(t *testing.T) {
	am := newTestAppManager(t)
	if _, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI); err != nil {
		t.Fatalf("addProviderForTest: %v", err)
	}
	var err error
	withSilencedStdout(t, func() {
		err = runRuleAddCI(am, "openai", "gpt-4o", "openai", "gpt-4o")
	})
	if err != nil {
		t.Fatalf("runRuleAddCI: %v", err)
	}
	rule := am.GetGlobalConfig().GetRuleByRequestModelAndScenario("gpt-4o", "openai")
	if rule == nil {
		t.Fatalf("rule not persisted")
	}

	withSilencedStdout(t, func() {
		err = (&RuleDeleteCmdKong{UUID: rule.UUID, Yes: true}).Run(am)
	})
	if err != nil {
		t.Fatalf("RuleDeleteCmdKong.Run: %v", err)
	}
	if got := am.GetGlobalConfig().GetRuleByRequestModelAndScenario("gpt-4o", "openai"); got != nil {
		t.Error("rule still exists after delete")
	}
}

func TestRuleExport_RequiresUUID(t *testing.T) {
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&RuleExportCmdKong{UUID: "00000000-0000-0000-0000-000000000000"}).Run(am)
	})
	if err == nil {
		t.Fatal("expected a not-found error for a bogus UUID, got nil")
	}
}
