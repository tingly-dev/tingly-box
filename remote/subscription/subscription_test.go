package subscription

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	valid := []string{"report", "ci-gate", "on_call2", "ab"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "a", "Report", "has space", "@report", "cc", "tb", "mock", "subs",
		strings.Repeat("x", 33)}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestTokenRoundTrip(t *testing.T) {
	plaintext, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Fatalf("token %q missing prefix", plaintext)
	}
	if !VerifyToken(plaintext, hash) {
		t.Fatal("freshly minted token failed verification")
	}
	if VerifyToken(plaintext+"x", hash) {
		t.Fatal("tampered token verified")
	}
	if VerifyToken("tb-user-something", hash) {
		t.Fatal("non-sub token verified")
	}
	if VerifyToken(plaintext, "") {
		t.Fatal("empty stored hash verified")
	}
}

func TestCurrentAgentMarker(t *testing.T) {
	sub := Subscription{UUID: "abc", Name: "report"}
	if got := sub.CurrentAgentValue(); got != "sub:abc" {
		t.Fatalf("CurrentAgentValue = %q", got)
	}
	if got := SubscriptionUUIDFromCurrentAgent("sub:abc"); got != "abc" {
		t.Fatalf("extract = %q", got)
	}
	if got := SubscriptionUUIDFromCurrentAgent("claude_code"); got != "" {
		t.Fatalf("non-marker extract = %q, want empty", got)
	}
}

func newTestSub(t *testing.T, store Store, name, bot, chat string) Subscription {
	t.Helper()
	sub := Subscription{Name: name, BotUUID: bot, ChatID: chat, Enabled: true}
	if err := store.Create(&sub); err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestMemStoreNameUniqueness(t *testing.T) {
	store := NewMemStore()
	newTestSub(t, store, "report", "bot1", "chat1")
	dup := Subscription{Name: "report", BotUUID: "bot2", ChatID: "chat2"}
	if err := store.Create(&dup); err != ErrNameTaken {
		t.Fatalf("duplicate create err = %v, want ErrNameTaken", err)
	}
}

func TestMailboxEnqueuePollAck(t *testing.T) {
	store := NewMemStore()
	sub := newTestSub(t, store, "report", "bot1", "chat1")
	mb := NewMailbox(store)

	for _, text := range []string{"one", "two"} {
		if err := mb.Enqueue(sub, Event{ChatID: sub.ChatID, Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	events, err := mb.Poll(context.Background(), sub.UUID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Text != "one" || events[1].Text != "two" {
		t.Fatalf("poll = %+v", events)
	}

	// Ack the first only: re-poll returns the second.
	if err := mb.Ack(sub.UUID, events[0].ID); err != nil {
		t.Fatal(err)
	}
	events, err = mb.Poll(context.Background(), sub.UUID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Text != "two" {
		t.Fatalf("post-ack poll = %+v", events)
	}

	// Ack everything: empty poll with zero timeout.
	if err := mb.Ack(sub.UUID, events[0].ID); err != nil {
		t.Fatal(err)
	}
	events, err = mb.Poll(context.Background(), sub.UUID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("drained poll = %+v", events)
	}
}

func TestMailboxLongPollWake(t *testing.T) {
	store := NewMemStore()
	sub := newTestSub(t, store, "report", "bot1", "chat1")
	mb := NewMailbox(store)

	type result struct {
		events []Event
		err    error
	}
	done := make(chan result, 1)
	go func() {
		events, err := mb.Poll(context.Background(), sub.UUID, 5*time.Second, 10)
		done <- result{events, err}
	}()

	// Wait until the poller is registered, then enqueue.
	deadline := time.Now().Add(2 * time.Second)
	for !mb.HasWaiter(sub.UUID) {
		if time.Now().After(deadline) {
			t.Fatal("poller never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := mb.Enqueue(sub, Event{ChatID: sub.ChatID, Text: "ping"}); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if len(res.events) != 1 || res.events[0].Text != "ping" {
			t.Fatalf("woken poll = %+v", res.events)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long-poll not woken by enqueue")
	}
}

func TestMailboxOfflineNoticeOncePerEpisode(t *testing.T) {
	store := NewMemStore()
	sub := newTestSub(t, store, "report", "bot1", "chat1")
	mb := NewMailbox(store)

	var mu sync.Mutex
	notices := 0
	noticed := make(chan struct{}, 10)
	mb.SetOfflineNotifier(func(Subscription, Event) {
		mu.Lock()
		notices++
		mu.Unlock()
		noticed <- struct{}{}
	})

	// Two enqueues with nobody polling → exactly one notice.
	_ = mb.Enqueue(sub, Event{Text: "a"})
	_ = mb.Enqueue(sub, Event{Text: "b"})
	select {
	case <-noticed:
	case <-time.After(2 * time.Second):
		t.Fatal("no offline notice")
	}
	mu.Lock()
	if notices != 1 {
		mu.Unlock()
		t.Fatalf("notices = %d, want 1", notices)
	}
	mu.Unlock()

	// A poller connecting resets the episode; next offline enqueue notices
	// again.
	if _, err := mb.Poll(context.Background(), sub.UUID, 0, 10); err != nil {
		t.Fatal(err)
	}
	_ = mb.Enqueue(sub, Event{Text: "c"})
	select {
	case <-noticed:
	case <-time.After(2 * time.Second):
		t.Fatal("no notice after episode reset")
	}
	mu.Lock()
	if notices != 2 {
		mu.Unlock()
		t.Fatalf("notices = %d, want 2", notices)
	}
	mu.Unlock()
}

func TestMailboxCapDropsOldest(t *testing.T) {
	store := NewMemStore()
	sub := newTestSub(t, store, "report", "bot1", "chat1")
	mb := NewMailbox(store)

	for i := 0; i < MaxQueuedEvents+5; i++ {
		if err := mb.Enqueue(sub, Event{Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.EventsAfter(sub.UUID, 0, MaxQueuedEvents+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != MaxQueuedEvents {
		t.Fatalf("kept %d events, want %d", len(events), MaxQueuedEvents)
	}
	// Oldest were dropped: the first surviving id is 6.
	if events[0].ID != 6 {
		t.Fatalf("first surviving id = %d, want 6", events[0].ID)
	}
}

func TestRecentSends(t *testing.T) {
	r := NewRecentSends(2)
	r.Track("chat1", "m1", "subA")
	r.Track("chat1", "m2", "subB")
	if got := r.Lookup("chat1", "m1"); got != "subA" {
		t.Fatalf("lookup m1 = %q", got)
	}
	// Third insert evicts the oldest.
	r.Track("chat1", "m3", "subC")
	if got := r.Lookup("chat1", "m1"); got != "" {
		t.Fatalf("evicted lookup = %q, want empty", got)
	}
	if got := r.Lookup("chat1", "m3"); got != "subC" {
		t.Fatalf("lookup m3 = %q", got)
	}
	// Empty message id never tracks or matches.
	r.Track("chat1", "", "subD")
	if got := r.Lookup("chat1", ""); got != "" {
		t.Fatalf("empty-id lookup = %q", got)
	}
	// Different chat, same message id → no cross-chat match.
	if got := r.Lookup("chat2", "m3"); got != "" {
		t.Fatalf("cross-chat lookup = %q", got)
	}
}
