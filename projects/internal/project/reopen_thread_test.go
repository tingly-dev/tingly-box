package project

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func (f *stopFixture) call(t *testing.T, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := f.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var text []string
	for _, c := range res.Content {
		text = append(text, c.(*mcp.TextContent).Text)
	}
	return strings.Join(text, "\n"), res.IsError
}

func (f *stopFixture) status(t *testing.T, id string) string {
	t.Helper()
	th, err := f.store.GetThread(id)
	require.NoError(t, err)
	return th.Status
}

func TestReopenThreadBringsAResolvedThreadBack(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "done")
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	_, err := f.svc.Threads.Resolve(th.ID)
	require.NoError(t, err)

	out, isErr := f.call(t, "message_thread", map[string]any{"thread_id": th.ID[:8], "message": "one more thing"})
	assert.True(t, isErr)
	assert.Contains(t, out, "call reopen_thread first")

	out, isErr = f.call(t, "reopen_thread", map[string]any{"thread_id": th.ID[:8], "cite": []string{f.user.ID[:8]}})
	require.False(t, isErr, out)
	assert.Equal(t, "reopened; the thread is now idle", out)
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Nil(t, got.ResolvedAt)
	notes := strings.Join(f.notes(t, th.ID), "\n")
	assert.Contains(t, notes, "Reopened by the coordinator.")
	assert.Contains(t, notes, "Stop the export thread, please.", "the user's words are recorded in the thread")

	out, isErr = f.call(t, "message_thread", map[string]any{"thread_id": th.ID[:8], "message": "one more thing"})
	assert.False(t, isErr, out)

	out, isErr = f.call(t, "reopen_thread", map[string]any{"thread_id": th.ID[:8]})
	assert.False(t, isErr, out)
	assert.Contains(t, out, "already open")
}

func TestReopenThreadLeavesAnOpenThreadAlone(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "run")
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	out, isErr := f.call(t, "reopen_thread", map[string]any{"thread_id": th.ID[:8]})
	assert.False(t, isErr, out)
	assert.Equal(t, "already open; the thread is working", out)
	assert.Equal(t, db.ThreadWorking, f.status(t, th.ID))
	assert.Empty(t, f.notes(t, th.ID))
}

func TestReopenThreadRefusesAForgedCite(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "done")
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	_, err := f.svc.Threads.Resolve(th.ID)
	require.NoError(t, err)
	out, isErr := f.call(t, "reopen_thread", map[string]any{"thread_id": th.ID[:8], "cite": []string{"nope"}})
	assert.True(t, isErr)
	assert.Contains(t, out, "not one of the user's own recent messages")
	assert.Equal(t, db.ThreadResolved, f.status(t, th.ID))
}

// An archived thread is the user's to manage: no tool of the coordinator finds
// it, lists it or shows it in the thread table.
func TestTheCoordinatorCannotSeeAnArchivedThread(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "done")
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	other := f.start(t, "done")
	waitStatus(t, f.store, other.ID, db.ThreadIdle)
	_, err := f.svc.Threads.ArchiveSession(th.ID)
	require.NoError(t, err)

	id, cite := th.ID[:8], []string{f.user.ID[:8]}
	calls := map[string]map[string]any{
		"get_thread":     {"thread_id": id},
		"message_thread": {"thread_id": id, "message": "hi"},
		"stop_thread":    {"thread_id": id, "cite": cite},
		"resolve_thread": {"thread_id": id, "cite": cite},
		"reopen_thread":  {"thread_id": id, "cite": cite},
		"rename_thread":  {"thread_id": id, "title": "New title"},
	}
	for name, args := range calls {
		out, isErr := f.call(t, name, args)
		assert.True(t, isErr, name)
		assert.Contains(t, out, "no thread", name)
	}
	out, _ := f.call(t, "list_threads", map[string]any{})
	assert.NotContains(t, out, threadName(th))
	assert.Contains(t, out, threadName(other))

	all, err := f.store.ListThreads(f.project.ID)
	require.NoError(t, err)
	prompt := coordinatorSystemPrompt(f.svc.Layout, f.project, all, th.CreatedAt)
	assert.NotContains(t, prompt, threadName(th))
	assert.Contains(t, prompt, threadName(other))
}

// handoff is a thread that rests with its branch pushed, as after a merge:
// origin has the branch and a merged PR of it is recorded.
func (f *stopFixture) handoff(t *testing.T) *db.Thread {
	t.Helper()
	th, err := f.svc.Threads.Start(context.Background(), f.project, StartSpec{Task: "done"})
	require.NoError(t, err)
	th = waitStatus(t, f.store, th.ID, db.ThreadIdle)
	require.NotEmpty(t, th.Branch)
	remote := t.TempDir()
	run(t, remote, "git", "init", "-q", "--bare", "-b", "main")
	run(t, f.repo, "git", "remote", "add", "origin", remote)
	run(t, f.repo, "git", "push", "-q", "origin", th.Branch)
	run(t, f.repo, "git", "fetch", "-q", "origin")
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func([]db.ThreadLink) []db.ThreadLink {
		return []db.ThreadLink{{Kind: db.LinkPR, Number: 7, State: db.LinkStateMerged, Head: th.Branch}}
	}))
	return th
}

