package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// sentText is one message the fake sink was asked to deliver.
type sentText struct{ platform, chatID, text string }

type fakeSink struct {
	mu   sync.Mutex
	sent []sentText
	ch   chan sentText
	err  error
}

func newFakeSink() *fakeSink { return &fakeSink{ch: make(chan sentText, 32)} }

func (f *fakeSink) SendChannelText(_ context.Context, platform, chatID, text string) error {
	m := sentText{platform, chatID, text}
	f.mu.Lock()
	f.sent = append(f.sent, m)
	f.mu.Unlock()
	f.ch <- m
	return f.err
}

func (f *fakeSink) next(t *testing.T) sentText {
	t.Helper()
	select {
	case m := <-f.ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was sent to the chat")
		return sentText{}
	}
}

func (f *fakeSink) none(t *testing.T) {
	t.Helper()
	select {
	case m := <-f.ch:
		t.Fatalf("unexpected send: %+v", m)
	case <-time.After(150 * time.Millisecond):
	}
}

// channelFixture is a project with a coordinator whose replies go to chats.
func channelFixture(t *testing.T) (*testFixture, *Service, *fakeSink) {
	t.Helper()
	return channelFixtureWith(t, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	})
}

// waitQuiet waits until a thread's turn has fully ended and the relay has
// delivered everything queued, so a later binding cannot catch a late reply.
func waitQuiet(t *testing.T, f *testFixture, svc *Service, threadID string) {
	t.Helper()
	require.Eventually(t, func() bool { return !f.threads.Running(threadID) }, 5*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		svc.Relay.mu.Lock()
		defer svc.Relay.mu.Unlock()
		return !svc.Relay.running && len(svc.Relay.queue) == 0
	}, 5*time.Second, 5*time.Millisecond)
}

func channelFixtureWith(t *testing.T, script func(turn *enginetest.Turn) error) (*testFixture, *Service, *fakeSink) {
	t.Helper()
	f := newFixture(t, script)
	src := &fakeLLM{fn: func(req llm.Request) llm.Result {
		return llm.Result{Text: "Echo: " + lastUserText(req)}
	}}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, src)
	t.Cleanup(func() { settleCoordinator(coord) })
	sink := newFakeSink()
	relay := NewRelay(f.store, sink)
	coord.OnFinal = relay.Message
	f.threads.OnWait = relay.NeedsUser
	f.threads.OnSettled = relay.Settled
	f.threads.OnReply = relay.ThreadReply
	f.threads.OnStep = relay.Step
	return f, &Service{Store: f.store, Layout: f.threads.layout, Hub: f.threads.hub, Threads: f.threads, Coordinator: coord, Relay: relay}, sink
}

func TestChannelMessageReachesTheCoordinatorAndTheReplyComesBack(t *testing.T) {
	f, svc, sink := channelFixture(t)
	b, err := svc.BindChannel("telegram", "42", "Me", "", f.project.ID)
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, b.ThreadID, "a project is bound through its main thread")
	assert.Equal(t, f.project.ID, b.ProjectID)

	handled, err := svc.PostFromChannel("telegram", "42", "How is it going?", nil)
	require.NoError(t, err)
	require.True(t, handled)

	got := sink.next(t)
	assert.Equal(t, "telegram", got.platform)
	assert.Equal(t, "42", got.chatID)
	assert.Equal(t, "[Demo] Echo: How is it going?", got.text, "replies carry the project name")

	// The question is a user message of the project conversation, marked as
	// coming from the chat, and the web sees the reply too.
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return len(m) == 2 })
	assert.Equal(t, db.EvUser, msgs[0].Kind)
	assert.Equal(t, db.SourceIM, msgs[0].Source)
	assert.Equal(t, "telegram", msgs[0].SourcePlatform)
	assert.Equal(t, "42", msgs[0].SourceChatID)
	assert.Equal(t, db.EvAssistant, msgs[1].Kind)
	assert.Empty(t, msgs[1].Source)

	// A message typed on the web is not echoed to the chat.
	_, err = svc.Coordinator.Post(f.project.ID, "From the web", nil, Source{})
	require.NoError(t, err)
	assert.Equal(t, "[Demo] Echo: From the web", sink.next(t).text)
	waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return len(m) == 4 })
	sink.none(t)
}

func TestUnboundChatIsNotHandled(t *testing.T) {
	f, svc, sink := channelFixture(t)
	handled, err := svc.PostFromChannel("telegram", "nobody", "hello", nil)
	require.NoError(t, err)
	assert.False(t, handled)
	sink.none(t)
	msgs := convo(t, f.store, f.project.ID)
	assert.Empty(t, msgs, "an unbound chat never touches a conversation")
}

