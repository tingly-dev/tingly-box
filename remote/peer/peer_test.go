package peer

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
	invalid := []string{"", "a", "Report", "has space", "@report", "cc", "tb", "mock", "peers",
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
		t.Fatal("non-peer token verified")
	}
	if VerifyToken(plaintext, "") {
		t.Fatal("empty stored hash verified")
	}
}

func TestCurrentAgentMarker(t *testing.T) {
	p := Peer{UUID: "abc", Name: "report"}
	if got := p.CurrentAgentValue(); got != "peer:abc" {
		t.Fatalf("CurrentAgentValue = %q", got)
	}
	if got := PeerUUIDFromCurrentAgent("peer:abc"); got != "abc" {
		t.Fatalf("extract = %q", got)
	}
	if got := PeerUUIDFromCurrentAgent("claude_code"); got != "" {
		t.Fatalf("non-marker extract = %q, want empty", got)
	}
}

func newTestPeer(t *testing.T, store Store, name, bot, chat string) Peer {
	t.Helper()
	p := Peer{Name: name, BotUUID: bot, ChatID: chat, Enabled: true}
	if err := store.Create(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMemStoreNameUniqueness(t *testing.T) {
	store := NewMemStore()
	newTestPeer(t, store, "report", "bot1", "chat1")
	dup := Peer{Name: "report", BotUUID: "bot2", ChatID: "chat2"}
	if err := store.Create(&dup); err != ErrNameTaken {
		t.Fatalf("duplicate create err = %v, want ErrNameTaken", err)
	}
}

func TestInboxOffsetConfirms(t *testing.T) {
	store := NewMemStore()
	p := newTestPeer(t, store, "report", "bot1", "chat1")
	in := NewInbox(store)

	for _, text := range []string{"one", "two"} {
		if err := in.Enqueue(p, Update{ChatID: p.ChatID, Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	// Offset 0: everything unconfirmed, in order, typed.
	updates, err := in.Poll(context.Background(), p.UUID, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates[0].Text != "one" || updates[1].Text != "two" {
		t.Fatalf("poll = %+v", updates)
	}
	if updates[0].Type != UpdateTypeMessage {
		t.Fatalf("update type = %q, want %q", updates[0].Type, UpdateTypeMessage)
	}

	// Crash replay: polling with offset 0 again re-reads the same batch.
	replay, err := in.Poll(context.Background(), p.UUID, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 2 {
		t.Fatalf("replay poll = %+v, want same 2 updates", replay)
	}

	// The Telegram idiom: next poll passes first id + 1 → confirms the
	// first, returns the second.
	updates, err = in.Poll(context.Background(), p.UUID, updates[0].ID+1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].Text != "two" {
		t.Fatalf("post-confirm poll = %+v", updates)
	}

	// Confirming past the last drains the stream.
	updates, err = in.Poll(context.Background(), p.UUID, updates[0].ID+1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 {
		t.Fatalf("drained poll = %+v", updates)
	}
	// Cursor never moves backwards: a stale offset does not resurface
	// confirmed updates.
	updates, err = in.Poll(context.Background(), p.UUID, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 {
		t.Fatalf("stale-offset poll = %+v, want empty", updates)
	}
}

func TestInboxLongPollWake(t *testing.T) {
	store := NewMemStore()
	p := newTestPeer(t, store, "report", "bot1", "chat1")
	in := NewInbox(store)

	type result struct {
		updates []Update
		err     error
	}
	done := make(chan result, 1)
	go func() {
		updates, err := in.Poll(context.Background(), p.UUID, 0, 5*time.Second, 10)
		done <- result{updates, err}
	}()

	// Wait until the poller is registered, then enqueue.
	deadline := time.Now().Add(2 * time.Second)
	for !in.HasWaiter(p.UUID) {
		if time.Now().After(deadline) {
			t.Fatal("poller never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := in.Enqueue(p, Update{ChatID: p.ChatID, Text: "ping"}); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if len(res.updates) != 1 || res.updates[0].Text != "ping" {
			t.Fatalf("woken poll = %+v", res.updates)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long-poll not woken by enqueue")
	}
}

func TestInboxOfflineNoticeOncePerEpisode(t *testing.T) {
	store := NewMemStore()
	p := newTestPeer(t, store, "report", "bot1", "chat1")
	in := NewInbox(store)

	var mu sync.Mutex
	notices := 0
	noticed := make(chan struct{}, 10)
	in.SetOfflineNotifier(func(Peer, Update) {
		mu.Lock()
		notices++
		mu.Unlock()
		noticed <- struct{}{}
	})

	// Two enqueues with nobody polling → exactly one notice.
	_ = in.Enqueue(p, Update{Text: "a"})
	_ = in.Enqueue(p, Update{Text: "b"})
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
	if _, err := in.Poll(context.Background(), p.UUID, 0, 0, 10); err != nil {
		t.Fatal(err)
	}
	_ = in.Enqueue(p, Update{Text: "c"})
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

func TestInboxCapDropsOldest(t *testing.T) {
	store := NewMemStore()
	p := newTestPeer(t, store, "report", "bot1", "chat1")
	in := NewInbox(store)

	for i := 0; i < MaxQueuedUpdates+5; i++ {
		if err := in.Enqueue(p, Update{Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	updates, err := store.UpdatesAfter(p.UUID, 0, MaxQueuedUpdates+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != MaxQueuedUpdates {
		t.Fatalf("kept %d updates, want %d", len(updates), MaxQueuedUpdates)
	}
	// Oldest were dropped: the first surviving id is 6.
	if updates[0].ID != 6 {
		t.Fatalf("first surviving id = %d, want 6", updates[0].ID)
	}
}

func TestRecentSends(t *testing.T) {
	r := NewRecentSends(2)
	r.Track("chat1", "m1", "peerA")
	r.Track("chat1", "m2", "peerB")
	if got := r.Lookup("chat1", "m1"); got != "peerA" {
		t.Fatalf("lookup m1 = %q", got)
	}
	// Third insert evicts the oldest.
	r.Track("chat1", "m3", "peerC")
	if got := r.Lookup("chat1", "m1"); got != "" {
		t.Fatalf("evicted lookup = %q, want empty", got)
	}
	if got := r.Lookup("chat1", "m3"); got != "peerC" {
		t.Fatalf("lookup m3 = %q", got)
	}
	// Empty message id never tracks or matches.
	r.Track("chat1", "", "peerD")
	if got := r.Lookup("chat1", ""); got != "" {
		t.Fatalf("empty-id lookup = %q", got)
	}
	// Different chat, same message id → no cross-chat match.
	if got := r.Lookup("chat2", "m3"); got != "" {
		t.Fatalf("cross-chat lookup = %q", got)
	}
}