func TestResolveThreadNeedsTheUsersWordsOrAFinishedHandoff(t *testing.T) {
	resolve := func(t *testing.T, mutate func(f *stopFixture, th *db.Thread)) (string, bool, string) {
		f := newStopFixture(t)
		th := f.handoff(t)
		if mutate != nil {
			mutate(f, th)
		}
		out, isErr := f.call(t, "resolve_thread", map[string]any{"thread_id": th.ID[:8]})
		return out, isErr, f.status(t, th.ID)
	}

	out, isErr, status := resolve(t, nil)
	assert.False(t, isErr, out)
	assert.Equal(t, "resolved", out)
	assert.Equal(t, db.ThreadResolved, status)

	refused := map[string]struct {
		mutate func(f *stopFixture, th *db.Thread)
		gap    string
	}{
		"no merged PR": {func(f *stopFixture, th *db.Thread) {
			require.NoError(t, f.store.UpdateThreadLinks(th.ID, func([]db.ThreadLink) []db.ThreadLink { return nil }))
		}, "no merged PR of this thread's branch is recorded"},
		"a merged PR of another branch": {func(f *stopFixture, th *db.Thread) {
			require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink { l[0].Head = "tp/other"; return l }))
		}, "no merged PR of this thread's branch is recorded"},
		"another PR still open": {func(f *stopFixture, th *db.Thread) {
			require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink {
				return append(l, db.ThreadLink{Kind: db.LinkPR, Number: 8, State: db.LinkStateOpen, Head: "tp/x"})
			}))
		}, "PR #8 is still open"},
		"a commit after the merge": {func(f *stopFixture, th *db.Thread) {
			run(t, th.Cwd, "git", "commit", "-q", "--allow-empty", "-m", "later")
		}, "1 commit after the merge"},
		"the branch is not on origin": {func(f *stopFixture, th *db.Thread) {
			run(t, f.repo, "git", "update-ref", "-d", "refs/remotes/origin/"+th.Branch)
		}, "the commits after the merge cannot be read"},
		"uncommitted changes": {func(f *stopFixture, th *db.Thread) {
			th.Uncommitted = true
			require.NoError(t, f.store.SaveThread(th))
		}, "uncommitted changes"},
		"waiting on the user": {func(f *stopFixture, th *db.Thread) {
			th.Status, th.WaitReason = db.ThreadWaiting, "Approve: Bash"
			require.NoError(t, f.store.SaveThread(th))
		}, "the thread is waiting, not resting"},
	}
	for name, c := range refused {
		t.Run(name, func(t *testing.T) {
			out, isErr, status := resolve(t, c.mutate)
			assert.True(t, isErr, out)
			assert.Contains(t, out, c.gap)
			assert.Contains(t, out, "cite", "the answer says how to proceed")
			assert.NotEqual(t, db.ThreadResolved, status)
		})
	}
}

func TestResolveThreadWithCiteStopsARunningTurn(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "run")
	waitStatus(t, f.store, th.ID, db.ThreadWorking)

	out, isErr := f.call(t, "resolve_thread", map[string]any{"thread_id": th.ID[:8]})
	assert.True(t, isErr)
	assert.Contains(t, out, "running")
	assert.Equal(t, db.ThreadWorking, f.status(t, th.ID))

	out, isErr = f.call(t, "resolve_thread", map[string]any{"thread_id": th.ID[:8], "cite": []string{"nope"}})
	assert.True(t, isErr)
	assert.Contains(t, out, "not one of the user's own recent messages")
	assert.Equal(t, db.ThreadWorking, f.status(t, th.ID))

	out, isErr = f.call(t, "resolve_thread", map[string]any{"thread_id": th.ID[:8], "cite": []string{f.user.ID[:8]}})
	require.False(t, isErr, out)
	assert.Equal(t, "resolved; its running turn was stopped", out)
	assert.Equal(t, db.ThreadResolved, f.status(t, th.ID))
}

// The tool and the service end in the same state and the same note, apart
// from who is named.
func TestReopenThreadMatchesTheServiceReopen(t *testing.T) {
	f := newStopFixture(t)
	viaTool, viaService := f.start(t, "done"), f.start(t, "done")
	for _, th := range []*db.Thread{viaTool, viaService} {
		waitStatus(t, f.store, th.ID, db.ThreadIdle)
		_, err := f.svc.Threads.Resolve(th.ID)
		require.NoError(t, err)
	}
	_, isErr := f.call(t, "reopen_thread", map[string]any{"thread_id": viaTool.ID[:8]})
	require.False(t, isErr)
	_, err := f.svc.Threads.Reopen(viaService.ID, Source{})
	require.NoError(t, err)

	a, err := f.store.GetThread(viaTool.ID)
	require.NoError(t, err)
	b, err := f.store.GetThread(viaService.ID)
	require.NoError(t, err)
	assert.Equal(t, b.Status, a.Status)
	assert.Equal(t, b.ResolvedAt, a.ResolvedAt)
	assert.Equal(t, []string{"Reopened by the coordinator."}, f.notes(t, viaTool.ID))
	assert.Equal(t, []string{"Reopened."}, f.notes(t, viaService.ID))
}
