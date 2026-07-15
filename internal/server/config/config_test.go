package config

import (
	"fmt"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestCreateProfile_FillsFirstGap(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	scenario := typ.RuleScenario("claude_code")

	// Create p1, p2, p3
	p1, err := cfg.CreateProfile(scenario, "profile-1", true)
	if err != nil {
		t.Fatalf("CreateProfile p1 failed: %v", err)
	}
	if p1.ID != "p1" {
		t.Errorf("expected p1.ID = 'p1', got %q", p1.ID)
	}

	p2, err := cfg.CreateProfile(scenario, "profile-2", true)
	if err != nil {
		t.Fatalf("CreateProfile p2 failed: %v", err)
	}
	if p2.ID != "p2" {
		t.Errorf("expected p2.ID = 'p2', got %q", p2.ID)
	}

	p3, err := cfg.CreateProfile(scenario, "profile-3", true)
	if err != nil {
		t.Fatalf("CreateProfile p3 failed: %v", err)
	}
	if p3.ID != "p3" {
		t.Errorf("expected p3.ID = 'p3', got %q", p3.ID)
	}

	// Delete p2 — now IDs in use are [p1, p3]
	if err := cfg.DeleteProfile(scenario, "p2"); err != nil {
		t.Fatalf("DeleteProfile p2 failed: %v", err)
	}

	// Create a new profile — should reuse p2 (first gap), not p4
	pNew, err := cfg.CreateProfile(scenario, "profile-new", true)
	if err != nil {
		t.Fatalf("CreateProfile after delete failed: %v", err)
	}
	if pNew.ID != "p2" {
		t.Errorf("expected new profile ID = 'p2' (first gap), got %q", pNew.ID)
	}
}

func TestCreateProfile_SequentialWhenNoGaps(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	scenario := typ.RuleScenario("claude_code")

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("profile-%d", i)
		p, err := cfg.CreateProfile(scenario, name, true)
		if err != nil {
			t.Fatalf("CreateProfile %d failed: %v", i, err)
		}
		want := fmt.Sprintf("p%d", i)
		if p.ID != want {
			t.Errorf("expected ID %q, got %q", want, p.ID)
		}
	}
}

func TestCreateProfile_DuplicateNameError(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	scenario := typ.RuleScenario("claude_code")

	if _, err := cfg.CreateProfile(scenario, "same-name", true); err != nil {
		t.Fatalf("first CreateProfile failed: %v", err)
	}

	_, err = cfg.CreateProfile(scenario, "same-name", true)
	if err == nil {
		t.Fatal("expected error for duplicate profile name, got nil")
	}
}

func TestCreateProfile_FillsMultipleGaps(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	scenario := typ.RuleScenario("claude_code")

	// Create p1, p2, p3, p4, p5
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("profile-%d", i)
		if _, err := cfg.CreateProfile(scenario, name, true); err != nil {
			t.Fatalf("CreateProfile %d failed: %v", i, err)
		}
	}

	// Delete p2 and p4 — gaps at [p2, p4]
	if err := cfg.DeleteProfile(scenario, "p2"); err != nil {
		t.Fatalf("DeleteProfile p2 failed: %v", err)
	}
	if err := cfg.DeleteProfile(scenario, "p4"); err != nil {
		t.Fatalf("DeleteProfile p4 failed: %v", err)
	}

	// First new profile should fill p2 (smallest gap)
	pA, err := cfg.CreateProfile(scenario, "profile-a", true)
	if err != nil {
		t.Fatalf("CreateProfile a failed: %v", err)
	}
	if pA.ID != "p2" {
		t.Errorf("expected first gap ID 'p2', got %q", pA.ID)
	}

	// Second new profile should fill p4 (next smallest gap)
	pB, err := cfg.CreateProfile(scenario, "profile-b", true)
	if err != nil {
		t.Fatalf("CreateProfile b failed: %v", err)
	}
	if pB.ID != "p4" {
		t.Errorf("expected second gap ID 'p4', got %q", pB.ID)
	}

	// Third new profile should get p6 (no more gaps)
	pC, err := cfg.CreateProfile(scenario, "profile-c", true)
	if err != nil {
		t.Fatalf("CreateProfile c failed: %v", err)
	}
	if pC.ID != "p6" {
		t.Errorf("expected sequential ID 'p6', got %q", pC.ID)
	}
}

func TestCreateProfile_EmptyProfilesStartsAtP1(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	scenario := typ.RuleScenario("claude_code")

	p, err := cfg.CreateProfile(scenario, "first", true)
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	if p.ID != "p1" {
		t.Errorf("expected first profile ID = 'p1', got %q", p.ID)
	}
}

func TestGetServerHost_DefaultReturnsLocalhost(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	host := cfg.GetServerHost()
	if host != "localhost" {
		t.Errorf("expected default host 'localhost', got %q", host)
	}
}

