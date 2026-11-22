package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A merge made on GitHub may first be seen by a tool's own record rather
// than by a reading: the thread comments on, edits or opens another PR from
// the branch before the sweep comes round. It is the same one event.

func approveAll(f *linksFixture) { f.answer = func(Approval) (bool, string) { return true, "" } }

func TestACommentIsTheFirstToSeeAMerge(t *testing.T) {
	f := eventsFixture(t)
	approveAll(f)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })

	_, err := f.links.Comment(context.Background(), f.thread(t), l.Number, "done")
	require.NoError(t, err)

	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0].Content, "PR #11 was merged into main.")
	in := githubInputs(t, f)
	require.Len(t, in, 1)
	assert.Contains(t, renderHistory(in, "")[0].Content, `kind="pr_merged"`)
	assert.Equal(t, 1, f.pending(t))

	// The sweep and a refresh after it say nothing more.
	f.sweepAfter(24 * time.Hour)
	_, err = f.links.RefreshThread(context.Background(), f.worker.ID, nil)
	require.NoError(t, err)
	assert.Len(t, githubNotes(t, f, f.worker.ID), 1)
	assert.Equal(t, 1, f.pending(t))
}

func TestASweepFirstLeavesTheCommentNothingToSay(t *testing.T) {
	f := eventsFixture(t)
	approveAll(f)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })
	f.sweepAfter(linkInterval)
	_, err := f.links.Comment(context.Background(), f.thread(t), l.Number, "done")
	require.NoError(t, err)
	assert.Len(t, githubNotes(t, f, f.worker.ID), 1)
	assert.Equal(t, 1, f.pending(t))
}

func TestAnUpdateIsTheFirstToSeeAClose(t *testing.T) {
	f := eventsFixture(t)
	approveAll(f)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "closed" })
	title := "Fix it"
	_, err := f.links.Update(context.Background(), f.thread(t), l.Number, &title, nil, nil)
	require.NoError(t, err)
	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0].Content, "PR #11 was closed without merging.")
	assert.Equal(t, 1, f.pending(t))
}

func TestOpeningAPRAgainTellsOfTheMergedOne(t *testing.T) {
	f := eventsFixture(t)
	approveAll(f)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })

	_, err := f.links.CreatePR(context.Background(), f.thread(t), "Follow-up", "", "", "", false, false)
	require.NoError(t, err)

	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 1, "the old PR's merge, once; the new PR is no event")
	assert.Contains(t, notes[0].Content, "PR #11 was merged into main.")
	assert.Equal(t, 1, f.pending(t))
}

// When GitHub cannot be read, the record is closed on a guess: nobody is told
// a PR was dropped that may have been merged.
func TestAGuessedCloseTellsNoOne(t *testing.T) {
	f := eventsFixture(t)
	approveAll(f)
	l := f.stackedPR(t, "Fix", "tp/work", "main")
	f.gh.mu.Lock()
	delete(f.gh.items, l.Number)
	f.gh.mu.Unlock()

	_, err := f.links.CreatePR(context.Background(), f.thread(t), "Follow-up", "", "", "", false, false)
	require.NoError(t, err)
	k, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", l.Number)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateClosed, k.State)
	assert.Empty(t, githubNotes(t, f, f.worker.ID))
	assert.Equal(t, 0, f.pending(t))
}