func TestBoundPausedProjectReportsTheErrorToTheCaller(t *testing.T) {
	f, svc, _ := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetPaused(p, true))

	handled, err := svc.PostFromChannel("telegram", "1", "hi", nil)
	assert.True(t, handled, "a bound chat is never passed on to @cc, even when the project cannot answer")
	assert.ErrorIs(t, err, ErrPaused)
}

func TestRepliesGoToEveryBoundChat(t *testing.T) {
	f, svc, sink := channelFixture(t)
	for _, c := range []string{"phone", "group"} {
		_, err := svc.BindChannel("telegram", c, "", "", f.project.ID)
		require.NoError(t, err)
	}
	_, err := svc.PostFromChannel("telegram", "phone", "ping", nil)
	require.NoError(t, err)
	first, second := sink.next(t), sink.next(t)
	assert.ElementsMatch(t, []string{"phone", "group"}, []string{first.chatID, second.chatID})
}

func TestMovingAChatChangesWhereRepliesGo(t *testing.T) {
	f, svc, sink := channelFixture(t)
	other := &db.Project{Name: "Other"}
	mkProject(t, f.store, other)
	require.NoError(t, svc.Layout.Ensure(other.ID))

	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "1", "", "", other.ID)
	require.NoError(t, err)

	_, err = svc.PostFromChannel("telegram", "1", "hi", nil)
	require.NoError(t, err)
	assert.Equal(t, "[Other] Echo: hi", sink.next(t).text)
	none := convo(t, f.store, f.project.ID)
	assert.Empty(t, none)
}

func TestBindChannelValidates(t *testing.T) {
	f, svc, _ := channelFixture(t)
	_, err := svc.BindChannel("", "1", "", "", f.project.ID)
	assert.ErrorIs(t, err, ErrBindingInvalid)
	_, err = svc.BindChannel("telegram", "1", "", "", "no-such-project")
	assert.ErrorIs(t, err, db.ErrNotFound)

	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetArchived(p, true))
	_, err = svc.BindChannel("telegram", "1", "", "", f.project.ID)
	assert.ErrorIs(t, err, ErrArchived)
}

func TestThreadReportsAreFoldedToOneLine(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	update := func(kind, text string) db.ThreadEvent {
		return db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvThreadUpdate, Content: text,
			Payload: `{"thread_id":"t1","seq":2,"title":"Fix login","kind":"` + kind + `","status":"idle"}`}
	}
	svc.Relay.Message(update(ReportFinished, "Fixed the login bug.\n\nDetails:\n- changed a.go\n- changed b.go"))
	assert.Equal(t, `[#2 Fix login] Finished: Fixed the login bug.`, sink.next(t).text)

	svc.Relay.Message(update(ReportFailed, "exit status 1"))
	assert.Equal(t, `[#2 Fix login] Failed: exit status 1`, sink.next(t).text)

	svc.Relay.Message(update(ReportNeedsInput, "Keep the legacy flag?"))
	got := sink.next(t).text
	assert.True(t, strings.HasPrefix(got, `[#2 Fix login] Needs you: Keep the legacy flag?`), got)
	assert.Contains(t, got, "Reply with @2 and your answer.", "a thread waiting on the user is answered with @N")

	long := strings.Repeat("x", 1000)
	svc.Relay.Message(update(ReportFinished, long))
	assert.Less(t, len([]rune(sink.next(t).text)), 300, "a long report is cut")

	// What the chat or web already shows, and kinds without text, are skipped.
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Content: "mine"})
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvThreadStarted, Payload: `{"thread_ids":["a"]}`})
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "  "})
	sink.none(t)

	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvError, Content: "coordinator unavailable"})
	assert.Equal(t, "[Demo] Error: coordinator unavailable", sink.next(t).text)
}

