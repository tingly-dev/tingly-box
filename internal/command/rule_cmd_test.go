package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// rule update/delete/export are a pure CI surface, same as provider: every
// operation is a single, fully-parameterized invocation with no
// interactive fallback of any kind — picking something is `tingly-box tui`
// or Web UI work. rule add's all-required-flags contract is already
// covered by TestRuleAddCmdRequiresAllFlags in rule_ci_test.go.

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
