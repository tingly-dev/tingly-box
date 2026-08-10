package db

import (
	"os"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/remote/session"
)

func newSessionStore(t *testing.T) *RemoteSessionStore {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm.RemoteSessions()
}

// TestSessionSetPersistsImmediately carries forward the durability contract
// the JSON store had to be patched into honoring: an update is on disk when
// Set returns, not at shutdown. Here it holds by construction — the write is
// a committed transaction.
func TestSessionSetPersistsImmediately(t *testing.T) {
	store := newSessionStore(t)

	sess := &session.Session{
		ID:        "sess-1",
		ChatID:    "chat-1",
		Agent:     "claude",
		Project:   "/tmp/proj",
		Status:    session.StatusPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set: %v", err)
	}

	sess.Status = session.StatusCompleted
	sess.Response = "done"
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set after update: %v", err)
	}
	if err := store.AppendMessage(sess.ID, session.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("session not stored")
	}
	if got.Status != session.StatusCompleted || got.Response != "done" {
		t.Errorf("stored = {%s, %q}, want {%s, %q}",
			got.Status, got.Response, session.StatusCompleted, "done")
	}
	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Errorf("messages = %+v, want one 'hi'", msgs)
	}
}

// TestSessionMessagesAppendInOrder checks the transcript preserves order
// across many appends.
func TestSessionMessagesAppendInOrder(t *testing.T) {
	store := newSessionStore(t)

	sess := &session.Session{ID: "sess-1", Status: session.StatusRunning}
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := store.AppendMessage(sess.ID, session.Message{
			Role:      "user",
			Content:   string(rune('a' + i)),
			Timestamp: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("messages = %d, want 5", len(msgs))
	}
	for i, m := range msgs {
		if want := string(rune('a' + i)); m.Content != want {
			t.Errorf("message %d = %q, want %q (transcript order is wrong)", i, m.Content, want)
		}
	}
}

// TestSessionIndexReadsDoNotLoadTranscripts is the point of the split: warming
// the manager at boot, or listing a chat's sessions, must not drag every
// conversation's text along. The index record carries no history at all — the
// transcript is fetched only when Messages is called.
func TestSessionIndexReadsDoNotLoadTranscripts(t *testing.T) {
	store := newSessionStore(t)

	sess := &session.Session{ID: "sess-1", ChatID: "chat-1", Status: session.StatusRunning}
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.AppendMessage(sess.ID, session.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	// The transcript is reachable on demand...
	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}

	// ...but the index alone is enough to answer every listing query, and a
	// store with no transcript configured still serves them.
	indexOnly := &RemoteSessionStore{db: store.db}
	if got, err := indexOnly.Get(sess.ID); err != nil || got == nil {
		t.Fatalf("index Get: got=%v err=%v", got, err)
	}
	if got := indexOnly.List(); len(got) != 1 {
		t.Errorf("index List = %d, want 1", len(got))
	}
	if got, err := indexOnly.ListByChat("chat-1"); err != nil || len(got) != 1 {
		t.Fatalf("index ListByChat: got=%d err=%v", len(got), err)
	}
	if got, err := indexOnly.Messages(sess.ID); err != nil || len(got) != 0 {
		t.Errorf("index-only Messages = %v (err %v), want empty", got, err)
	}
}

// TestSessionDeleteRemovesMessages checks a deleted session leaves neither an
// index row nor a transcript file behind.
func TestSessionDeleteRemovesMessages(t *testing.T) {
	store := newSessionStore(t)

	sess := &session.Session{ID: "sess-1", Status: session.StatusCompleted}
	if err := store.Set(sess.ID, sess); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.AppendMessage(sess.ID, session.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	transcriptPath := store.transcript.Path(sess.ID)

	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Error("session still present after delete")
	}

	if _, err := os.Stat(transcriptPath); !os.IsNotExist(err) {
		t.Errorf("transcript file survived the delete: %v", err)
	}
}

// TestFindByChatAgentProject checks the binding lookup: newest active session
// wins, and terminal ones are invisible.
func TestFindByChatAgentProject(t *testing.T) {
	store := newSessionStore(t)

	base := time.Now().UTC().Add(-time.Hour)
	mk := func(id string, status session.Status, activity time.Time) {
		sess := &session.Session{
			ID:      id,
			ChatID:  "chat-1",
			Agent:   "claude",
			Project: "/proj",
			Status:  status,
		}
		if err := store.Set(id, sess); err != nil {
			t.Fatalf("set %s: %v", id, err)
		}
		// Set stamps LastActivity; rewrite it so the ordering is deterministic.
		if err := store.db.Model(&RemoteSessionRecord{}).
			Where("id = ?", id).Update("last_activity", activity).Error; err != nil {
			t.Fatalf("stamp %s: %v", id, err)
		}
	}

	mk("old", session.StatusCompleted, base)
	mk("new", session.StatusRunning, base.Add(30*time.Minute))
	mk("closed", session.StatusClosed, base.Add(59*time.Minute))

	got, err := store.FindByChatAgentProject("chat-1", "claude", "/proj")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("no session found")
	}
	if got.ID != "new" {
		t.Errorf("found %q, want %q (closed sessions must be skipped)", got.ID, "new")
	}

	missing, err := store.FindByChatAgentProject("chat-1", "claude", "/other")
	if err != nil {
		t.Fatalf("find other: %v", err)
	}
	if missing != nil {
		t.Errorf("found %q for an unbound project", missing.ID)
	}
}

// TestListByChatSeparatesTranscripts checks each session keeps its own
// transcript rather than sharing or mixing them.
func TestListByChatSeparatesTranscripts(t *testing.T) {
	store := newSessionStore(t)

	for _, spec := range []struct{ id, msg string }{{"a", "alpha"}, {"b", "beta"}} {
		sess := &session.Session{ID: spec.id, ChatID: "chat-1", Status: session.StatusRunning}
		if err := store.Set(spec.id, sess); err != nil {
			t.Fatalf("set %s: %v", spec.id, err)
		}
		if err := store.AppendMessage(spec.id, session.Message{Role: "user", Content: spec.msg}); err != nil {
			t.Fatalf("append %s: %v", spec.id, err)
		}
	}

	got, err := store.ListByChat("chat-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("sessions = %d, want 2", len(got))
	}

	// One transcript per session — nothing shared, nothing mixed.
	for _, spec := range []struct{ id, msg string }{{"a", "alpha"}, {"b", "beta"}} {
		msgs, err := store.Messages(spec.id)
		if err != nil {
			t.Fatalf("messages %s: %v", spec.id, err)
		}
		if len(msgs) != 1 || msgs[0].Content != spec.msg {
			t.Errorf("session %s transcript = %+v, want one %q", spec.id, msgs, spec.msg)
		}
	}
}

// TestImportPreservesLastActivity pins why Import exists: migration must not
// make dormant conversations look freshly active, which would reorder the
// binding lookup and defeat retention.
func TestImportPreservesLastActivity(t *testing.T) {
	store := newSessionStore(t)

	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	sess := &session.Session{
		ID:           "sess-1",
		ChatID:       "chat-1",
		Status:       session.StatusCompleted,
		CreatedAt:    old,
		LastActivity: old,
	}
	if err := store.Import(sess); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, err := store.Get("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.LastActivity.Truncate(time.Second).Equal(old) {
		t.Errorf("LastActivity = %s, want %s", got.LastActivity, old)
	}
}
