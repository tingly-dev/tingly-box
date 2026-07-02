package config

import (
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestUpdateRule_CompactsTiers verifies that moving the sole tier-0 service to
// a higher tier doesn't leave tier 0 permanently empty: UpdateRule renumbers
// the persisted tiers so the lowest tier in use is always 0.
func TestUpdateRule_CompactsTiers(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{
				UUID:     "r1",
				Scenario: typ.ScenarioOpenAI,
				Active:   true,
				Services: []*loadbalance.Service{
					{Provider: "p1", Model: "m1", Tier: 0},
				},
			},
		},
	}

	// Simulate the frontend moving the sole T0 service down to T1.
	rule := c.Rules[0]
	rule.Services = []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Tier: 1},
	}
	if err := c.UpdateRule("r1", rule); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}

	if got := c.Rules[0].Services[0].Tier; got != 0 {
		t.Errorf("expected sole service to be compacted back to tier 0, got %d", got)
	}
}

// TestUpdateRule_CompactsTiersWithGap verifies a gap in the middle of the
// tier sequence (e.g. after deleting the tier-1 service) is closed too.
func TestUpdateRule_CompactsTiersWithGap(t *testing.T) {
	c := &Config{
		ConfigFile: filepath.Join(t.TempDir(), "config.json"),
		Rules: []typ.Rule{
			{
				UUID:     "r1",
				Scenario: typ.ScenarioOpenAI,
				Active:   true,
				Services: []*loadbalance.Service{
					{Provider: "p1", Model: "m1", Tier: 0},
					{Provider: "p2", Model: "m2", Tier: 2},
				},
			},
		},
	}

	rule := c.Rules[0]
	if err := c.UpdateRule("r1", rule); err != nil {
		t.Fatalf("UpdateRule error: %v", err)
	}

	got := []int{c.Rules[0].Services[0].Tier, c.Rules[0].Services[1].Tier}
	want := []int{0, 1}
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("expected tiers compacted to %v, got %v", want, got)
	}
}
