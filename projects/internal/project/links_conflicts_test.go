package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func (f *linksFixture) setMerge(n int, state string) {
	f.gh.set(n, func(it *GitHubItem) { it.MergeState = state })
}

func TestEnteringConflictsIsOneEventAndOneInputThatNeedsTheUser(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix the export button", "tp/work", "main")
	f.setMerge(l.Number, "conflicts")
	before := f.thread(t)

	f.sweepAfter(linkInterval)

	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 1)
	assert.Equal(t, "PR #11 has merge conflicts with main. Thread: ready, no turn running, 0 unpushed commits, no uncommitted changes.", notes[0].Content)
	in := githubInputs(t, f)
	require.Len(t, in, 1)
	assert.Equal(t, 1, f.pending(t))
	body := renderHistory(in, "")[0].Content
	assert.Contains(t, body, `kind="pr_conflicts"`)
	assert.Contains(t, body, `needs_user="true"`)
	assert.Contains(t, body, `trust="relay"`)
	assert.Equal(t, before.Status, f.thread(t).Status)

	// Still conflicting on the next sweep: nothing more.
	f.sweepAfter(linkInterval)
	assert.Len(t, githubNotes(t, f, f.worker.ID), 1)
	assert.Equal(t, 1, f.pending(t))
}

func TestMergedAndClosedNeverAskForTheUser(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })
	f.sweepAfter(linkInterval)
	assert.NotContains(t, renderHistory(githubInputs(t, f), "")[0].Content, "needs_user")
}

func TestLeavingConflictsIsOnlyRecorded(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval)
	require.Equal(t, 1, f.pending(t))

	f.setMerge(l.Number, "ready")
	f.sweepAfter(linkInterval)

	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 2)
	assert.Equal(t, "Conflicts on PR #11 are resolved.", notes[1].Content)
	assert.Len(t, githubInputs(t, f), 1, "no new input")
	assert.Equal(t, 1, f.pending(t))
}

func TestConflictsWakeTheCoordinatorOncePerPRInThirtyMinutes(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval) // woke at +5
	f.setMerge(l.Number, "ready")
	f.sweepAfter(linkInterval) // +10
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval) // +15: entered again, within 30 minutes of the wake

	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 3, "entered, resolved, entered again: all recorded")
	assert.Contains(t, notes[2].Content, "has merge conflicts")
	assert.Equal(t, 1, f.pending(t), "but the coordinator was woken once")

	f.setMerge(l.Number, "ready")
	f.sweepAfter(linkInterval) // +20
	f.advance(conflictWakeGap - 10*time.Minute)
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval) // past 30 minutes since the first wake
	assert.Equal(t, 2, f.pending(t), "after the gap a new entry wakes it")
}

func TestConflictsOfTwoPRsAreOneInputPerSweep(t *testing.T) {
	f := eventsFixture(t)
	a := f.stackedPR(t, "A", "tp/a", "main")
	b := f.stackedPR(t, "B", "tp/b", "main")
	f.setMerge(a.Number, "conflicts")
	f.setMerge(b.Number, "conflicts")

	f.sweepAfter(linkInterval)

	assert.Len(t, githubNotes(t, f, f.worker.ID), 2)
	require.Len(t, githubInputs(t, f), 1)
	assert.Equal(t, 1, f.pending(t))
	assert.Equal(t, 2, strings.Count(renderHistory(githubInputs(t, f), "")[0].Content, `needs_user="true"`))
}

func TestAFirstReadingThatIsAlreadyConflictingCounts(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	assert.Empty(t, f.link(t, l.Number).MergeState, "never read for merge state")
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval)
	assert.Equal(t, 1, f.pending(t))
}

// conflictFixture is a project whose coordinator is scripted by reply, with a
// chat bound to it and a thread with an open PR that GitHub now says conflicts.
func conflictFixture(t *testing.T, reply func(*coordtest.Turn) error) (*testFixture, *Service, *fakeSink, *fakeGitHub, *db.Thread) {
	t.Helper()
	sink := newFakeSink()
	f, svc := newServiceFixture(t, sinkHost{fakeSink: sink}, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	}, reply)
	gh := newFakeGitHub()
	svc.SetGitHub(gh)
	_, err := svc.BindChannel("telegram", "42", "Me", "", f.project.ID)
	require.NoError(t, err)
	w := &db.Thread{ProjectID: f.project.ID, ParentID: f.project.MainThreadID, Agent: db.AgentClaudeCode, Title: "Fix export", Status: db.ThreadReady}
	require.NoError(t, f.store.CreateThread(w))
	it := gh.add(db.LinkPR, "Fix export", func(it *GitHubItem) { it.Head, it.Base, it.MergeState = "tp/x", "main", "conflicts" })
	_, err = f.store.UpsertThreadLink(w.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: it.Number, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/x", Base: "main"})
	require.NoError(t, err)
	return f, svc, sink, gh, w
}

