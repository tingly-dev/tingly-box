package server

import (
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func TestSelectGuardrailsRegistryPolicyFragment_MatchSingleFromMultiPolicyFragment(t *testing.T) {
	cfg := guardrailscore.Config{
		Policies: []guardrailscore.Policy{
			{ID: "p1"},
			{ID: "p2"},
			{ID: "p3"},
		},
	}

	selected, err := selectGuardrailsRegistryPolicyFragment(cfg, "p2")
	if err != nil {
		t.Fatalf("selectGuardrailsRegistryPolicyFragment() error = %v", err)
	}
	if len(selected.Policies) != 1 {
		t.Fatalf("selected policy count = %d, want 1", len(selected.Policies))
	}
	if selected.Policies[0].ID != "p2" {
		t.Fatalf("selected policy id = %q, want p2", selected.Policies[0].ID)
	}
}

func TestSelectGuardrailsRegistryPolicyFragment_MissingPolicyReturnsError(t *testing.T) {
	cfg := guardrailscore.Config{
		Policies: []guardrailscore.Policy{
			{ID: "p1"},
		},
	}

	if _, err := selectGuardrailsRegistryPolicyFragment(cfg, "missing"); err == nil {
		t.Fatalf("expected missing policy error")
	}
}

func TestSelectGuardrailsRegistryPolicyFragment_DuplicatePolicyReturnsError(t *testing.T) {
	cfg := guardrailscore.Config{
		Policies: []guardrailscore.Policy{
			{ID: "p1"},
			{ID: "p1"},
		},
	}

	if _, err := selectGuardrailsRegistryPolicyFragment(cfg, "p1"); err == nil {
		t.Fatalf("expected duplicate policy error")
	}
}