func TestEveryApprovalIsAskedInTheChat(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	worker := &db.Thread{ProjectID: f.project.ID, ParentID: f.project.MainThreadID, Title: "Fix login"}
	require.NoError(t, f.store.CreateThread(worker))
	th := *worker
	svc.Relay.NeedsUser(th, approvalEvent("r1"))
	assert.Equal(t, fmt.Sprintf("[#%d Fix login] Approve: Run `ls`\nReply y to allow, always to allow it for this thread, n to deny, or say what to do instead.", th.Seq), sink.next(t).text)
	svc.Relay.NeedsUser(th, approvalEvent("r2")) // each one needs its answer
	sink.next(t)
	threadID, requestID, ok := svc.Relay.Awaiting("telegram", "1")
	require.True(t, ok)
	assert.Equal(t, [2]string{th.ID, "r1"}, [2]string{threadID, requestID}, "the oldest is answered first")

	// A request without an id cannot be answered from a chat.
	svc.Relay.NeedsUser(th, db.ThreadEvent{Kind: db.EvApproval, Content: "Run `ls`"})
	assert.Contains(t, sink.next(t).text, "Open Projects in the web UI to answer.")

	// A folder thread nobody bound hears nothing, whoever is bound elsewhere.
	folderThread := &db.Thread{ProjectID: mkProject(t, f.store, &db.Project{Name: "F"}).ID, Title: "folder"}
	require.NoError(t, f.store.CreateThread(folderThread))
	svc.Relay.NeedsUser(*folderThread, approvalEvent("r4"))
	sink.none(t)
}

func TestSinkFailureDoesNotStopLaterMessages(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	sink.err = errors.New("bot offline")
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "one"})
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "two"})
	assert.Equal(t, "[Demo] one", sink.next(t).text)
	assert.Equal(t, "[Demo] two", sink.next(t).text, "delivered in order, after a failure")
}

func TestWithoutASinkNothingIsRelayed(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { return nil })
	relay := NewRelay(f.store, nil)
	_, err := (&Service{Store: f.store}).BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "hi"})
	relay.NeedsUser(db.Thread{ID: "t", ProjectID: f.project.ID}, approvalEvent("r"))
	relay.mu.Lock()
	defer relay.mu.Unlock()
	assert.Empty(t, relay.queue)
	assert.False(t, relay.running, "standalone hosts have no IM; the relay does nothing")
}

func TestWorkerTurnEndReachesChatsBoundToThatThreadOnly(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "own", "", th.ID, "")
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)

	_, err = svc.Send(context.Background(), th.ID, "again", nil, Source{})
	require.NoError(t, err)

	// The chat on the worker hears its reply once; the chat on the main thread
	// hears the coordinator (the folded report and its own reply), never the
	// worker's raw text as well.
	byChat := map[string][]string{}
	deadline := time.After(5 * time.Second)
	for len(byChat["own"]) < 1 || len(byChat["up"]) < 2 {
		select {
		case m := <-sink.ch:
			byChat[m.chatID] = append(byChat[m.chatID], m.text)
		case <-deadline:
			t.Fatalf("got only %v", byChat)
		}
	}
	sink.none(t)
	tag := fmt.Sprintf("[#%d Worker] ", th.Seq)
	assert.Equal(t, []string{tag + "ok"}, byChat["own"], "the worker's final text, named by its number and title")
	assert.NotContains(t, byChat["up"], tag+"ok")
	assert.Contains(t, byChat["up"], tag+"Finished: ok")
}

func TestWorkerFailureReachesItsChatAsAnError(t *testing.T) {
	f, svc, sink := channelFixtureWith(t, func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "boom") {
			return errors.New("it broke")
		}
		turn.Say("ok", 1, 1)
		return nil
	})
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "own", "", th.ID, "")
	require.NoError(t, err)

	_, err = svc.Send(context.Background(), th.ID, "boom", nil, Source{})
	require.NoError(t, err)
	assert.Contains(t, sink.next(t).text, fmt.Sprintf("[#%d Worker] Error: ", th.Seq))
	sink.none(t)
}

func TestFolderThreadReplyReachesItsChat(t *testing.T) {
	f, svc, sink := channelFixture(t)
	dir := t.TempDir()
	s, err := f.startFolder(dir, FolderThreadSpec{Task: "go"})
	require.NoError(t, err)
	waitQuiet(t, f, svc, s.ID)
	sink.none(t)
	_, err = svc.BindChannel("telegram", "fold", "", s.ID, "")
	require.NoError(t, err)

	_, err = svc.Send(context.Background(), s.ID, "more", nil, Source{})
	require.NoError(t, err)
	got := sink.next(t)
	assert.Equal(t, "fold", got.chatID)
	cur, err := f.store.GetThread(s.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, cur.Seq)
	assert.Equal(t, "["+threadTag(cur.Seq, cur.Title)+"] ok", got.text, "a folder thread is named like any other thread")
	assert.True(t, strings.HasPrefix(got.text, "[#1"), got.text)
}

