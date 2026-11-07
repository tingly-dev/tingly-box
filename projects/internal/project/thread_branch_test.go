package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func idleScript(turn *enginetest.Turn) error {
	turn.Say("ok", 1, 1)
	return nil
}

func TestStartThreadFromAnotherBaseAndBranchName(t *testing.T) {
	f := newFixture(t, idleScript)
	run(t, f.repo, "git", "branch", "release/1.2")
	run(t, f.repo, "git", "checkout", "-q", "release/1.2")
	run(t, f.repo, "git", "commit", "-q", "--allow-empty", "-m", "release only")
	run(t, f.repo, "git", "checkout", "-q", "main")

	th, err := f.threads.Start(context.Background(), f.project, StartSpec{
		Title: "Backport fix", Task: "backport", Base: "release/1.2", Branch: "fix/backport", Origin: "coordinator",
	})
	require.NoError(t, err)
	assert.Equal(t, "fix/backport", th.Branch)
	assert.Equal(t, "release/1.2", th.BaseBranch)
	assert.Equal(t, run(t, f.repo, "git", "rev-parse", "release/1.2"), run(t, th.Cwd, "git", "rev-parse", "HEAD"),
		"the worktree starts at the chosen base, not at main")
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	def, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Plain", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	assert.Equal(t, "main", def.BaseBranch, "without base the repository's base branch is used")
	assert.Contains(t, def.Branch, "tp/plain-")
	waitStatus(t, f.store, def.ID, db.ThreadIdle)
}

func TestStartThreadRefusesBadBaseOrBranchBeforeCreatingAnything(t *testing.T) {
	f := newFixture(t, idleScript)
	run(t, f.repo, "git", "branch", "keep-me")
	cases := []struct {
		name string
		spec StartSpec
		want string
	}{
		{"missing base", StartSpec{Base: "nope"}, `base "nope" is not a local branch`},
		{"taken branch", StartSpec{Branch: "keep-me"}, `branch "keep-me" already exists`},
		{"invalid branch", StartSpec{Branch: "a..b"}, "not a valid branch name"},
		{"option-like branch", StartSpec{Branch: "-D"}, "not a valid branch name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.spec.Title, c.spec.Task, c.spec.Origin = "T", "task", "coordinator"
			_, err := f.threads.Start(context.Background(), f.project, c.spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
	assert.Empty(t, threadsOf(t, f), "a refused start leaves no thread behind")
	assert.True(t, BranchExists(context.Background(), f.repo, "keep-me"), "the user's branch is never deleted")
}

func TestStartThreadBaseNeedsItsOwnWorktree(t *testing.T) {
	f := newFixture(t, idleScript)
	f.project.DisableWorktree = true
	require.NoError(t, f.store.SaveProject(f.project))
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "T", Task: "x", Base: "main", Origin: "coordinator"})
	assert.ErrorIs(t, err, ErrNoOwnBranch)

	none, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "T", Task: "x", Repo: "none", Branch: "b", Origin: "coordinator"})
	assert.ErrorIs(t, err, ErrNoOwnBranch)
	assert.Nil(t, none)
}

func TestAddWorktreeNeverDeletesAnExistingBranch(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "git", "branch", "precious")
	err := AddWorktree(context.Background(), repo, t.TempDir()+"/wt", "precious", "main")
	require.Error(t, err)
	assert.True(t, BranchExists(context.Background(), repo, "precious"))
}

func TestCoordinatorStartsThreadFromChosenBase(t *testing.T) {
	var started, refused string
	var refusedErr bool
	f, svc := newServiceFixture(t, testHost{}, idleScript, func(c *coordtest.Turn) error {
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		refused, refusedErr = c.Call("start_thread", map[string]any{"title": "A", "task": "a", "base": "nope"})
		started, _ = c.Call("start_thread", map[string]any{"title": "B", "task": "b", "base": "release", "branch": "work/b"})
		c.Call("propose_threads", map[string]any{"threads": []map[string]string{{"title": "C", "task": "c", "base": "release", "branch": "work/c"}}})
		c.Reply("done")
		return nil
	})
	run(t, f.repo, "git", "branch", "release")
	_, err := svc.Coordinator.Post(f.project.ID, "go", nil, Source{})
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvSuggestions) == 1 })

	assert.True(t, refusedErr)
	assert.Contains(t, refused, `base "nope" is not a local branch`)
	assert.Contains(t, started, "branch work/b, base release")

	var sug db.ThreadEvent
	for _, m := range msgs {
		if m.Kind == db.EvSuggestions {
			sug = m
		}
	}
	_, err = svc.Coordinator.StartSuggestions(context.Background(), f.project.MainThreadID, sug.ID, nil)
	require.NoError(t, err)
	byTitle := map[string]db.Thread{}
	for _, th := range threadsOf(t, f) {
		byTitle[th.Title] = th
	}
	require.Len(t, byTitle, 2, "the refused start created nothing")
	assert.Equal(t, "work/b", byTitle["B"].Branch)
	assert.Equal(t, "release", byTitle["C"].BaseBranch)
	assert.Equal(t, "work/c", byTitle["C"].Branch)
}
