package project

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// close ends any issue or PR of the thread's repository, after the user
// approves the call on a card that names who opened it. See
// .design/github-tools.md.

func (f *linksFixture) closes() []string {
	var out []string
	for _, c := range f.gh.called() {
		if len(c) >= 5 && c[:5] == "close" {
			out = append(out, c)
		}
	}
	return out
}

func TestCloseClosesAnOpenedPRAndTheRecordFollows(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Wrong"})
	require.False(t, isErr, out)
	f.events = nil

	// A bare call means the thread's one open PR.
	out, isErr = f.call(t, cs, "close", map[string]any{})
	require.False(t, isErr, out)
	assert.Contains(t, out, "closed pr #11")
	assert.Equal(t, []string{"close acme/widgets #11"}, f.closes())
	require.Len(t, f.approvals, 1, "the user is asked, even for the thread's own PR")
	assert.Contains(t, f.approvals[0].Summary, `Close pr #11 "Wrong", opened by this thread`)
	assert.Empty(t, f.approvals[0].Key, "an answer is for this call only")

	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateClosed, l.State)
	assert.Equal(t, db.LinkOpened, l.Role)
	assert.Equal(t, [2]string{"tp/work", "main"}, [2]string{l.Head, l.Base}, "the issues API's answer has no branches: the record keeps them")
	assert.NotEmpty(t, f.events, "the thread event is published")
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "squash")
	}
}

func TestCloseClosesAnOpenedIssue(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_issue", map[string]any{"title": "Dup"})
	require.False(t, isErr)
	out, isErr := f.call(t, cs, "close", map[string]any{"number": 11})
	require.False(t, isErr, out)
	assert.Contains(t, out, "closed issue #11")
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateClosed, l.State)
	assert.Equal(t, db.LinkIssue, l.Kind)
}

func TestCloseOfWhatTheThreadDidNotOpenNamesWhoDidAndIsRecorded(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)

	// Opened on GitHub by someone, not by a thread.
	theirs := f.gh.add(db.LinkIssue, "Theirs", func(it *GitHubItem) { it.Author = "alice" })
	out, isErr := f.call(t, cs, "close", map[string]any{"number": theirs.Number})
	require.False(t, isErr, out)
	assert.Contains(t, out, "opened by @alice on GitHub, not a thread")
	require.Len(t, f.approvals, 1)
	assert.Contains(t, f.approvals[0].Summary, `Close issue #11 "Theirs", opened by @alice on GitHub, not a thread`)
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", theirs.Number)
	require.NoError(t, err, "the closed item is in the thread's records")
	assert.Equal(t, db.LinkStateClosed, l.State)
	assert.Equal(t, db.LinkCommented, l.Role, "touched, not opened: editing it later still asks")

	// Opened by another thread: the card says which.
	other := f.gh.add(db.LinkPR, "Superseded", nil)
	_, err = f.links.record(f.folder, GitHubRepo{Owner: "acme", Name: "widgets"}, db.LinkPR, db.LinkOpened, other)
	require.NoError(t, err)
	out, isErr = f.call(t, cs, "close", map[string]any{"number": other.Number})
	require.False(t, isErr, out)
	require.Len(t, f.approvals, 2)
	assert.Contains(t, f.approvals[1].Summary, fmt.Sprintf("opened by another thread, %q", f.folder.Title))
	assert.Equal(t, "closed", f.gh.items[other.Number].State)
}

func TestCloseDoesNothingWhenTheUserSaysNo(t *testing.T) {
	f := newLinksFixture(t)
	f.answer = func(Approval) (bool, string) { return false, "keep it" }
	cs, _ := f.client(t, f.worker.ID)
	theirs := f.gh.add(db.LinkPR, "Theirs", nil)

	out, isErr := f.call(t, cs, "close", map[string]any{"number": theirs.Number})
	assert.True(t, isErr)
	assert.Contains(t, out, "did not approve")
	assert.Contains(t, out, "keep it")
	assert.Empty(t, f.closes(), "GitHub was never asked to close")
	assert.Equal(t, "open", f.gh.items[theirs.Number].State)
	_, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", theirs.Number)
	assert.ErrorIs(t, err, db.ErrNotFound, "a refused close records nothing")

	out, isErr = f.call(t, cs, "close", map[string]any{"number": 999})
	assert.True(t, isErr, out)
	assert.Empty(t, f.closes())
}