// A folder has no coordinator: its thread's PR is noted and that is all.
func TestAFolderThreadOnlyRecords(t *testing.T) {
	f := eventsFixture(t)
	hook := logtest.NewGlobal()
	t.Cleanup(hook.Reset)
	it := f.gh.add(db.LinkPR, "F", func(it *GitHubItem) { it.Head, it.Base = "tp/f", "main" })
	_, err := f.store.UpsertThreadLink(f.folder.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: it.Number, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/f", Base: "main"})
	require.NoError(t, err)
	f.gh.set(it.Number, func(it *GitHubItem) { it.State = "merged" })

	f.sweepAfter(linkInterval)

	notes := githubNotes(t, f, f.folder.ID)
	require.Len(t, notes, 1)
	assert.NotContains(t, notes[0].Payload, "coordinator_event")
	n, err := f.store.PendingCoordinatorInputs(f.folder.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	for _, e := range hook.AllEntries() {
		assert.Greater(t, e.Level, logrus.ErrorLevel, "an error was logged: %s", e.Message)
	}
}

// The thread's note says the coordinator was told, and which input it was.
func TestTheNoteLeadsToTheCoordinatorsInput(t *testing.T) {
	f := eventsFixture(t)
	a := f.stackedPR(t, "A", "tp/a", "main")
	b := f.stackedPR(t, "B", "tp/b", "main")
	f.gh.set(a.Number, func(it *GitHubItem) { it.State = "merged" })
	f.gh.set(b.Number, func(it *GitHubItem) { it.State = "closed" })
	f.sweepAfter(linkInterval)

	in := githubInputs(t, f)
	require.Len(t, in, 1)
	notes := githubNotes(t, f, f.worker.ID)
	require.Len(t, notes, 2)
	for _, n := range notes {
		var pl map[string]any
		require.NoError(t, json.Unmarshal([]byte(n.Payload), &pl))
		assert.Equal(t, in[0].ID, pl["coordinator_event"])
		assert.True(t, !n.CreatedAt.Before(in[0].CreatedAt), "the note is written after its input")
	}
}

func TestANoteNobodyWasToldOfSaysSo(t *testing.T) {
	for name, set := range map[string]func(t *testing.T, f *linksFixture){
		"paused": func(t *testing.T, f *linksFixture) {
			p, err := f.store.GetProject(f.p.ID)
			require.NoError(t, err)
			p.Paused = true
			require.NoError(t, f.store.SaveProject(p))
		},
		"resolved": func(t *testing.T, f *linksFixture) {
			now := time.Now()
			th := f.thread(t)
			th.Status, th.ResolvedAt = db.ThreadResolved, &now
			require.NoError(t, f.store.SaveThread(th))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := eventsFixture(t)
			l := f.stackedPR(t, "Fix", "tp/work", "main")
			set(t, f)
			f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })
			f.sweepAfter(linkInterval)
			notes := githubNotes(t, f, f.worker.ID)
			require.Len(t, notes, 1)
			assert.NotContains(t, notes[0].Payload, "coordinator_event")
		})
	}
}

// End to end: the coordinator, woken by a merge with work left, asks the
// thread to check; the thread runs one turn and its status is the thread's
// own, never the event's.
func TestTheCoordinatorCanAskTheThreadToFollowUpAMerge(t *testing.T) {
	var mu sync.Mutex
	threadInputs := []string{}
	f, svc := newServiceFixture(t, sinkHost{fakeSink: newFakeSink()}, func(turn *enginetest.Turn) error {
		mu.Lock()
		threadInputs = append(threadInputs, turn.Prompt)
		mu.Unlock()
		turn.Say("PR #12 still needs its base moved to main.", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		if !strings.Contains(c.InputEnvelope(), `kind="pr_merged"`) {
			c.Call("no_update", nil)
			return nil
		}
		out, isErr := c.Call("message_thread", map[string]any{
			"thread_id": c.ThreadRef("Fix export"),
			"message":   "From GitHub, not the user: PR #11 was merged. Check what is left and report.",
		})
		assert.False(t, isErr, out)
		c.Reply("PR #11 landed; PR #12 is still open, I asked the thread to check.")
		return nil
	})
	gh := newFakeGitHub()
	svc.SetGitHub(gh)
	w, err := svc.Threads.Start(context.Background(), f.project, StartSpec{Title: "Fix export", Task: "fix the export"})
	require.NoError(t, err)
	waitStatus(t, f.store, w.ID, db.ThreadIdle)
	waitQuiet(t, f, svc, w.ID)
	for _, pr := range []struct{ head, base string }{{"tp/x", "main"}, {"tp/y", "tp/x"}} {
		it := gh.add(db.LinkPR, "Fix export", func(it *GitHubItem) { it.Head, it.Base = pr.head, pr.base })
		_, err := f.store.UpsertThreadLink(w.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: it.Number, Role: db.LinkOpened, State: db.LinkStateOpen, Head: pr.head, Base: pr.base})
		require.NoError(t, err)
	}
	gh.set(11, func(it *GitHubItem) { it.State = "merged" })

	_, err = svc.Links.RefreshThread(context.Background(), w.ID, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(threadInputs) == 2 }, 5*time.Second, 10*time.Millisecond)
	waitStatus(t, f.store, w.ID, db.ThreadIdle)
	waitQuiet(t, f, svc, w.ID)

	mu.Lock()
	assert.Contains(t, threadInputs[1], "From GitHub, not the user")
	mu.Unlock()
	th, err := f.store.GetThread(w.ID)
	require.NoError(t, err)
	assert.Nil(t, th.ResolvedAt, "a merge never closes the thread")
	assert.NotEqual(t, db.ThreadResolved, th.Status)
}

func TestThePromptLetsTheCoordinatorAskForAFollowUp(t *testing.T) {
	p := coordinatorSystemPrompt(Layout{Root: t.TempDir()}, &db.Project{ID: "p1", Name: "Demo"}, nil, time.Now())
	assert.Contains(t, p, `source="github"`)
	assert.Contains(t, p, "you may message_thread to ask the thread to check what remains")
	assert.Contains(t, p, "Never resolve or close the thread yourself")
}
