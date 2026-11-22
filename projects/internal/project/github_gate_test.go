package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The GitHub tools ask the user unless the project auto-approves every group a
// call belongs to; what is never automatic always asks. See
// .design/thread-permissions.md.

func TestAGitHubWriteAsksWhenTheProjectAutoApprovesNothing(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t)
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_issue", map[string]any{"title": "Bug"})
	assert.True(t, isErr, out)
	assert.Contains(t, out, "did not approve")
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Summary, `Open an issue in acme/widgets: "Bug"`)
	assert.Contains(t, f.approvals[0].Reason, "does not auto-approve publish")
	assert.Equal(t, "github\x00publish", f.approvals[0].Key, "allow for this thread remembers the group")
	assert.NotContains(t, f.gh.called(), `create-issue acme/widgets "Bug"`, "nothing was sent")

	f.allowAll()
	for _, call := range []struct {
		tool string
		in   map[string]any
	}{
		{"create_issue", map[string]any{"title": "Bug"}},
		{"create_pr", map[string]any{"title": "Fix"}},
		{"comment", map[string]any{"number": 11, "body": "see"}},
		{"update", map[string]any{"number": 12, "title": "Fix it"}},
	} {
		before := len(f.approvals)
		out, isErr := f.call(t, cs, call.tool, call.in)
		require.False(t, isErr, "%s: %s", call.tool, out)
		assert.Len(t, f.approvals, before+1, "%s asks", call.tool)
	}
}

func TestAGitHubWriteRunsOnItsOwnWhenItsGroupIsAutoApproved(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t, GitHubPublish)
	cs, _ := f.client(t, f.worker.ID)
	for _, call := range []struct {
		tool string
		in   map[string]any
	}{
		{"create_issue", map[string]any{"title": "Bug"}},
		{"create_pr", map[string]any{"title": "Fix"}},
		{"comment", map[string]any{"number": 11, "body": "see"}},
		{"update", map[string]any{"number": 12, "title": "Fix it"}},
	} {
		out, isErr := f.call(t, cs, call.tool, call.in)
		require.False(t, isErr, "%s: %s", call.tool, out)
	}
	assert.Empty(t, f.approvals, "publish covers the thread's own everyday writes")

	// A force of its own branch is another group.
	f.amendOwn(t)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Fix", "force": true})
	assert.True(t, isErr, out)
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Reason, "does not auto-approve rewrite")
	f.setAuto(t, GitHubPublish, GitHubRewrite)
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Fix", "force": true})
	require.False(t, isErr, out)
	assert.Len(t, f.approvals, 1, "publish and rewrite cover a leased force of the thread's own branch")
}

func TestMergeIsOneCardForThePushAndTheMerge(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t, GitHubPublish)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "merge", map[string]any{"method": "squash"})
	require.False(t, isErr, out)
	require.Len(t, f.approvals, 1, "one card, not one for the push and one for the merge")
	assert.Contains(t, f.approvals[0].Summary, "then squash-merge its PR")
	assert.Contains(t, f.approvals[0].Reason, "does not auto-approve merge")

	// merge_request only opens the PR: publish covers it.
	f.stackBranch(t, "tp/next", "main")
	out, isErr = f.call(t, cs, "merge", map[string]any{"method": "merge_request", "head": "tp/next"})
	require.False(t, isErr, out)
	assert.Len(t, f.approvals, 1)
}

func TestWhatIsNeverAutomaticAsksWithEveryGroupOn(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t, GitHubGroups...)
	f.gh.protected = map[string]bool{"release": true}
	f.stackBranch(t, "release", "main")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Release", "head": "release"})
	assert.True(t, isErr, out)
	require.Len(t, f.approvals, 1, "a protected head asks whatever the project says")
	assert.Contains(t, f.approvals[0].Reason, "branch release is protected on GitHub")
	assert.NotEqual(t, "github\x00publish", f.approvals[0].Key, "allow for this thread covers only this target")
	assert.False(t, f.remoteHas(t, "release"), "nothing was pushed")
}

// The Merge dialog is the user's own click: nothing asks again.
func TestTheUsersMergeDialogIsNotGated(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t)
	_, err := f.links.Merge(byUser(context.Background()), f.thread(t), MergeInput{Method: MergeSquash})
	require.NoError(t, err)
	assert.Empty(t, f.approvals)
	links, _ := f.store.ThreadLinks(f.worker.ID)
	require.Len(t, links, 1)
	assert.Equal(t, db.LinkStateMerged, links[0].State)
}
