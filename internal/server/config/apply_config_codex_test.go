package config

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestBuildContextWindowsFromRules(t *testing.T) {
	cfg := &Config{Rules: []typ.Rule{
		{UUID: "c1", Scenario: typ.ScenarioCodex, RequestModel: "gpt-5-codex", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
		// Keys stay verbatim — including a [1m]-suffixed name — so they line
		// up with the request models collectCodexRuleModels puts in the catalog.
		{UUID: "c2", Scenario: typ.ScenarioCodex, RequestModel: "team/coder[1m]", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
		// Flag off → no entry.
		{UUID: "c3", Scenario: typ.ScenarioCodex, RequestModel: "plain", Active: true},
		// Inactive → no entry.
		{UUID: "c4", Scenario: typ.ScenarioCodex, RequestModel: "off", Active: false,
			Flags: typ.RuleFlags{Context1M: true}},
		// Non-Codex scenario → no entry.
		{UUID: "cc", Scenario: typ.ScenarioClaudeCode, RequestModel: "haiku", Active: true,
			Flags: typ.RuleFlags{Context1M: true}},
	}}

	got := BuildContextWindowsFromRules(cfg)

	want := map[string]int{
		"gpt-5-codex":    codex1MContextWindow,
		"team/coder[1m]": codex1MContextWindow,
	}
	if len(got) != len(want) {
		t.Fatalf("context windows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("contextWindows[%q] = %d, want %d", k, got[k], v)
		}
	}
}
