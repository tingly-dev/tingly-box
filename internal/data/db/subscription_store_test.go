package db

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/remote/subscription"
)

func newTestSubscriptionStore(t *testing.T) *SubscriptionStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_fk=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&subscriptionRecord{}, &subscriptionEventRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return NewSubscriptionStore(db)
}

func TestSubscriptionStoreCRUD(t *testing.T) {
	store := newTestSubscriptionStore(t)

	sub := subscription.Subscription{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true, TokenHash: "hash1"}
	if err := store.Create(&sub); err != nil {
		t.Fatal(err)
	}
	if sub.UUID == "" {
		t.Fatal("Create did not assign UUID")
	}

	// Name uniqueness on create.
	dup := subscription.Subscription{Name: "report", BotUUID: "bot2", ChatID: "chat2"}
	if err := store.Create(&dup); !errors.Is(err, subscription.ErrNameTaken) {
		t.Fatalf("dup create err = %v", err)
	}

	got, err := store.Get(sub.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "report" || got.ChatID != "chat1" || !got.Enabled || got.TokenHash != "hash1" {
		t.Fatalf("Get = %+v", got)
	}

	if _, err := store.Get("missing"); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("missing Get err = %v", err)
	}

	byToken, err := store.GetByToken("hash1")
	if err != nil || byToken.UUID != sub.UUID {
		t.Fatalf("GetByToken = %+v, %v", byToken, err)
	}
	if _, err := store.GetByToken(""); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("empty GetByToken err = %v", err)
	}

	// Second sub on another bot; ListByBot and HasEnabledForBot scope correctly.
	other := subscription.Subscription{Name: "ci", BotUUID: "bot2", ChatID: "chat2", Enabled: false}
	if err := store.Create(&other); err != nil {
		t.Fatal(err)
	}
	byBot, err := store.ListByBot("bot1")
	if err != nil || len(byBot) != 1 || byBot[0].Name != "report" {
		t.Fatalf("ListByBot = %+v, %v", byBot, err)
	}
	if !store.HasEnabledForBot("bot1") {
		t.Fatal("bot1 should have an enabled subscription")
	}
	if store.HasEnabledForBot("bot2") {
		t.Fatal("bot2's only subscription is disabled")
	}

	// Update: rename + disable; renaming onto a taken name fails.
	got.Name = "ci"
	if err := store.Update(&got); !errors.Is(err, subscription.ErrNameTaken) {
		t.Fatalf("rename-collision err = %v", err)
	}
	got.Name = "report2"
	got.Enabled = false
	if err := store.Update(&got); err != nil {
		t.Fatal(err)
	}
	reread, _ := store.Get(sub.UUID)
	if reread.Name != "report2" || reread.Enabled {
		t.Fatalf("post-update = %+v", reread)
	}
	if store.HasEnabledForBot("bot1") {
		t.Fatal("bot1 subscription now disabled")
	}
	missing := subscription.Subscription{UUID: "missing", Name: "zzz"}
	if err := store.Update(&missing); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("missing update err = %v", err)
	}

	// Delete removes the sub and its events; deleting again is a no-op.
	ev := subscription.Event{SubscriptionUUID: sub.UUID, Text: "x"}
	if _, err := store.AppendEvent(&ev); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(sub.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(sub.UUID); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("post-delete Get err = %v", err)
	}
	events, _ := store.EventsAfter(sub.UUID, 0, 10)
	if len(events) != 0 {
		t.Fatalf("events survived delete: %+v", events)
	}
	if err := store.Delete(sub.UUID); err != nil {
		t.Fatalf("re-delete err = %v", err)
	}
}

func TestSubscriptionStoreEvents(t *testing.T) {
	store := newTestSubscriptionStore(t)
	sub := subscription.Subscription{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true}
	if err := store.Create(&sub); err != nil {
		t.Fatal(err)
	}

	var ids []int64
	for _, text := range []string{"a", "b", "c"} {
		ev := subscription.Event{SubscriptionUUID: sub.UUID, ChatID: "chat1", Text: text}
		if _, err := store.AppendEvent(&ev); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ev.ID)
	}

	events, err := store.EventsAfter(sub.UUID, 0, 10)
	if err != nil || len(events) != 3 || events[0].Text != "a" {
		t.Fatalf("EventsAfter = %+v, %v", events, err)
	}
	// Cursor semantics: after the first id → two remain; limit truncates.
	events, _ = store.EventsAfter(sub.UUID, ids[0], 10)
	if len(events) != 2 || events[0].Text != "b" {
		t.Fatalf("after-first = %+v", events)
	}
	events, _ = store.EventsAfter(sub.UUID, 0, 1)
	if len(events) != 1 || events[0].Text != "a" {
		t.Fatalf("limited = %+v", events)
	}

	// Ack advances the cursor, prunes, and never goes backwards.
	if err := store.AckEvents(sub.UUID, ids[1]); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(sub.UUID)
	if got.AckedEventID != ids[1] {
		t.Fatalf("cursor = %d, want %d", got.AckedEventID, ids[1])
	}
	events, _ = store.EventsAfter(sub.UUID, got.AckedEventID, 10)
	if len(events) != 1 || events[0].Text != "c" {
		t.Fatalf("post-ack = %+v", events)
	}
	if err := store.AckEvents(sub.UUID, ids[0]); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(sub.UUID)
	if got.AckedEventID != ids[1] {
		t.Fatalf("cursor moved backwards: %d", got.AckedEventID)
	}
	if err := store.AckEvents("missing", 5); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("missing ack err = %v", err)
	}
}

func TestSubscriptionStoreEventCap(t *testing.T) {
	store := newTestSubscriptionStore(t)
	sub := subscription.Subscription{Name: "report", BotUUID: "bot1", ChatID: "chat1", Enabled: true}
	if err := store.Create(&sub); err != nil {
		t.Fatal(err)
	}
	totalDropped := 0
	for i := 0; i < subscription.MaxQueuedEvents+3; i++ {
		ev := subscription.Event{SubscriptionUUID: sub.UUID, Text: "x"}
		dropped, err := store.AppendEvent(&ev)
		if err != nil {
			t.Fatal(err)
		}
		totalDropped += dropped
	}
	if totalDropped != 3 {
		t.Fatalf("dropped = %d, want 3", totalDropped)
	}
	events, _ := store.EventsAfter(sub.UUID, 0, subscription.MaxQueuedEvents+10)
	if len(events) != subscription.MaxQueuedEvents {
		t.Fatalf("kept %d, want %d", len(events), subscription.MaxQueuedEvents)
	}
	if events[0].ID != 4 {
		t.Fatalf("first surviving id = %d, want 4", events[0].ID)
	}
}
