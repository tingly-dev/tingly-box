package logfields

import "testing"

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