func refreshConflicts(t *testing.T, svc *Service, w *db.Thread) {
	t.Helper()
	_, err := svc.Links.RefreshThread(context.Background(), w.ID, nil)
	require.NoError(t, err)
}

func isConflictInput(c *coordtest.Turn) bool {
	return strings.Contains(c.InputEnvelope(), `needs_user="true"`)
}

func TestACoordinatorThatSaysNothingAboutConflictsLeavesALineInTheChat(t *testing.T) {
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		c.Call("no_update", nil)
		return nil
	})
	refreshConflicts(t, svc, w)

	got := sink.nextContaining(t, "has merge conflicts with main.")
	assert.Contains(t, got.text, "Reply with #")
	assert.Contains(t, got.text, "to tell the thread what to do.")
	assert.Contains(t, got.text, "Fix export")
	waitQuiet(t, f, svc, w.ID)
	assert.Equal(t, db.ThreadReady, threadStatus(t, f, w.ID), "nothing moved the thread")
}

func TestACoordinatorThatAsksViaADecisionCardNeedsNoFallback(t *testing.T) {
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		if isConflictInput(c) {
			out, isErr := c.Call("request_decision", map[string]any{
				"question": "PR #11 conflicts with main. What now?",
				"options": []map[string]string{
					{"label": "Merge main into the branch and push"},
					{"label": "I will handle it"},
					{"label": "Drop the PR"},
				},
				"recommended": 0,
			})
			assert.False(t, isErr, out)
		}
		return nil
	})
	refreshConflicts(t, svc, w)
	waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvDecision) == 1 })
	waitQuiet(t, f, svc, w.ID)
	for {
		select {
		case m := <-sink.ch:
			assert.NotContains(t, m.text, "has merge conflicts", "the card is how the user is told")
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
}

func TestACoordinatorThatRepliesAboutConflictsNeedsNoFallback(t *testing.T) {
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		c.Reply("Fix export has conflicts with main. What should I do?")
		return nil
	})
	refreshConflicts(t, svc, w)
	got := sink.nextContaining(t, "has conflicts with main. What should I do?")
	assert.NotContains(t, got.text, "Reply with #")
	waitQuiet(t, f, svc, w.ID)
	sink.none(t)
}

func TestPausedProjectGetsTheConflictLineAndNoInput(t *testing.T) {
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		t.Error("a paused project's coordinator must not run")
		return nil
	})
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	p.Paused = true
	require.NoError(t, f.store.SaveProject(p))

	refreshConflicts(t, svc, w)

	got := sink.nextContaining(t, "has merge conflicts with main.")
	assert.Contains(t, got.text, "Reply with #")
	n, err := f.store.PendingCoordinatorInputs(f.project.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, db.ThreadReady, threadStatus(t, f, w.ID))
}

func threadStatus(t *testing.T, f *testFixture, id string) string {
	t.Helper()
	th, err := f.store.GetThread(id)
	require.NoError(t, err)
	return th.Status
}

// A coordinator turn that fails asked nobody: the chats still get the line.
func TestACoordinatorTurnThatFailsLeavesTheConflictLineInTheChat(t *testing.T) {
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		return errors.New("model unavailable")
	})
	refreshConflicts(t, svc, w)
	got := sink.nextContaining(t, "has merge conflicts with main.")
	assert.Contains(t, got.text, "to tell the thread what to do.")
	waitQuiet(t, f, svc, w.ID)
}

// restart is a new Links over the same store, as after the service restarted:
// whatever it kept in memory is gone.
func (f *linksFixture) restart() {
	events := f.links.events
	f.links = NewLinks(f.store, f.gh, func(id string) { f.events = append(f.events, id) })
	f.links.now = func() time.Time { return *f.clock }
	f.links.events = events
	f.svc.Links = f.links
}

func TestAConflictThatStaysIsOneAlertThroughUnknownReadingsAndRestarts(t *testing.T) {
	f := eventsFixture(t)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval)
	require.Equal(t, 1, f.pending(t))

	// Hours of the same conflict, GitHub recomputing in between (main moved),
	// and the service restarting twice: well past the 30-minute cap.
	for i := 0; i < 30; i++ {
		if i%2 == 0 {
			f.setMerge(l.Number, "unknown")
		} else {
			f.setMerge(l.Number, "conflicts")
		}
		if i == 9 || i == 20 {
			f.restart()
		}
		f.sweepAfter(linkInterval)
		if i%2 == 0 {
			assert.Equal(t, "conflicts", f.link(t, l.Number).MergeState, "an unknown reading keeps the last settled one")
		}
	}

	assert.Len(t, githubNotes(t, f, f.worker.ID), 1, "nothing more recorded")
	assert.Equal(t, 1, f.pending(t), "the coordinator was woken once")

	// Leaving and coming back, past the cap, is a new entry.
	f.setMerge(l.Number, "ready")
	f.sweepAfter(linkInterval)
	f.setMerge(l.Number, "unknown")
	f.sweepAfter(linkInterval)
	assert.Equal(t, "ready", f.link(t, l.Number).MergeState)
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval)
	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 3, "entered, resolved, entered again")
	assert.Equal(t, 2, f.pending(t))
}