func TestArchivingAProjectUnbindsItsChats(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "main", "", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "worker", "", th.ID, "")
	require.NoError(t, err)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err = svc.BindChannel("telegram", "other", "", "", other.ID)
	require.NoError(t, err)

	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	require.NoError(t, svc.SetArchived(p, true))

	for _, chat := range []string{"main", "worker"} {
		handled, err := svc.PostFromChannel("telegram", chat, "hi", nil)
		assert.False(t, handled, "%s falls back to what an unbound chat does", chat)
		assert.NoError(t, err)
	}
	_, err = f.store.GetChannelBinding("telegram", "other")
	assert.NoError(t, err, "another project's chat stays bound")

	require.NoError(t, svc.SetArchived(p, false))
	left, err := f.store.ListChannelBindings(db.BindingFilter{ProjectID: f.project.ID})
	require.NoError(t, err)
	assert.Empty(t, left, "unarchiving does not bind chats again")
	sink.none(t)
}

// approvalEvent is a pending approval of `ls` as the transcript has it.
func approvalEvent(requestID string) db.ThreadEvent {
	return db.ThreadEvent{Kind: db.EvApproval, Content: "Run `ls`", Payload: `{"request_id":"` + requestID + `","tool":"Bash","state":"pending"}`}
}

func TestVerboseChatsSeeEachStepOthersOnlyTheReply(t *testing.T) {
	f, svc, sink := channelFixtureWith(t, func(turn *enginetest.Turn) error {
		turn.Say("Looking around.", 1, 1)
		turn.Use("tu1", "Bash", map[string]any{"command": "ls"})
		turn.Use("tu2", "Read", map[string]any{"file_path": "go.mod"})
		turn.Say("done", 1, 1)
		return nil
	})
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	for _, chat := range []string{"loud", "quiet"} {
		_, err := svc.BindChannel("telegram", chat, "", th.ID, "")
		require.NoError(t, err)
	}
	handled, reply, err := svc.ChannelCommand("telegram", "loud", ChatVerbose, "on")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Contains(t, reply, "Verbose is on")

	_, err = svc.Send(context.Background(), th.ID, "again", nil, Source{})
	require.NoError(t, err)
	byChat := map[string][]string{}
	deadline := time.After(5 * time.Second)
	for len(byChat["loud"]) < 3 || len(byChat["quiet"]) < 1 {
		select {
		case m := <-sink.ch:
			byChat[m.chatID] = append(byChat[m.chatID], m.text)
		case <-deadline:
			t.Fatalf("got only %v", byChat)
		}
	}
	sink.none(t)
	tag := fmt.Sprintf("[#%d Worker] ", th.Seq)
	assert.Equal(t, []string{
		tag + "Looking around.\n→ Run `ls`",
		tag + "→ Read: go.mod",
		tag + "done",
	}, byChat["loud"], "each tool call with what was said before it, then the reply once")
	assert.Equal(t, []string{tag + "done"}, byChat["quiet"])

	_, reply, err = svc.ChannelCommand("telegram", "loud", ChatVerbose, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "Verbose is on")
	_, reply, err = svc.ChannelCommand("telegram", "loud", ChatVerbose, "off")
	require.NoError(t, err)
	assert.Contains(t, reply, "Verbose is off")
	b, err := f.store.GetChannelBinding("telegram", "loud")
	require.NoError(t, err)
	assert.False(t, b.Verbose)
	_, reply, err = svc.ChannelCommand("telegram", "loud", ChatVerbose, "loud")
	require.NoError(t, err)
	assert.Equal(t, "Usage: /verbose on|off", reply)
}

func TestFilesAChatSendsReachTheThreadAsAttachments(t *testing.T) {
	f, svc, sink := channelFixture(t)
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go"})
	require.NoError(t, err)
	waitQuiet(t, f, svc, s.ID)
	_, err = svc.BindChannel("telegram", "fold", "", s.ID, "")
	require.NoError(t, err)

	sent := filepath.Join(t.TempDir(), "AgACAgUAAx.jpg")
	require.NoError(t, os.WriteFile(sent, []byte("not really a photo"), 0o644))
	handled, err := svc.PostFromChannel("telegram", "fold", "what is this", []ChannelFile{{Name: "photo.jpg", Path: sent}})
	require.NoError(t, err)
	require.True(t, handled)
	sink.next(t)
	waitQuiet(t, f, svc, s.ID)

	evs, err := f.store.ListThreadEvents(s.ID)
	require.NoError(t, err)
	var atts []Attachment
	for _, e := range evs {
		if e.Kind == db.EvUser && e.Content == "what is this" {
			atts = ParseAttachments(e.Payload)
		}
	}
	require.Len(t, atts, 1)
	assert.Equal(t, "photo.jpg", atts[0].Name, "named as the chat named it")
	abs, err := f.threads.layout.LibraryPath(s.ProjectID, atts[0].Path)
	require.NoError(t, err)
	body, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, "not really a photo", string(body), "a copy the thread owns, so the host may remove its own")
}

