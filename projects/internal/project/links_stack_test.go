package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A thread may work on several branches (a stack of PRs): create_pr takes a
// head, and the thread's links hold every PR. See .design/github-tools.md.

func TestCreatePRWithHeadOpensAPRForAnotherLocalBranch(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Other", "head": "feat/other"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "feat/other → main", "an other head targets the repository's default branch, not the thread's base")
	assert.Contains(t, f.gh.called(), `create-pr acme/widgets "Other" head=feat/other base=main draft=false`)
	assert.Equal(t, []string{"feat/other", "main"}, f.remoteBranches(t), "only the head was pushed")
	assert.False(t, f.remoteHas(t, "tp/work"), "the thread's own branch was not asked for")

	links, _ := f.store.ThreadLinks(f.worker.ID)
	require.Len(t, links, 1)
	assert.Equal(t, "feat/other", links[0].Head)
	assert.Equal(t, "main", links[0].Base)
}

func TestStackedPRsEachGetARecordAndABaseCanBeRetargeted(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "tp/top", "tp/work")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Bottom"})
	require.False(t, isErr, out)
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Top", "head": "tp/top", "base": "tp/work"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "opened PR #12")
	assert.Contains(t, out, "tp/top → tp/work")
	assert.Contains(t, f.gh.called(), `create-pr acme/widgets "Top" head=tp/top base=tp/work draft=false`)

	links, err := f.store.ThreadLinks(f.worker.ID)
	require.NoError(t, err)
	require.Len(t, links, 2, "both PRs are recorded")
	assert.Equal(t, [2]string{"tp/work", "main"}, [2]string{links[0].Head, links[0].Base})
	assert.Equal(t, [2]string{"tp/top", "tp/work"}, [2]string{links[1].Head, links[1].Base})

	assert.Contains(t, linksDetail(links, *f.clock), "branch=tp/top→tp/work", "a stack's PRs are told apart by branch")

	// With two PRs a bare update asks which, and names them by branch.
	out, isErr = f.call(t, cs, "update", map[string]any{"title": "x"})
	assert.True(t, isErr)
	assert.Contains(t, out, "several PRs")
	assert.Contains(t, out, "#12 (tp/top → tp/work)")

	// The bottom PR merges; the top one is pointed back at main.
	f.gh.set(11, func(it *GitHubItem) { it.State = "merged" })
	out, isErr = f.call(t, cs, "update", map[string]any{"number": 12, "base": "main"})
	require.False(t, isErr, out)
	assert.Contains(t, f.gh.called(), "update-pr acme/widgets #12 base=main")
	top, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 12)
	require.NoError(t, err)
	assert.Equal(t, [2]string{"tp/top", "main"}, [2]string{top.Head, top.Base}, "the record follows the new base")
	bottom, _ := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	assert.Equal(t, db.LinkOpened, bottom.Role)

	// With the merged one out of the way a bare update means the open one.
	_, err = f.links.record(f.thread(t), GitHubRepo{Owner: "acme", Name: "widgets"}, db.LinkPR, db.LinkOpened, f.gh.items[11])
	require.NoError(t, err)
	out, isErr = f.call(t, cs, "update", map[string]any{"title": "Top, rebased"})
	require.False(t, isErr, out)
	assert.Contains(t, f.gh.called(), "update-pr acme/widgets #12 title=Top, rebased")
}

func TestCreatePRWithHeadIsIdempotentPerBranch(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	existing := f.gh.add(db.LinkPR, "Already", func(it *GitHubItem) { it.Head, it.Base = "feat/other", "main" })
	f.gh.existing = &existing
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Other", "head": "feat/other"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already open for branch feat/other")
	assert.Contains(t, f.gh.called(), "open-pr acme/widgets feat/other", "the lookup is for the head asked for")
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
	assert.True(t, f.remoteHas(t, "feat/other"))
}

