package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The thread lifecycle of .design/thread-lifecycle.md: one rule for where a
// thread rests, a merge that never closes the thread, and no waiting reason
// that names a dead question. Nothing but the user closes a thread.

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

// commitEachTurn commits a new file on every turn.
func commitEachTurn(t *testing.T) func(turn *enginetest.Turn) error {
	n := 0
	return func(turn *enginetest.Turn) error {
		n++
		dir := turn.Opts.ProjectPath
		name := fmt.Sprintf("step%d.txt", n)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644))
		run(t, dir, "git", "add", ".")
		run(t, dir, "git", "commit", "-qm", name)
		turn.Say("Committed "+name+".", 1, 1)
		return nil
	}
}

func TestAMergeLeavesTheThreadOpenAndItsLaterWorkIsMergedAgain(t *testing.T) {
	f, th, gh, _ := mergeFixtureWith(t, commitEachTurn(t))
	var revoked []string
	f.threads.RevokeTools = func(id string) { revoked = append(revoked, id) }

	merged, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, merged.Status, "merged work is not waiting for review")
	assert.Nil(t, merged.ResolvedAt, "a merge never closes the thread")
	assert.NotNil(t, merged.MergedAt)
	assert.Empty(t, revoked, "the thread keeps its tools: it can be asked for more")

	// More work on the same branch: it is not merged, and reads so.
	_, err = f.threads.Send(context.Background(), th.ID, "one more step", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	more := waitStatus(t, f.store, th.ID, db.ThreadReady)
	assert.Nil(t, more.MergedAt, "commits after the merge are not merged")
	assert.Equal(t, 2, more.CommitsAhead)

	// Merging again lands them: a new PR, not "already merged".
	before := len(gh.called())
	again, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	calls := strings.Join(gh.called()[before:], "\n")
	assert.Contains(t, calls, "create-pr", "the later commits get their own PR")
	assert.Contains(t, calls, "squash-pr")
	assert.NotNil(t, again.MergedAt)
	assert.Equal(t, db.ThreadIdle, again.Status)
}

func TestAMergeLeavesAThreadThatWaitsOnTheUserWaiting(t *testing.T) {
	f, th, _, _ := mergeFixture(t)
	_, err := f.threads.update(th.ID, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWaiting, "Which name?" })
	require.NoError(t, err)
	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWaiting, got.Status, "the question is still open")
	assert.NotNil(t, got.MergedAt)
}
