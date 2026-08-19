package db

import "testing"

func TestTeamStore_EnsuresDefaultAndCRUD(t *testing.T) {
	store, err := NewTeamStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	defaultTeam, err := store.Get(DefaultTeamID)
	if err != nil {
		t.Fatalf("get default team: %v", err)
	}
	if defaultTeam.Slug != DefaultTeamSlug || !defaultTeam.Enabled {
		t.Fatalf("unexpected default team: %#v", defaultTeam)
	}

	created, err := store.Create("Engineering", "engineering")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if created.ID == "" || created.ID == DefaultTeamID {
		t.Fatalf("unexpected created ID: %q", created.ID)
	}

	updated, err := store.Update(created.ID, "Platform", "platform")
	if err != nil {
		t.Fatalf("update team: %v", err)
	}
	if updated.Name != "Platform" || updated.Slug != "platform" {
		t.Fatalf("unexpected update: %#v", updated)
	}

	if err := store.SetEnabled(created.ID, false); err != nil {
		t.Fatalf("disable team: %v", err)
	}
	disabled, _ := store.Get(created.ID)
	if disabled.Enabled {
		t.Fatal("team remained enabled")
	}
	if err := store.Delete(created.ID); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if _, err := store.Get(created.ID); err == nil {
		t.Fatal("deleted team still exists")
	}
	if err := store.Delete(DefaultTeamID); err == nil {
		t.Fatal("default team deletion should be rejected")
	}
}

func TestTeamStore_RejectsInvalidAndDuplicateSlugs(t *testing.T) {
	store, err := NewTeamStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, slug := range []string{"", "Upper", "has space", "has/slash"} {
		if _, err := store.Create("Team", slug); err == nil {
			t.Errorf("Create with slug %q succeeded", slug)
		}
	}
	if _, err := store.Create("One", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("Another", "one"); err == nil {
		t.Fatal("duplicate slug should be rejected")
	}
}

func TestAPITokenStore_DefaultBindingMoveAndTeamDisable(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewStoreManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	teams := manager.Team()
	tokens := manager.APIToken()
	record, err := tokens.CreateTokenWithTokenID("user", "tb-share-default", "default", "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if record.TeamID != DefaultTeamID {
		t.Fatalf("default team ID = %q", record.TeamID)
	}

	other, err := teams.Create("Other", "other")
	if err != nil {
		t.Fatal(err)
	}
	if err := tokens.MoveTokenToTeam(record.TokenID, other.ID); err != nil {
		t.Fatalf("move token: %v", err)
	}
	moved, err := tokens.ValidateToken(record.TokenID)
	if err != nil {
		t.Fatalf("validate moved token: %v", err)
	}
	if moved.TeamID != other.ID {
		t.Fatalf("moved team ID = %q, want %q", moved.TeamID, other.ID)
	}
	if err := teams.Delete(other.ID); err == nil {
		t.Fatal("team with sharing keys should not be deleted")
	}
	if err := teams.SetEnabled(other.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.ValidateToken(record.TokenID); err == nil {
		t.Fatal("token for disabled team should be rejected")
	}
}

func TestAPITokenStore_BackfillsEmptyTeamID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAPITokenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTokenWithTokenID("user", "tb-share-legacy", "legacy", "admin", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&APITokenRecord{}).Where("token_id = ?", "tb-share-legacy").Update("team_id", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewAPITokenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record, err := reopened.ValidateToken("tb-share-legacy")
	if err != nil {
		t.Fatalf("validate backfilled token: %v", err)
	}
	if record.TeamID != DefaultTeamID {
		t.Fatalf("backfilled team ID = %q", record.TeamID)
	}
}