func TestAChatThatNeverTalkedStartsInItsFolder(t *testing.T) {
	f, svc, sink := channelFixture(t)
	dir := t.TempDir()
	require.NoError(t, svc.ChannelToFolder("telegram", "new", "Alice", dir, "", true))

	handled, reply, err := svc.ChannelCommand("telegram", "new", ChatStop, "")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Contains(t, reply, "starts a new session in "+db.FolderName(dir))
	list, err := f.store.ListChannelBindings(db.BindingFilter{})
	require.NoError(t, err)
	assert.Empty(t, list, "a chat with only a folder is not listed on the web")

	handled, err = svc.PostFromChannel("telegram", "new", "hello", nil)
	require.NoError(t, err)
	require.True(t, handled)
	got := sink.next(t)
	assert.Equal(t, "new", got.chatID)
	b, err := f.store.GetChannelBinding("telegram", "new")
	require.NoError(t, err)
	assert.Empty(t, b.Folder)
	th, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	p, err := f.store.GetProject(th.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, db.ProjectFolder, p.Type)
	assert.Equal(t, db.PathKey(dir), db.PathKey(p.Path))
	assert.Equal(t, "["+threadTag(th.Seq, th.Title)+"] ok", got.text)
	waitQuiet(t, f, svc, th.ID)

	// Once it has a place, ifNew leaves it there.
	require.NoError(t, svc.ChannelToFolder("telegram", "new", "", t.TempDir(), "", true))
	b, err = f.store.GetChannelBinding("telegram", "new")
	require.NoError(t, err)
	assert.Empty(t, b.Folder)
}

func TestAChatThatNeverTalkedStartsUnderItsBotsProfile(t *testing.T) {
	f, svc, sink := channelFixture(t)
	work := filepath.Join(t.TempDir(), "work.json")
	require.NoError(t, os.WriteFile(work, []byte(`{"env":{"X":"1"}}`), 0o644))
	f.threads.env = profiles{settings: work}
	require.NoError(t, svc.ChannelToFolder("telegram", "new", "", t.TempDir(), "work", true))

	handled, err := svc.PostFromChannel("telegram", "new", "hello", nil)
	require.NoError(t, err)
	require.True(t, handled)
	sink.next(t)
	b, err := f.store.GetChannelBinding("telegram", "new")
	require.NoError(t, err)
	first, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "work", first.Profile)
	waitQuiet(t, f, svc, first.ID)

	// The next session keeps the profile of the one before.
	_, _, err = svc.ChannelCommand("telegram", "new", ChatNew, "again")
	require.NoError(t, err)
	b, err = f.store.GetChannelBinding("telegram", "new")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, b.ThreadID)
	next, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "work", next.Profile)
	waitQuiet(t, f, svc, next.ID)

	// A profile that is gone falls back to the default.
	require.NoError(t, svc.ChannelToFolder("telegram", "other", "", t.TempDir(), "gone", true))
	_, err = svc.PostFromChannel("telegram", "other", "hello", nil)
	require.NoError(t, err)
	sink.next(t)
	b, err = f.store.GetChannelBinding("telegram", "other")
	require.NoError(t, err)
	other, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	assert.Empty(t, other.Profile)
	waitQuiet(t, f, svc, other.ID)
}

func TestMovingAChatToAnotherFolderStartsThereNext(t *testing.T) {
	f, svc, sink := channelFixture(t)
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go"})
	require.NoError(t, err)
	waitQuiet(t, f, svc, s.ID)
	_, err = svc.BindChannel("telegram", "c", "", s.ID, "")
	require.NoError(t, err)

	other := t.TempDir()
	require.NoError(t, svc.ChannelToFolder("telegram", "c", "", other, "", false))
	assert.ErrorIs(t, svc.ChannelToFolder("telegram", "c", "", filepath.Join(other, "missing"), "", false), ErrNotFolder)
	_, reply, err := svc.ChannelCommand("telegram", "c", ChatSession, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "#1", "other commands still act on the thread it is on")

	_, err = svc.PostFromChannel("telegram", "c", "over here", nil)
	require.NoError(t, err)
	sink.next(t)
	b, err := f.store.GetChannelBinding("telegram", "c")
	require.NoError(t, err)
	assert.NotEqual(t, s.ID, b.ThreadID)
	th, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "over here", th.Task)
	p, err := f.store.GetProject(th.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, db.PathKey(other), db.PathKey(p.Path))
	waitQuiet(t, f, svc, th.ID)
}
