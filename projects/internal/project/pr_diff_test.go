package project

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestPRDiffOfAThreadsOpenPRIsReadLocally(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	addOpenPR(t, f, th, 12)
	d, err := f.threads.PRDiff(context.Background(), th.ID, 12)
	require.NoError(t, err)
	assert.Equal(t, "local", d.Source)
	assert.Equal(t, 12, d.Number)
	assert.Equal(t, th.Branch, d.Head)
	require.Len(t, d.Files, 1)
	assert.Equal(t, "feature.txt", d.Files[0].Path)
	assert.Empty(t, gh.called(), "GitHub is not asked when the checkout has the branches")
}

func TestPRDiffOfAFinishedOrUnknownPRIsRefused(t *testing.T) {
	f, th, _, _ := mergeFixture(t)
	addOpenPR(t, f, th, 12)
	_, err := f.threads.PRDiff(context.Background(), th.ID, 99)
	assert.ErrorContains(t, err, "not one of this thread's PRs")
	for _, state := range []string{db.LinkStateMerged, db.LinkStateClosed} {
		require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink { l[0].State = state; return l }))
		_, err = f.threads.PRDiff(context.Background(), th.ID, 12)
		assert.ErrorContains(t, err, "only an open PR's changes are shown", state)
		_, err = f.threads.PRDiffFile(context.Background(), th.ID, 12, "feature.txt", "")
		assert.Error(t, err, state)
	}
}

// With GitHub's read switched off (the default), a PR whose branches are not
// here says so and GitHub is not asked.
func TestPRDiffOfABranchNotHereIsRefusedWithoutAskingGitHub(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink {
		return append(l, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 12, State: db.LinkStateOpen, Head: "someone-elses", Base: "main", Role: db.LinkOpened})
	}))
	_, err := f.threads.PRDiff(context.Background(), th.ID, 12)
	assert.ErrorIs(t, err, errNoLocalRefs)
	_, err = f.threads.PRDiffFile(context.Background(), th.ID, 12, "a.go", "")
	assert.ErrorIs(t, err, errNoLocalRefs)
	assert.Empty(t, gh.called(), "GitHub is not asked")
}

func TestPRDiffOfABranchNotHereComesFromGitHubOnceWhenSwitchedOn(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	f.threads.remoteDiff = true
	gh.prFiles = map[int][]GitHubFile{12: {
		{Path: "a.go", Status: "modified", Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-a\n+b"},
		{Path: "b.go", Status: "added", Additions: 1, Patch: "@@ -0,0 +1 @@\n+c"},
	}}
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink {
		return append(l, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 12, State: db.LinkStateOpen, Head: "someone-elses", Base: "main", Role: db.LinkOpened})
	}))
	d, err := f.threads.PRDiff(context.Background(), th.ID, 12)
	require.NoError(t, err)
	assert.Equal(t, "github", d.Source)
	require.Len(t, d.Files, 2)
	fd, err := f.threads.PRDiffFile(context.Background(), th.ID, 12, "a.go", "")
	require.NoError(t, err)
	assert.Contains(t, fd.Patch, "+b")
	_, err = f.threads.PRDiffFile(context.Background(), th.ID, 12, "b.go", "")
	require.NoError(t, err)
	n := 0
	for _, c := range gh.called() {
		if strings.HasPrefix(c, "files ") {
			n++
		}
	}
	assert.Equal(t, 1, n, "the PR's files are asked for once while its files are opened one after the other")
}

func TestMergeDraftIsThePRsOwnWordsOrTheCommits(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	draft, err := f.threads.MergeDraft(context.Background(), th.ID)
	require.NoError(t, err)
	subject := run(t, f.repo, "git", "log", "-1", "--format=%s", th.Branch)
	assert.Equal(t, MergeDraft{Source: "commit", Title: subject}, draft, "one commit: its subject, as gh pr create --fill")

	for _, s := range []string{"second step", "third step"} {
		run(t, th.Cwd, "git", "commit", "-q", "--allow-empty", "-m", s)
	}
	draft, err = f.threads.MergeDraft(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Equal(t, "commits", draft.Source)
	assert.Equal(t, humanBranch(th.Branch), draft.Title)
	assert.Equal(t, fmt.Sprintf("- %s\n- second step\n- third step", subject), draft.Body)

	// With an open PR the draft is what the PR says now on GitHub, not what was recorded.
	pr := gh.add(db.LinkPR, "Edited on GitHub", func(it *GitHubItem) { it.Head, it.Base, it.Body = th.Branch, "main", "The description.\n" })
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink {
		return append(l, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: pr.Number, Title: "Old title", State: db.LinkStateOpen, Head: th.Branch, Base: "main"})
	}))
	draft, err = f.threads.MergeDraft(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Equal(t, MergeDraft{Number: pr.Number, Source: "pr", Title: "Edited on GitHub", Body: "The description."}, draft)
}

func TestHumanBranch(t *testing.T) {
	assert.Equal(t, "Fix the bug", humanBranch("fix-the-bug"))
	assert.Equal(t, "Add feature 20925b", humanBranch("add_feature-20925b"))
	assert.Equal(t, "", humanBranch(""))
}
