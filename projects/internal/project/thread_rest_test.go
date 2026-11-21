package project

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The thread lifecycle of .design/thread-lifecycle.md: one rule for where a
// thread rests, one way to close it, a 7-day rule that only tidies away
// finished threads, and no waiting reason that names a dead question.

func TestRestingStatusIsOneRule(t *testing.T) {
	merged := time.Now()
	for _, c := range []struct {
		name   string
		thread db.Thread
		want   string
	}{
		{"no commits", db.Thread{}, db.ThreadIdle},
		{"commits to review", db.Thread{CommitsAhead: 2}, db.ThreadReady},
		{"merged from the Overview", db.Thread{CommitsAhead: 2, MergedAt: &merged}, db.ThreadIdle},
	} {
		assert.Equal(t, c.want, db.RestingStatus(&c.thread), c.name)
	}
}

// commitOnce commits a file on the thread's first turn only, so later turns
// find the same commits ahead and nothing new.
func commitOnce(t *testing.T, block bool) func(turn *enginetest.Turn) error {
	return func(turn *enginetest.Turn) error {
		dir := turn.Opts.ProjectPath
		if _, err := os.Stat(filepath.Join(dir, "work.txt")); os.IsNotExist(err) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("x\n"), 0o644))
			run(t, dir, "git", "add", ".")
			run(t, dir, "git", "commit", "-qm", "work")
		}
		if block {
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("Done.", 1, 1)
		return nil
	}
}

func TestAStoppedThreadWithCommitsIsReadyForReview(t *testing.T) {
	f := newFixture(t, commitOnce(t, true))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Work", Task: "commit then wait"})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(th.Cwd, "work.txt"))
		return err == nil && run(t, th.Cwd, "git", "status", "--porcelain") == ""
	}, 5*time.Second, 10*time.Millisecond, "the turn committed")

	f.threads.Stop(th.ID, Source{})
	got := waitStatus(t, f.store, th.ID, db.ThreadReady)
	assert.Equal(t, 1, got.CommitsAhead, "stopping does not hide the commits it made")
}

func TestAThreadMergedFromTheOverviewRestsIdleAfterEveryPath(t *testing.T) {
	f := newFixture(t, commitOnce(t, false))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Work", Task: "commit"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadReady)
	// As the Merge dialog leaves it: merged and resolved, commits still ahead
	// of the local base (the merge was a squash on GitHub).
	_, err = f.threads.update(th.ID, func(t *db.Thread) { now := time.Now(); t.MergedAt = &now })
	require.NoError(t, err)
	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)

	reopened, err := f.threads.Reopen(th.ID, Source{})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, reopened.Status, "reopened")

	_, err = f.threads.Send(context.Background(), th.ID, "one more look", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	got := waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Equal(t, 1, got.CommitsAhead, "a turn's end agrees with Reopen: merged work is not waiting for review")
}

func TestMergeFromTheOverviewClosesTheThreadLikeResolve(t *testing.T) {
	f, th, _, _ := mergeFixture(t)
	var mu sync.Mutex
	var revoked []string
	f.threads.RevokeTools = func(id string) { mu.Lock(); revoked = append(revoked, id); mu.Unlock() }

	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadResolved, got.Status)
	assert.NotNil(t, got.ResolvedAt)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{th.ID}, revoked, "a merged thread's tool URL goes, as on every close")
	assert.NoDirExists(t, f.threads.layout.TmpDir(th.ProjectID, th.ID))
}

func TestAutoResolveOnlyTidiesAwayFinishedThreads(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { return nil })
	idle := func(title string, mut func(*db.Thread)) *db.Thread {
		th := &db.Thread{ProjectID: f.project.ID, ParentID: f.project.MainThreadID, Agent: db.AgentClaudeCode, Title: title, Status: db.ThreadIdle}
		if mut != nil {
			mut(th)
		}
		require.NoError(t, f.store.CreateThread(th))
		return th
	}
	done := idle("answered a question", nil)
	withPR := idle("opened a PR", nil)
	require.NoError(t, f.store.UpdateThreadLinks(withPR.ID, func(ls []db.ThreadLink) []db.ThreadLink {
		return append(ls, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 7, Role: db.LinkOpened, State: db.LinkStateOpen})
	}))
	// Its own worktree, with a change never committed; the last turn's
	// figures said clean, the worktree says otherwise.
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, f.repo, "git", "worktree", "add", "-q", "-b", "tp/dirty", wt, "main")
	require.NoError(t, os.WriteFile(filepath.Join(wt, "draft.txt"), []byte("wip\n"), 0o644))
	dirty := idle("left changes", func(th *db.Thread) {
		th.RepoPath, th.Cwd, th.Branch, th.BaseBranch = f.repo, wt, "tp/dirty", "main"
	})

	f.threads.AutoResolve(-time.Hour)

	for _, c := range []struct {
		th   *db.Thread
		want string
	}{{done, db.ThreadResolved}, {withPR, db.ThreadIdle}, {dirty, db.ThreadIdle}} {
		got, err := f.store.GetThread(c.th.ID)
		require.NoError(t, err)
		assert.Equal(t, c.want, got.Status, c.th.Title)
	}
}

func TestRecoverDoesNotLeaveADeadQuestionAsTheWaitingReason(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { return nil })
	mk := func(reason string) *db.Thread {
		th := &db.Thread{ProjectID: f.project.ID, ParentID: f.project.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadWaiting, WaitReason: reason}
		require.NoError(t, f.store.CreateThread(th))
		return th
	}
	asked := mk("Approve: Bash rm -rf build")
	require.NoError(t, f.store.AddThreadEvent(&db.ThreadEvent{ThreadID: asked.ID, Kind: db.EvApproval, Content: "Bash", Payload: `{"request_id":"r1","state":"pending"}`}))
	needsInput := mk("Which of the two layouts?")

	f.threads.Recover()

	got, err := f.store.GetThread(asked.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWaiting, got.Status, "it still needs the user: a message continues it")
	assert.Equal(t, expiredReason, got.WaitReason)
	got, err = f.store.GetThread(needsInput.ID)
	require.NoError(t, err)
	assert.Equal(t, "Which of the two layouts?", got.WaitReason, "a NEEDS_INPUT question survives a restart")
}