func TestCreatePRWithHeadRefusals(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(f *linksFixture)
		args  map[string]any
		want  string
	}{
		{"head is the default branch", func(*linksFixture) {}, map[string]any{"head": "main", "base": "feat/other"}, "never pushed to"},
		{"head is the default branch whatever the base", func(f *linksFixture) { f.gh.def = "feat/other" }, map[string]any{"head": "feat/other", "base": "main"}, "never pushed to"},
		{"head is protected", func(f *linksFixture) { f.gh.protected = map[string]bool{"feat/other": true} }, map[string]any{"head": "feat/other"}, "protected"},
		{"head is not a local branch", func(*linksFixture) {}, map[string]any{"head": "feat/nope"}, "no local branch feat/nope"},
		{"head is a remote-only name", func(*linksFixture) {}, map[string]any{"head": "origin/main"}, "no local branch origin/main"},
		{"head is an option", func(*linksFixture) {}, map[string]any{"head": "--force"}, "not a valid branch name"},
		{"head is a refspec", func(*linksFixture) {}, map[string]any{"head": "feat/other:main"}, "not a valid branch name"},
		{"head is the base", func(*linksFixture) {}, map[string]any{"head": "feat/other", "base": "feat/other"}, "never pushed to"},
		{"base is not on GitHub", func(*linksFixture) {}, map[string]any{"head": "feat/other", "base": "feat/unpushed"}, "base branch feat/unpushed is not on GitHub"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLinksFixture(t)
			f.stackBranch(t, "feat/other", "main")
			f.stackBranch(t, "feat/unpushed", "main")
			c.setup(f)
			args := map[string]any{"title": "x"}
			for k, v := range c.args {
				args[k] = v
			}
			cs, _ := f.client(t, f.worker.ID)
			out, isErr := f.call(t, cs, "create_pr", args)
			assert.True(t, isErr, out)
			assert.Contains(t, out, c.want)
			assert.Equal(t, []string{"main"}, f.remoteBranches(t), "a refused call pushes nothing")
			for _, call := range f.gh.called() {
				assert.NotContains(t, call, "create-pr")
			}
			links, _ := f.store.ThreadLinks(f.worker.ID)
			assert.Empty(t, links)
		})
	}
}

// The push of an extra head is as plain as the thread's own: it can only
// fast-forward that one branch.
func TestCreatePRWithHeadNeverForcesAPush(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	// origin already has a feat/other this checkout does not contain.
	other := newRepo(t)
	run(t, other, "git", "remote", "add", "origin", f.remote)
	run(t, other, "git", "commit", "-q", "--allow-empty", "-m", "diverged")
	run(t, other, "git", "push", "-q", "origin", "HEAD:refs/heads/feat/other")
	before := run(t, f.remote, "git", "rev-parse", "feat/other")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/other"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "a force push is not available")
	assert.Equal(t, before, run(t, f.remote, "git", "rev-parse", "feat/other"))
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
}

func TestCreatePRWithHeadWorksFromAFolderSession(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "feat/other", "main")
	f.folder.Cwd, f.folder.Branch = f.repo, ""
	require.NoError(t, f.store.SaveThread(f.folder))
	cs, _ := f.client(t, f.folder.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x", "head": "feat/other"})
	require.False(t, isErr, out)
	assert.Contains(t, f.gh.called(), `create-pr acme/widgets "x" head=feat/other base=main draft=false`)
}

func TestGhCLIProtectedReadsTheBranchAndTreatsAMissingOneAsUnprotected(t *testing.T) {
	repo := GitHubRepo{Owner: "acme", Name: "widgets"}
	bin, logFile := fakeGH(t, `{"name":"release/1.0","protected":true}`, 0)
	got, err := ghCLI{bin: bin}.Protected(context.Background(), repo, "release/1.0")
	require.NoError(t, err)
	assert.True(t, got)
	b, _ := os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X GET repos/acme/widgets/branches/release/1.0")

	// A branch only this checkout has is not on GitHub: 404, not protected.
	dir := t.TempDir()
	notFound := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(notFound, []byte("#!/bin/sh\necho 'gh: Branch not found (HTTP 404)' >&2\nexit 1\n"), 0o755))
	got, err = ghCLI{bin: notFound}.Protected(context.Background(), repo, "feat/local")
	require.NoError(t, err)
	assert.False(t, got)

	// Any other failure is an error: an unreadable answer must not read as "not protected".
	bin, _ = fakeGH(t, "", 1)
	_, err = ghCLI{bin: bin}.Protected(context.Background(), repo, "feat/x")
	assert.Error(t, err)
}
