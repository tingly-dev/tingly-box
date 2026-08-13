package db

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/remote/peer"
)

func newTestPeerStore(t *testing.T) *PeerStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_fk=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&peerRecord{}, &peerUpdateRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return NewPeerStore(db)
}

func TestPeerStoreCRUD(t *testing.T) {
	store := newTestPeerStore(t)

	p := peer.Peer{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true, TokenHash: "hash1"}
	if err := store.Create(&p); err != nil {
		t.Fatal(err)
	}
	if p.UUID == "" {
		t.Fatal("Create did not assign UUID")
	}

	// Name uniqueness on create.
	dup := peer.Peer{Name: "report", BotUUID: "bot2", ChatID: "chat2"}
	if err := store.Create(&dup); !errors.Is(err, peer.ErrNameTaken) {
		t.Fatalf("dup create err = %v", err)
	}

	got, err := store.Get(p.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "report" || got.ChatID != "chat1" || !got.Enabled || got.TokenHash != "hash1" {
		t.Fatalf("Get = %+v", got)
	}

	if _, err := store.Get("missing"); !errors.Is(err, peer.ErrNotFound) {
		t.Fatalf("missing Get err = %v", err)
	}

	byToken, err := store.GetByToken("hash1")
	if err != nil || byToken.UUID != p.UUID {
		t.Fatalf("GetByToken = %+v, %v", byToken, err)
	}
	if _, err := store.GetByToken(""); !errors.Is(err, peer.ErrNotFound) {
		t.Fatalf("empty GetByToken err = %v", err)
	}

	// Second peer on another bot; ListByBot and HasEnabledForBot scope correctly.
	other := peer.Peer{Name: "ci", BotUUID: "bot2", ChatID: "chat2", Enabled: false}
	if err := store.Create(&other); err != nil {
		t.Fatal(err)
	}
	byBot, err := store.ListByBot("bot1")
	if err != nil || len(byBot) != 1 || byBot[0].Name != "report" {
		t.Fatalf("ListByBot = %+v, %v", byBot, err)
	}
	if !store.HasEnabledForBot("bot1") {
		t.Fatal("bot1 should have an enabled peer")
	}
	if store.HasEnabledForBot("bot2") {
		t.Fatal("bot2's only peer is disabled")
	}

	// Save: rename + disable; renaming onto a taken name fails.
	got.Name = "ci"
	if err := store.Save(&got); !errors.Is(err, peer.ErrNameTaken) {
		t.Fatalf("rename-collision err = %v", err)
	}
	got.Name = "report2"
	got.Enabled = false
	if err := store.Save(&got); err != nil {
		t.Fatal(err)
	}
	reread, _ := store.Get(p.UUID)
	if reread.Name != "report2" || reread.Enabled {
		t.Fatalf("post-save = %+v", reread)
	}
	if store.HasEnabledForBot("bot1") {
		t.Fatal("bot1 peer now disabled")
	}
	missing := peer.Peer{UUID: "missing", Name: "zzz"}
	if err := store.Save(&missing); !errors.Is(err, peer.ErrNotFound) {
		t.Fatalf("missing save err = %v", err)
	}

	// Delete removes the peer and its updates; deleting again is a no-op.
	u := peer.Update{PeerUUID: p.UUID, Text: "x"}
	if _, err := store.AppendUpdate(&u); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(p.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(p.UUID); !errors.Is(err, peer.ErrNotFound) {
		t.Fatalf("post-delete Get err = %v", err)
	}
	updates, _ := store.UpdatesAfter(p.UUID, 0, 10)
	if len(updates) != 0 {
		t.Fatalf("updates survived delete: %+v", updates)
	}
	if err := store.Delete(p.UUID); err != nil {
		t.Fatalf("re-delete err = %v", err)
	}
}

func TestPeerStoreUpdates(t *testing.T) {
	store := newTestPeerStore(t)
	p := peer.Peer{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true}
	if err := store.Create(&p); err != nil {
		t.Fatal(err)
	}

	var ids []int64
	for _, text := range []string{"a", "b", "c"} {
		u := peer.Update{PeerUUID: p.UUID, ChatID: "chat1", Type: peer.UpdateTypeMessage, Text: text}
		if _, err := store.AppendUpdate(&u); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}

	updates, err := store.UpdatesAfter(p.UUID, 0, 10)
	if err != nil || len(updates) != 3 || updates[0].Text != "a" {
		t.Fatalf("UpdatesAfter = %+v, %v", updates, err)
	}
	if updates[0].Type != peer.UpdateTypeMessage {
		t.Fatalf("type round-trip = %q", updates[0].Type)
	}
	// Cursor semantics: after the first id → two remain; limit truncates.
	updates, _ = store.UpdatesAfter(p.UUID, ids[0], 10)
	if len(updates) != 2 || updates[0].Text != "b" {
		t.Fatalf("after-first = %+v", updates)
	}
	updates, _ = store.UpdatesAfter(p.UUID, 0, 1)
	if len(updates) != 1 || updates[0].Text != "a" {
		t.Fatalf("limited = %+v", updates)
	}

	// Ack advances the cursor, prunes, and never goes backwards.
	if err := store.AckUpdates(p.UUID, ids[1]); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(p.UUID)
	if got.AckedUpdateID != ids[1] {
		t.Fatalf("cursor = %d, want %d", got.AckedUpdateID, ids[1])
	}
	updates, _ = store.UpdatesAfter(p.UUID, got.AckedUpdateID, 10)
	if len(updates) != 1 || updates[0].Text != "c" {
		t.Fatalf("post-ack = %+v", updates)
	}
	if err := store.AckUpdates(p.UUID, ids[0]); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(p.UUID)
	if got.AckedUpdateID != ids[1] {
		t.Fatalf("cursor moved backwards: %d", got.AckedUpdateID)
	}
	if err := store.AckUpdates("missing", 5); !errors.Is(err, peer.ErrNotFound) {
		t.Fatalf("missing ack err = %v", err)
	}
}

func TestPeerStoreUpdateCap(t *testing.T) {
	store := newTestPeerStore(t)
	p := peer.Peer{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true}
	if err := store.Create(&p); err != nil {
		t.Fatal(err)
	}
	totalDropped := 0
	for i := 0; i < peer.MaxQueuedUpdates+3; i++ {
		u := peer.Update{PeerUUID: p.UUID, Text: "x"}
		dropped, err := store.AppendUpdate(&u)
		if err != nil {
			t.Fatal(err)
		}
		totalDropped += dropped
	}
	if totalDropped != 3 {
		t.Fatalf("dropped = %d, want 3", totalDropped)
	}
	updates, _ := store.UpdatesAfter(p.UUID, 0, peer.MaxQueuedUpdates+10)
	if len(updates) != peer.MaxQueuedUpdates {
		t.Fatalf("kept %d, want %d", len(updates), peer.MaxQueuedUpdates)
	}
	if updates[0].ID != 4 {
		t.Fatalf("first surviving id = %d, want 4", updates[0].ID)
	}
}
