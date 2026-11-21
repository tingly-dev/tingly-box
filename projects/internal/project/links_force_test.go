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
// own branch may be force-pushed with a lease. Everything beyond that (another
// thread's branch, the default or a protected branch, a force on a branch the
// thread did not create, overwriting commits it has not seen, a PR it did not
// open) is put to the user as an approval, never refused outright. See
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
	assert.Empty(t, f.approvals, "a lease on the thread's own branch needs no one's say")
}

// pushBehind makes origin's tp/work carry a commit this checkout has not seen,
// and the thread's own branch a rewritten one; it returns origin's commit.
func (f *linksFixture) pushBehind(t *testing.T) string {
	t.Helper()
	other := t.TempDir()
	run(t, other, "git", "clone", "-q", "-b", "tp/work", f.remote, ".")
	run(t, other, "git", "commit", "-q", "--allow-empty", "-m", "someone else's work")
	run(t, other, "git", "push", "-q", "origin", "tp/work")
	f.amendOwn(t)
	return run(t, f.remote, "git", "rev-parse", "tp/work")
}

// A lease that does not hold is an error, not a card: the model fetches,
// rebases and calls again.
func TestForceOverCommitsNotSeenIsAnErrorAndAsksNoOne(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x"})
	require.False(t, isErr)
	theirs := f.pushBehind(t)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "origin/tp/work is at "+theirs[:10], "the error names the commit that would be lost")
	assert.Contains(t, out, "git fetch origin")
	assert.Contains(t, out, "git said:")
	assert.Equal(t, theirs, run(t, f.remote, "git", "rev-parse", "tp/work"), "their commit is not overwritten")
	assert.Empty(t, f.approvals)

	// After a fetch and a rebase onto it the same call goes through.
	run(t, f.repo, "git", "fetch", "-q", "origin")
	run(t, f.repo, "git", "reset", "-q", "--soft", "origin/tp/work") // the rewritten work, on top of theirs
	run(t, f.repo, "git", "commit", "-qam", "work, rebased onto theirs")
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.False(t, isErr, out)
	assert.Equal(t, run(t, f.repo, "git", "rev-parse", "tp/work"), run(t, f.remote, "git", "rev-parse", "tp/work"))
}

func TestForceOnABranchOriginHasAndThisCheckoutNeverSawIsAnError(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	other := newRepo(t)
	run(t, other, "git", "remote", "add", "origin", f.remote)
	run(t, other, "git", "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, other, "git", "push", "-q", "origin", "HEAD:refs/heads/tp/work")
	theirs := run(t, f.remote, "git", "rev-parse", "tp/work")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "never saw the branch")
	assert.Contains(t, out, "origin/tp/work is at "+theirs[:10])
	assert.Equal(t, theirs, run(t, f.remote, "git", "rev-parse", "tp/work"))
	assert.Empty(t, f.approvals)
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
}

func TestForceOnABranchTheThreadDidNotCreateAsksUntilItHasAPR(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/side", "main")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/side", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "did not approve")
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Reason, "neither created nor opened a PR from")
	assert.Equal(t, []string{"main"}, f.remoteBranches(t), "nothing was pushed")

	// Allowed once, the branch is pushed and has a PR: the thread opened it.
	f.allowAll()
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "side", "head": "feat/side", "force": true})
	require.False(t, isErr, out)
	assert.Len(t, f.approvals, 2)

	// Rewritten and pushed again: its own now, no more asking.
	run(t, f.repo, "git", "checkout", "-q", "feat/side")
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "side.txt"), []byte("rewritten\n"), 0o644))
	run(t, f.repo, "git", "commit", "-qa", "--amend", "-m", "side, rebased")
	run(t, f.repo, "git", "checkout", "-q", "tp/work")
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "side", "head": "feat/side", "force": true})
	require.False(t, isErr, out)
	assert.Len(t, f.approvals, 2, "the PR it opened makes the branch its own")
	assert.Equal(t, run(t, f.repo, "git", "rev-parse", "feat/side"), run(t, f.remote, "git", "rev-parse", "feat/side"))
}

func TestTheDefaultBranchAsAHeadIsTheUsersToAllow(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	f.gh.def = "feat/other"
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/other", "base": "main"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "did not approve")
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Reason, "default branch")
	assert.Equal(t, []string{"main"}, f.remoteBranches(t))

	f.allowAll()
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/other", "base": "main"})
	require.False(t, isErr, out)
	assert.True(t, f.remoteHas(t, "feat/other"))
}

