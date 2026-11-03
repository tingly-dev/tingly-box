package logfields

import "testing"

func TestBotOmitsEmptyParts(t *testing.T) {
	f := Bot("u1", "", "telegram")
	if f[BotUUID] != "u1" || f[Platform] != "telegram" {
		t.Fatalf("got %v", f)
	}
	if _, ok := f[BotName]; ok {
		t.Fatalf("an empty name must not be logged: %v", f)
	}
	if f = Bot("u1", "Ops", ""); f[BotName] != "Ops" {
		t.Fatalf("got %v", f)
	}
}

func TestChatCarriesExternalIDAndAlias(t *testing.T) {
	f := Chat("u1", "-100", "Release Crew")
	if f[ChatID] != "-100" || f[ChatAlias] != "Release Crew" || f[BotUUID] != "u1" {
		t.Fatalf("got %v", f)
	}
	if _, ok := Chat("u1", "-100", "")[ChatAlias]; ok {
		t.Fatal("no alias, no field")
	}
}

func TestKeysDoNotCollide(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range []string{BotUUID, BotName, Platform, ChatID, ChatAlias, ActorID, AccessTargetID, AccessActorID, IlinkBotID} {
		if seen[k] {
			t.Fatalf("duplicate key %q", k)
		}
		seen[k] = true
	}
}
