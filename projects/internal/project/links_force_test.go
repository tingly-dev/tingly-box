package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Pushing is open to every local branch of the thread's checkout and a rebased
// one may be force-pushed with a lease, but never another thread's branch. See
// .design/github-tools.md ("Which branches a thread may push").

// amendOwn rewrites the tip of the thread's own branch, so origin's copy (once
// pushed) is no ancestor of it.
func (f *linksFixture) amendOwn(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "a.txt"), []byte("rewritten\n"), 0o644))
	run(t, f.repo, "git", "commit", "-qa", "--amend", "-m", "work, rebased")
}

func TestForceWithLeaseUpdatesTheThreadsOwnPushedBranch(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x"})
	require.False(t, isErr, out)
	f.amendOwn(t)

	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x"})
	require.True(t, isErr, "a plain push of a rewritten branch is still refused")
	assert.Contains(t, out, "force")

	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already open for branch tp/work")
	assert.Equal(t, run(t, f.repo, "git", "rev-parse", "tp/work"), run(t, f.remote, "git", "rev-parse", "tp/work"), "origin has the rewritten branch")
}

func TestForceWithLeaseRefusesWhenOriginMovedSinceThisCheckoutSawIt(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x"})
	require.False(t, isErr)
	// Someone else pushes a commit on top of the branch.
	other := t.TempDir()
	run(t, other, "git", "clone", "-q", "-b", "tp/work", f.remote, ".")
	run(t, other, "git", "commit", "-q", "--allow-empty", "-m", "someone else's work")
	run(t, other, "git", "push", "-q", "origin", "tp/work")
	theirs := run(t, f.remote, "git", "rev-parse", "tp/work")
	f.amendOwn(t)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "moved")
	assert.Equal(t, theirs, run(t, f.remote, "git", "rev-parse", "tp/work"), "their commit is not overwritten")
}

func TestForceWithLeaseNeedsABranchThatWasNeverOnOrigin(t *testing.T) {
	f := newLinksFixture(t)
	// origin has a tp/work this checkout never saw: the lease expects no such branch.
	other := newRepo(t)
	run(t, other, "git", "remote", "add", "origin", f.remote)
	run(t, other, "git", "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, other, "git", "push", "-q", "origin", "HEAD:refs/heads/tp/work")
	theirs := run(t, f.remote, "git", "rev-parse", "tp/work")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.True(t, isErr, out)
	assert.Equal(t, theirs, run(t, f.remote, "git", "rev-parse", "tp/work"))
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
}

func TestForceIsOnlyForBranchesTheThreadCreatedOrOpenedAPRFrom(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/side", "main")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/side", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "force")
	assert.Empty(t, f.remoteBranches(t)[1:], "nothing was pushed")

	// Once the thread opened a PR from it the branch is the thread's to force.
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "side", "head": "feat/side"})
	require.False(t, isErr, out)
	run(t, f.repo, "git", "checkout", "-q", "feat/side")
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "side.txt"), []byte("rewritten\n"), 0o644))
	run(t, f.repo, "git", "commit", "-qa", "--amend", "-m", "side, rebased")
	run(t, f.repo, "git", "checkout", "-q", "tp/work")
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "side", "head": "feat/side", "force": true})
	require.False(t, isErr, out)
	assert.Equal(t, run(t, f.repo, "git", "rev-parse", "feat/side"), run(t, f.remote, "git", "rev-parse", "feat/side"))
}

func TestForceNeverReachesTheDefaultOrABaseBranch(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	before := run(t, f.remote, "git", "rev-parse", "main")
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "main", "base": "tp/work", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "never pushed to")
	assert.Equal(t, before, run(t, f.remote, "git", "rev-parse", "main"))
}

// A branch another thread works on is a local branch of the same repository,
// but it is not this thread's to push.
func TestAnotherThreadsBranchIsNotPushed(t *testing.T) {
	for _, c := range []struct {
		name  string
		other func(f *linksFixture) *db.Thread
	}{
		{"its own branch", func(f *linksFixture) *db.Thread {
			return &db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/theirs", BaseBranch: "main", RepoPath: f.repo}
		}},
		{"a branch of its stack", func(f *linksFixture) *db.Thread {
			return &db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/elsewhere", BaseBranch: "main", RepoPath: f.repo,
				Links: []db.ThreadLink{{Kind: db.LinkPR, Repo: "acme/widgets", Number: 99, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/theirs", Base: "main"}}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLinksFixture(t)
			f.stackBranch(t, "tp/theirs", "main")
			require.NoError(t, f.store.CreateThread(c.other(f)))
			cs, _ := f.client(t, f.worker.ID)

			for _, force := range []bool{false, true} {
				out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/theirs", "force": force})
				require.True(t, isErr, out)
				assert.Contains(t, out, "another thread")
			}
			assert.Equal(t, []string{"main"}, f.remoteBranches(t))
			for _, call := range f.gh.called() {
				assert.NotContains(t, call, "create-pr")
			}
		})
	}
}

func TestAThreadMayPushABranchItsOwnRecordsHold(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "tp/shared", "main")
	// Another thread also holds a PR from the branch, but this thread opened one too.
	require.NoError(t, f.store.CreateThread(&db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/elsewhere", RepoPath: f.repo,
		Links: []db.ThreadLink{{Kind: db.LinkPR, Repo: "acme/widgets", Number: 99, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/shared", Base: "main"}}}))
	_, err := f.store.UpsertThreadLink(f.worker.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 98, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/shared", Base: "main"})
	require.NoError(t, err)
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/shared"})
	require.False(t, isErr, out)
}