func TestAProtectedHeadIsAskedAboutAndAnImpossibleOneStillRefused(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	f.gh.protected = map[string]bool{"feat/other": true}
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/other"})
	require.False(t, isErr, out)
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Reason, "protected on GitHub")

	// A PR cannot come from its own base: no one is asked, there is nothing to allow.
	f.approvals = nil
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "base": "tp/work"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "base of its own PR")
	assert.Empty(t, f.approvals)
}

// A branch another thread works on is a local branch of the same repository,
// but it is not this thread's to push without the user's say.
func TestAnotherThreadsBranchAsksTheUser(t *testing.T) {
	for _, c := range []struct {
		name  string
		other func(f *linksFixture) *db.Thread
		link  db.ThreadLink
	}{
		{"its own branch", func(f *linksFixture) *db.Thread {
			return &db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/theirs", BaseBranch: "main", RepoPath: f.repo}
		}, db.ThreadLink{}},
		{"a branch of its stack", func(f *linksFixture) *db.Thread {
			return &db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/elsewhere", BaseBranch: "main", RepoPath: f.repo}
		}, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 99, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/theirs", Base: "main"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLinksFixture(t)
			f.stackBranch(t, "tp/theirs", "main")
			other := c.other(f)
			require.NoError(t, f.store.CreateThread(other))
			if c.link.Number != 0 {
				_, err := f.store.UpsertThreadLink(other.ID, c.link)
				require.NoError(t, err)
			}
			cs, _ := f.client(t, f.worker.ID)

			out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/theirs"})
			require.True(t, isErr, out)
			assert.Contains(t, out, "did not approve")
			require.Len(t, f.approvals, 1)
			assert.Contains(t, f.approvals[0].Reason, `belongs to another thread ("Other")`)
			assert.Equal(t, "mcp__tingly-projects__create_pr", f.approvals[0].Tool)
			assert.Equal(t, []string{"main"}, f.remoteBranches(t))
			for _, call := range f.gh.called() {
				assert.NotContains(t, call, "create-pr")
			}

			// With force too, one card says both.
			f.approvals = nil
			_, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/theirs", "force": true})
			require.True(t, isErr)
			require.Len(t, f.approvals, 1)
			assert.Contains(t, f.approvals[0].Reason, "belongs to another thread")
			assert.Contains(t, f.approvals[0].Reason, "force push")

			f.allowAll()
			out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/theirs"})
			require.False(t, isErr, out)
			assert.True(t, f.remoteHas(t, "tp/theirs"))
		})
	}
}

func TestAThreadMayPushABranchItsOwnRecordsHold(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "tp/shared", "main")
	// Another thread also holds a PR from the branch, but this thread opened one too.
	other := &db.Thread{ProjectID: f.p.ID, Agent: db.AgentClaudeCode, Title: "Other", Branch: "tp/elsewhere", RepoPath: f.repo}
	require.NoError(t, f.store.CreateThread(other))
	_, err := f.store.UpsertThreadLink(other.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 99, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/shared", Base: "main"})
	require.NoError(t, err)
	_, err = f.store.UpsertThreadLink(f.worker.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 98, Role: db.LinkOpened, State: db.LinkStateOpen, Head: "tp/shared", Base: "main"})
	require.NoError(t, err)
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "tp/shared"})
	require.False(t, isErr, out)
	assert.Empty(t, f.approvals)
}

func TestNobodyToAskIsAnErrorNotAPush(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/side", "main")
	f.links.approve = nil
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/side", "force": true})
	require.True(t, isErr, out)
	assert.Contains(t, out, "no way to ask")
	assert.Equal(t, []string{"main"}, f.remoteBranches(t))
}

func TestUpdateOfAPRTheThreadDidNotOpenAsksTheUserWithTheChange(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	theirs := f.gh.add(db.LinkPR, "Theirs", func(it *GitHubItem) { it.Head, it.Base = "feat/x", "main" })

	out, isErr := f.call(t, cs, "update", map[string]any{"number": theirs.Number, "title": "Better", "base": "tp/work"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "did not approve")
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Summary, "title, base")
	assert.Equal(t, "Better", f.approvals[0].Input["title"])
	assert.Empty(t, f.updates(), "nothing is edited before the user allows it")
	links, _ := f.store.ThreadLinks(f.worker.ID)
	assert.Empty(t, links)

	f.allowAll()
	out, isErr = f.call(t, cs, "update", map[string]any{"number": theirs.Number, "title": "Better"})
	require.False(t, isErr, out)
	assert.Contains(t, f.updates(), "update-pr acme/widgets #"+itoa(theirs.Number)+" title=Better")
	link, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", theirs.Number)
	require.NoError(t, err)
	assert.Equal(t, db.LinkCommented, link.Role, "recorded as touched, not as opened: the next edit asks again")
}
