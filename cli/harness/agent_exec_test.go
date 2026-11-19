package main

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// Every agent `harness agent` can run needs a virtual driver, and the model
// the virtual gateway registers must be the one the built-in rule serves (the
// real-provider path sends BuiltinRequestModel to the same drivers).
func TestVirtualAgentsCoverEveryAgent(t *testing.T) {
	for _, name := range batchAgents {
		agentType := parseAgentType(name)
		spec, ok := virtualAgents[agentType]
		if !ok || spec.run == nil || spec.provider == "" || spec.model == "" {
			t.Fatalf("agent %q has no complete virtual driver: %+v", name, spec)
		}
		if agentType != protocoltest.AgentTypeClaudeCode && spec.model != protocoltest.BuiltinRequestModel(agentType) {
			t.Errorf("agent %q: virtual model %q != built-in request model %q", name, spec.model, protocoltest.BuiltinRequestModel(agentType))
		}
	}
	if _, err := executeAgentCommand("nope", "hi"); err == nil {
		t.Fatal("unknown agent type must be rejected")
	}
}

func TestApplyAgentResultCopiesOutcomeButNotDuration(t *testing.T) {
	r := &RealAgentTestResult{Duration: 7}
	applyAgentResult(r, &AgentTestResult{
		Success: true, TimedOut: true, Output: "out", Error: "err", ExitCode: 3, Duration: 99,
	})
	want := RealAgentTestResult{Success: true, TimedOut: true, Output: "out", Error: "err", ExitCode: 3, Duration: 7}
	if *r != want {
		t.Fatalf("got %+v, want %+v", *r, want)
	}
}

func TestSelectRunnableEntries(t *testing.T) {
	ok := protocoltest.RealModelEntry{Name: "a-m1", Provider: "a", BaseURL: "https://a.test", APIKey: "k", Model: "m1", APIStyle: "openai"}
	noKey := ok
	noKey.Name, noKey.APIKey = "a-m2", "${UNSET_KEY}"
	other := ok
	other.Name, other.Provider = "b-m1", "B"
	entries := []protocoltest.RealModelEntry{ok, noKey, other}

	got, err := selectRunnableEntries(entries, "f.yaml", nil)
	if err != nil || len(got) != 2 || got[0].Name != "a-m1" || got[1].Name != "b-m1" {
		t.Fatalf("no filter: %v %+v", err, got)
	}
	got, err = selectRunnableEntries(entries, "f.yaml", []string{"b"})
	if err != nil || len(got) != 1 || got[0].Name != "b-m1" {
		t.Fatalf("filter b (case-insensitive): %v %+v", err, got)
	}
	if _, err := selectRunnableEntries(entries, "f.yaml", []string{"zzz"}); err == nil {
		t.Fatal("filter matching nothing must error")
	}
	if _, err := selectRunnableEntries([]protocoltest.RealModelEntry{noKey}, "f.yaml", nil); err == nil {
		t.Fatal("no runnable entries must error")
	}
}