func TestCloseOfAnItemAlreadyClosedOrMergedIsNotAnError(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "One"})
	require.False(t, isErr)

	// GitHub says closed while the record still says open.
	f.gh.set(11, func(it *GitHubItem) { it.State = "closed" })
	out, isErr := f.call(t, cs, "close", map[string]any{"number": 11})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already closed")
	assert.Empty(t, f.closes())
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateClosed, l.State)

	// A merged PR stays merged: never recorded as closed.
	f.gh.set(11, func(it *GitHubItem) { it.State = "merged" })
	out, isErr = f.call(t, cs, "close", map[string]any{"number": 11})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already merged")
	assert.Empty(t, f.closes())
	l, err = f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateMerged, l.State)
}

func TestCloseWithSeveralOpenPRsAsksForANumber(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	f.stackBranch(t, "release", "main")
	run(t, f.repo, "git", "push", "-q", "origin", "release")
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "A", "base": "release"})
	require.False(t, isErr)
	_, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "B", "head": "release"})
	require.False(t, isErr)

	out, isErr := f.call(t, cs, "close", map[string]any{})
	assert.True(t, isErr)
	assert.Contains(t, out, "several PRs")
	assert.Contains(t, out, "pass number")
	assert.Empty(t, f.closes())

	out, isErr = f.call(t, cs, "close", map[string]any{"number": 11})
	require.False(t, isErr, out)
	// With one left open, a bare call finds it.
	out, isErr = f.call(t, cs, "close", map[string]any{})
	require.False(t, isErr, out)
	assert.Contains(t, out, "closed pr #12")
}

func TestCloseLeavesNoRecordWhenGitHubFails(t *testing.T) {
	f := newLinksFixture(t)
	f.allowAll()
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "One"})
	require.False(t, isErr)
	f.gh.fail = errString("HTTP 403")
	_, isErr = f.call(t, cs, "close", map[string]any{"number": 11})
	assert.True(t, isErr)
	f.gh.fail = nil
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, db.LinkStateOpen, l.State)
}

func TestGhCLICloseIsOneRESTPatchOnTheIssuesEndpoint(t *testing.T) {
	bin, logFile := fakeGH(t, `{"number":7,"html_url":"https://github.com/acme/widgets/pull/7","title":"T","state":"closed","pull_request":{}}`, 0)
	it, err := ghCLI{bin: bin}.Close(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 7)
	require.NoError(t, err)
	assert.Equal(t, "closed", it.State)
	assert.True(t, it.IsPR)
	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(b), "api -X PATCH --input - repos/acme/widgets/issues/7")
	assert.Contains(t, string(b), `{"state":"closed"}`)
	assert.NotContains(t, string(b), "merge")
}

// The tool asks itself, on a card that can name the item: no rule on its name
// adds a second card, and thread_github_access does not cover it.
func TestCloseIsAskedByTheToolNotByARule(t *testing.T) {
	for _, access := range []string{"", GitHubAllow, GitHubAsk} {
		doc := map[string]any{}
		grantBuiltinTools(doc, &db.Project{ThreadGitHubAccess: access})
		assert.NotContains(t, permissionsOf(doc)["ask"], githubToolRule(toolClose), "access %q", access)
		assert.NotContains(t, permissionsOf(doc)["allow"], githubToolRule(toolClose), "only the server-wide allow")
	}
	assert.NotContains(t, githubWriteTools, toolClose, "thread_github_access does not cover it")
	assert.NotContains(t, alwaysAskTools, toolClose)
}