func TestSetServerHost_StoresAndReturnsHost(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Set custom host
	err = cfg.SetServerHost("192.168.1.100")
	if err != nil {
		t.Fatalf("SetServerHost failed: %v", err)
	}

	host := cfg.GetServerHost()
	if host != "192.168.1.100" {
		t.Errorf("expected host '192.168.1.100', got %q", host)
	}
}

func TestSetServerHost_ZeroAll(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Set to 0.0.0.0 (bind to all interfaces)
	err = cfg.SetServerHost("0.0.0.0")
	if err != nil {
		t.Fatalf("SetServerHost failed: %v", err)
	}

	host := cfg.GetServerHost()
	if host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", host)
	}
}

func TestSetServerHost_EmptyStringReturnsLocalhost(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// Set to empty string (should still return localhost)
	err = cfg.SetServerHost("")
	if err != nil {
		t.Fatalf("SetServerHost failed: %v", err)
	}

	host := cfg.GetServerHost()
	if host != "localhost" {
		t.Errorf("expected empty string to return 'localhost', got %q", host)
	}
}

func TestResolveProfileAlias(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	scenario := typ.RuleScenario("claude_code")

	p1, err := cfg.CreateProfile(scenario, "mine", true)
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	// Simulate a legacy profile whose name predates the creation-time
	// constraint (CreateProfile now rejects names with spaces). The routing
	// path must still refuse to resolve it by name.
	cfg.Profiles[string(scenario)] = append(cfg.Profiles[string(scenario)], typ.ProfileMeta{
		ID:   "p99",
		Name: "my work profile",
	})

	// Resolve by canonical ID.
	if id, ok := cfg.ResolveProfileAlias(scenario, p1.ID); !ok || id != p1.ID {
		t.Errorf("ResolveProfileAlias(%q) = (%q, %v), want (%q, true)", p1.ID, id, ok, p1.ID)
	}

	// Resolve by simple name.
	if id, ok := cfg.ResolveProfileAlias(scenario, "mine"); !ok || id != p1.ID {
		t.Errorf("ResolveProfileAlias(\"mine\") = (%q, %v), want (%q, true)", id, ok, p1.ID)
	}

	// Non-simple name is not routable by name.
	if id, ok := cfg.ResolveProfileAlias(scenario, "my work profile"); ok {
		t.Errorf("ResolveProfileAlias(\"my work profile\") = (%q, true), want not ok", id)
	}

	// Unknown alias.
	if _, ok := cfg.ResolveProfileAlias(scenario, "nope"); ok {
		t.Errorf("ResolveProfileAlias(\"nope\") = ok, want not ok")
	}

	// Empty alias.
	if _, ok := cfg.ResolveProfileAlias(scenario, ""); ok {
		t.Errorf("ResolveProfileAlias(\"\") = ok, want not ok")
	}
}

func TestCreateProfile_RejectsNonURLFriendlyName(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	scenario := typ.RuleScenario("claude_code")

	for _, bad := range []string{"my profile", "a:b", "a/b", "", " mine", "default", "DEFAULT", "p1", "p07"} {
		if _, err := cfg.CreateProfile(scenario, bad, true); err == nil {
			t.Errorf("CreateProfile(%q) = nil error, want rejection", bad)
		}
	}

	// A valid name still works; renaming it to a new bad name is rejected.
	p, err := cfg.CreateProfile(scenario, "mine", true)
	if err != nil {
		t.Fatalf("CreateProfile(\"mine\") failed: %v", err)
	}
	if err := cfg.UpdateProfile(scenario, p.ID, "not ok", nil); err == nil {
		t.Errorf("UpdateProfile to \"not ok\" = nil error, want rejection")
	}
}

func TestUpdateProfile_AllowsEditingLegacyBadName(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	scenario := typ.RuleScenario("claude_code")

	// Simulate a legacy profile whose name predates the creation-time
	// constraint (created directly, bypassing CreateProfile validation).
	cfg.Profiles = map[string][]typ.ProfileMeta{
		string(scenario): {{ID: "p1", Name: "my work profile"}},
	}

	// Editing the legacy profile while preserving its (bad) name must succeed —
	// the handler replays the existing name when the caller doesn't rename.
	if err := cfg.UpdateProfile(scenario, "p1", "my work profile", nil); err != nil {
		t.Errorf("UpdateProfile preserving legacy name = %v, want nil", err)
	}

	// Cleaning it up to a valid name is allowed.
	if err := cfg.UpdateProfile(scenario, "p1", "work", nil); err != nil {
		t.Errorf("UpdateProfile to valid name = %v, want nil", err)
	}

	// But renaming to a different still-bad name is rejected.
	if err := cfg.UpdateProfile(scenario, "p1", "still bad", nil); err == nil {
		t.Errorf("UpdateProfile to \"still bad\" = nil error, want rejection")
	}
}