func TestACommentReadingUnknownDoesNotEnterConflictsAgain(t *testing.T) {
	f := eventsFixture(t)
	f.allowAll()
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.setMerge(l.Number, "conflicts")
	f.sweepAfter(linkInterval)
	require.Equal(t, 1, f.pending(t))

	// The comment tool records its own reading of the PR.
	f.setMerge(l.Number, "unknown")
	_, err := f.links.Comment(context.Background(), f.thread(t), l.Number, "Rebasing on main.")
	require.NoError(t, err)
	assert.Equal(t, "conflicts", f.link(t, l.Number).MergeState)

	f.setMerge(l.Number, "conflicts")
	f.advance(conflictWakeGap)
	f.sweepAfter(linkInterval)
	assert.Len(t, githubNotes(t, f, f.worker.ID), 1)
	assert.Equal(t, 1, f.pending(t))
}

// The whole loop of a conflict: the coordinator asks about the thread, the
// user picks, the answer reaches the coordinator naming that card and thread,
// and the coordinator relays it to that thread with the pick quoted.
func TestAConflictAnswerReachesItsOwnThread(t *testing.T) {
	attr := func(env, key string) string {
		if m := regexp.MustCompile(` ` + key + `="([^"]*)"`).FindStringSubmatch(env); m != nil {
			return m[1]
		}
		return ""
	}
	var mu sync.Mutex
	var seen string // the answer's input, as the coordinator's session got it
	f, svc, sink, _, w := conflictFixture(t, func(c *coordtest.Turn) error {
		env := c.InputEnvelope()
		switch {
		case isConflictInput(c):
			out, isErr := c.Call("request_decision", map[string]any{
				"question":    "Resolve the conflicts with main?",
				"options":     []map[string]string{{"label": "Merge main and push"}, {"label": "I will handle it"}, {"label": "Drop the PR"}},
				"recommended": 0,
				"thread_id":   attr(env, "id"),
			})
			assert.False(t, isErr, out)
		case attr(env, "kind") == "answer":
			mu.Lock()
			seen = env + "\n" + c.Input()
			mu.Unlock()
			out, isErr := c.Call("message_thread", map[string]any{"thread_id": attr(env, "id"), "message": "The user decided.", "cite": []string{attr(env, "msg")}})
			assert.False(t, isErr, out)
		default:
			c.Reply("ok")
		}
		return nil
	})
	w.Cwd, w.SessionID = f.repo, "fix-export" // the relayed message runs a turn of the thread
	require.NoError(t, f.store.SaveThread(w))
	done := make(chan struct{})
	defer close(done)
	go func() { // the chat hears the card and the replies; nobody reads them here
		for {
			select {
			case <-sink.ch:
			case <-done:
				return
			}
		}
	}()
	refreshConflicts(t, svc, w)
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvDecision) == 1 })
	var card db.ThreadEvent
	for _, m := range msgs {
		if m.Kind == db.EvDecision {
			card = m
		}
	}
	var d Decision
	require.NoError(t, json.Unmarshal([]byte(card.Payload), &d))
	assert.Equal(t, w.ID, d.ThreadID)
	assert.Equal(t, db.LinkPR, d.LinkKind, "the thread's one open PR")

	_, err := svc.Coordinator.Choose(context.Background(), f.project.MainThreadID, card.ID, 1, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		events, _ := f.store.ListThreadEvents(w.ID)
		for _, e := range events {
			if e.Kind == db.EvUser && strings.Contains(e.Content, "> I will handle it") &&
				strings.Contains(e.Content, `the user's pick on the decision card "Resolve the conflicts with main?"`) {
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "the thread got the user's pick and the question it answers")
	mu.Lock()
	got := seen
	mu.Unlock()
	t.Logf("the coordinator's input for the answer:\n%s", got)
	assert.Contains(t, got, fmt.Sprintf(`thread="#%d Fix export"`, w.Seq))
	assert.Contains(t, got, fmt.Sprintf("(System: answer to your decision card %s, about #%d Fix export · PR #", short(card.ID), w.Seq))
	assert.Contains(t, got, "The user picked option 2:)\nI will handle it")
	waitQuiet(t, f, svc, w.ID)
}
