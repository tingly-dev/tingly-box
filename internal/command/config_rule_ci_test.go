package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestResolveProviderRef covers the three branches that `config rule add`'s
// --provider flag depends on: UUID hit, unique name hit, ambiguous name
// rejection. The third branch is the load-bearing one — silently picking
// "the first match" would mis-route rules.
func TestResolveProviderRef(t *testing.T) {
	am := newTestAppManager(t)

	uniqUUID, err := am.AddProvider("only-mine", "https://a.example", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider unique: %v", err)
	}
	dupAUUID, err := am.AddProvider("dup", "https://b.example", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider dup-a: %v", err)
	}
	dupBUUID, err := am.AddProvider("dup", "https://c.example", "tok", protocol.APIStyleOpenAI)
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

	providerUUID, err := am.AddProvider("openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	withSilencedStdout(t, func() {
		if err := runRuleAddCI(am, "openai", "gpt-4o", "openai", "gpt-4o"); err != nil {
			t.Fatalf("runRuleAddCI: %v", err)
		}
	})

	rule := am.GetRuleByRequestModelAndScenario("gpt-4o", typ.ScenarioOpenAI)
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

// TestConfigRuleAddCmdPartialFlagsErrors guards the "all or nothing" flag
// contract — partial flags must not silently drop into bufio prompts, which
// would hang a CI job on a TTY read.
func TestConfigRuleAddCmdPartialFlagsErrors(t *testing.T) {
	am := newTestAppManager(t)
	if _, err := am.AddProvider("openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	cases := []struct {
		name string
		cmd  ConfigRuleAddCmdKong
	}{
		{"only scenario", ConfigRuleAddCmdKong{Scenario: "openai"}},
		{"missing model", ConfigRuleAddCmdKong{Scenario: "openai", RequestModel: "gpt-4o", Provider: "openai"}},
		{"missing provider", ConfigRuleAddCmdKong{Scenario: "openai", RequestModel: "gpt-4o", Model: "gpt-4o"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(am)
			if err == nil {
				t.Fatalf("expected partial-flags error")
			}
			if !strings.Contains(err.Error(), "partial flags") {
				t.Errorf("error should mention 'partial flags'; got: %v", err)
			}
		})
	}
}
