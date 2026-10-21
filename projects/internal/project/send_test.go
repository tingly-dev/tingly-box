package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// userEvents are the user events of a thread, in order.
func userEvents(t *testing.T, store *db.Store, threadID string) []db.ThreadEvent {
	t.Helper()
	all, err := store.ListThreadEvents(threadID)
	require.NoError(t, err)
	var out []db.ThreadEvent
	for _, e := range all {
		if e.Kind == db.EvUser {
			out = append(out, e)
		}
	}
	return out
}

func startWorker(t *testing.T, f *testFixture) *db.Thread {
	t.Helper()
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Worker", Task: "first", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	return th
}

func TestSendRecordsWhereAMessageCameFrom(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	svc := &Service{Store: f.store, Threads: f.threads}
	th := startWorker(t, f)

	for _, src := range []Source{
		{},
		{Kind: db.SourceIM, Platform: "telegram", ChatID: "42"},
		{Kind: db.SourceThread, Ref: f.project.MainThreadID},
	} {
		_, err := svc.Send(context.Background(), th.ID, "msg", nil, src)
		require.NoError(t, err)
		waitStatus(t, f.store, th.ID, db.ThreadIdle)
	}

	evs := userEvents(t, f.store, th.ID)
	require.Len(t, evs, 4, "the task and three messages")
	assert.Equal(t, db.SourceThread, evs[0].Source, "the coordinator's task came from the main thread")
	assert.Equal(t, f.project.MainThreadID, evs[0].SourceRef)
	assert.Empty(t, evs[1].Source, "the web is the empty source")
	assert.Equal(t, db.SourceIM, evs[2].Source)
	assert.Equal(t, "telegram", evs[2].SourcePlatform)
	assert.Equal(t, "42", evs[2].SourceChatID)
	assert.Equal(t, db.SourceThread, evs[3].Source)
	assert.Equal(t, f.project.MainThreadID, evs[3].SourceRef)
	assert.Empty(t, evs[3].SourcePlatform)
}

func TestSendChecksTheProjectAndTheThread(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	svc := &Service{Store: f.store, Threads: f.threads, Hub: f.threads.hub}
	th := startWorker(t, f)

	_, err := svc.Send(context.Background(), "nope", "x", nil, Source{})
	assert.ErrorIs(t, err, db.ErrNotFound)

	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetPaused(p, true))
	_, err = svc.Send(context.Background(), th.ID, "x", nil, Source{})
	assert.ErrorIs(t, err, ErrPaused)
	require.NoError(t, svc.SetPaused(p, false))

	require.NoError(t, svc.SetArchived(p, true))
	_, err = svc.Send(context.Background(), th.ID, "x", nil, Source{})
	assert.ErrorIs(t, err, ErrArchived)
	require.NoError(t, svc.SetArchived(p, false))

	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)
	_, err = svc.Send(context.Background(), th.ID, "x", nil, Source{})
	assert.ErrorIs(t, err, ErrResolved)
}

func TestSendToAMainThreadGoesToTheCoordinator(t *testing.T) {
	f, svc, _ := channelFixture(t)

	got, err := svc.Send(context.Background(), f.project.MainThreadID, "ping", nil, Source{Kind: db.SourceIM, Platform: "telegram", ChatID: "7"})
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, got.ID)
	evs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return len(m) == 2 })
	assert.Equal(t, db.EvUser, evs[0].Kind)
	assert.Equal(t, "ping", evs[0].Content)
	assert.Equal(t, db.SourceIM, evs[0].Source)
	assert.Equal(t, "7", evs[0].SourceChatID)
	assert.Equal(t, db.EvAssistant, evs[1].Kind)
	assert.Equal(t, "Echo: ping", evs[1].Content)

	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetArchived(p, true))
	_, err = svc.Send(context.Background(), f.project.MainThreadID, "late", nil, Source{})
	assert.ErrorIs(t, err, ErrArchived)
}

func TestChatBoundToAThreadTalksToThatThread(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	// The turn's report reaches the coordinator, which the main thread's
	// chats would hear; this chat is on the worker.
	b, err := svc.BindChannel("telegram", "w", "", th.ID, "")
	require.NoError(t, err)
	assert.Equal(t, th.ID, b.ThreadID)
	assert.Equal(t, f.project.ID, b.ProjectID)

	handled, err := svc.PostFromChannel("telegram", "w", "from the chat", nil)
	require.NoError(t, err)
	require.True(t, handled)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	assert.Equal(t, "[#1 Worker] ok", sink.next(t).text, "the worker answers the chat bound to it, named by its number")
	evs := userEvents(t, f.store, th.ID)
	last := evs[len(evs)-1]
	assert.Equal(t, "from the chat", last.Content)
	assert.Equal(t, db.SourceIM, last.Source)
	assert.Equal(t, "telegram", last.SourcePlatform)
	assert.Equal(t, "w", last.SourceChatID)
	sink.none(t)
}

func TestNeedsYouReachesChatsOfTheThreadAndItsAncestors(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", th.ID, "")
	require.NoError(t, err)

	svc.Relay.NeedsUser(*th, approvalEvent("r1"))
	first, second := sink.next(t), sink.next(t)
	assert.ElementsMatch(t, []string{"up", "own"}, []string{first.chatID, second.chatID})
	sink.none(t)
}

func TestBindingAFolderNeedsAThread(t *testing.T) {
	f, svc, _ := channelFixture(t)
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go"})
	require.NoError(t, err)

	_, err = svc.BindChannel("telegram", "1", "", "", s.ProjectID)
	assert.ErrorIs(t, err, ErrBindFolder, "a folder has no main thread to stand for it")
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	b, err := svc.BindChannel("telegram", "1", "", s.ID, "")
	require.NoError(t, err)
	assert.Equal(t, s.ProjectID, b.ProjectID)

	_, err = svc.BindChannel("telegram", "1", "", s.ID, f.project.ID)
	assert.ErrorIs(t, err, ErrBindingInvalid, "a thread or a project, not both")
	_, err = svc.BindChannel("telegram", "1", "", "", "")
	assert.ErrorIs(t, err, ErrBindingInvalid)
}
