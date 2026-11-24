package project

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A PR opened wrong is put right on the same PR: create_pr on a branch that has
// an open PR, and update, edit it; a closed PR is recorded as closed and its
// branch can have a new one. See .design/github-tools.md ("Putting a PR right").

func (f *linksFixture) updates() []string {
	var out []string
	for _, c := range f.gh.called() {
		if len(c) >= 9 && c[:9] == "update-pr" {
			out = append(out, c)
		}
	}
	return out
}

func (f *linksFixture) creates() int {
	n := 0
	for _, c := range f.gh.called() {
		if len(c) >= 9 && c[:9] == "create-pr" {
			n++
		}
	}
	return n
}

func TestCreatePRAgainAmendsTheOpenPRInsteadOfOpeningAnother(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "release", "main")
	run(t, f.repo, "git", "push", "-q", "origin", "release")
	cs, _ := f.client(t, f.worker.ID)
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Wrong title", "body": "Wrong text", "base": "release"})
	require.False(t, isErr, out)

	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Right title", "body": "Right text", "base": "main"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already open")
	assert.Contains(t, out, "title, description, base")
	assert.Equal(t, 1, f.creates(), "no second PR")
	assert.Equal(t, []string{"update-pr acme/widgets #11 title=Right title body=Right text base=main"}, f.updates())

	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.NoError(t, err)
	assert.Equal(t, "Right title", l.Title)
	assert.Equal(t, [2]string{"tp/work", "main"}, [2]string{l.Head, l.Base}, "the record follows the PR")
	assert.Equal(t, db.LinkOpened, l.Role)
}

func TestCreatePRAgainChangesOnlyWhatDiffersAndNeverGuessesABase(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a", "body": "Adds a."})
	require.False(t, isErr)

	// Same words: nothing to change, nothing sent.
	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a", "body": "Adds a."})
	require.False(t, isErr, out)
	assert.Empty(t, f.updates())
	assert.NotContains(t, out, "updated")

	// No body and no base given: the PR keeps its description and its base.
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Add a, properly"})
	require.False(t, isErr, out)
	assert.Equal(t, []string{"update-pr acme/widgets #11 title=Add a, properly"}, f.updates())
	assert.Contains(t, out, "updated its title")
	assert.Equal(t, "Adds a.", f.gh.items[11].Body)
	assert.Equal(t, "main", f.gh.items[11].Base)
}

func TestCreatePRAgainRefusesABaseThatIsNotOnGitHubBeforeAnythingChanges(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a"})
	require.False(t, isErr)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Changed", "base": "nowhere"})
	assert.True(t, isErr)
	assert.Contains(t, out, "not on GitHub")
	assert.Empty(t, f.updates(), "a refused call changes nothing, title included")
}

func TestCreatePRAgainDoesNotEditAPRTheThreadDoesNotHold(t *testing.T) {
	f := newLinksFixture(t)
	// Someone opened a PR from the thread's branch by hand.
	byHand := f.gh.add(db.LinkPR, "Hand written", func(it *GitHubItem) { it.Head, it.Base, it.Body = "tp/work", "main", "Careful text" })
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Thread's idea", "body": "Other text"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "not edited")
	assert.Empty(t, f.updates())
	assert.Equal(t, "Careful text", f.gh.items[byHand.Number].Body)
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", byHand.Number)
	require.NoError(t, err, "it is recorded, as it always was")
	assert.Equal(t, db.LinkOpened, l.Role)

	// Held now: the same call edits it.
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Thread's idea"})
	require.False(t, isErr, out)
	assert.Equal(t, []string{"update-pr acme/widgets #" + itoa(byHand.Number) + " title=Thread's idea"}, f.updates())
}

func TestMergeRequestDoesNotRewriteAnOpenPR(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Carefully worded"})
	require.False(t, isErr)

	out, isErr := f.call(t, cs, "merge", map[string]any{"method": "merge_request"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "already open")
	assert.Empty(t, f.updates(), "merge's default title is the thread's, never written over a PR's")
}

func TestAClosedPRIsRecordedAsClosedWhenItsBranchGetsANewOne(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Wrong PR"})
	require.False(t, isErr)
	// The user closes it on GitHub; the thread's record still says open.
	f.gh.set(11, func(it *GitHubItem) { it.State = "closed" })
	old, _ := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	require.Equal(t, db.LinkStateOpen, old.State)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Right PR"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "opened PR #12")
	assert.Contains(t, out, "#11 was closed")

	old, _ = f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	assert.Equal(t, db.LinkStateClosed, old.State, "the closed PR is told apart from the new one")
	fresh, _ := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 12)
	assert.Equal(t, db.LinkStateOpen, fresh.State)

	// A bare update means the open one; it used to ask "several PRs".
	out, isErr = f.call(t, cs, "update", map[string]any{"title": "Right PR, again"})
	require.False(t, isErr, out)
	assert.Contains(t, f.gh.called(), "update-pr acme/widgets #12 title=Right PR, again")
}

func TestAClosedRecordIsMarkedClosedEvenWhenGitHubCannotBeRead(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Wrong PR"})
	require.False(t, isErr)
	f.gh.set(11, func(it *GitHubItem) { it.State = "closed" })
	f.gh.failPR = assert.AnError

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Right PR"})
	require.False(t, isErr, out)
	old, _ := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	assert.Equal(t, db.LinkStateClosed, old.State, "GitHub has no open PR for the branch, so the record cannot be open")
}

func TestAMergedBranchWithNothingNewCannotGetAnotherPR(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a"})
	require.False(t, isErr)
	f.gh.set(11, func(it *GitHubItem) { it.State = "merged" })
	// The merge landed on main (as a squash commit would): the branch has no commit beyond it.
	run(t, f.repo, "git", "branch", "-f", "main", "tp/work")
	run(t, f.repo, "git", "push", "-q", "origin", "main")

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Again"})
	assert.True(t, isErr)
	assert.Contains(t, out, "no commit beyond")
	old, _ := f.store.FindThreadLink(f.worker.ID, "acme/widgets", 11)
	assert.Equal(t, db.LinkStateMerged, old.State, "the record learned the PR is merged")
}

func TestAPRForAnotherBranchAgainstTheSameBaseIsFlaggedAsPossibleReplacement(t *testing.T) {
	f := newLinksFixture(t)
	f.stackBranch(t, "tp/right", "main")
	cs, _ := f.client(t, f.worker.ID)
	_, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "From the wrong branch"})
	require.False(t, isErr)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "From the right branch", "head": "tp/right", "base": "main"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "opened PR #12")
	assert.Contains(t, out, "#11 (tp/work → main) is still open")
	assert.Contains(t, out, "close it with the close tool (the user is asked)")

	// A stack is not a replacement: another base does not trigger the note.
	f.stackBranch(t, "tp/top", "tp/right")
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "Top", "head": "tp/top", "base": "tp/right"})
	require.False(t, isErr, out)
	assert.NotContains(t, out, "still open")
}

func TestUpdateRefusalOfAClosedPRSaysHowToCarryOn(t *testing.T) {
	f := newLinksFixture(t)
	cs, _ := f.client(t, f.worker.ID)
	_, _ = f.call(t, cs, "create_pr", map[string]any{"title": "Add a"})
	f.gh.set(11, func(it *GitHubItem) { it.State = "closed" })
	_, _ = f.call(t, cs, "view", map[string]any{})

	out, isErr := f.call(t, cs, "update", map[string]any{"title": "x", "base": "main"})
	assert.True(t, isErr)
	assert.Contains(t, out, "is closed")
	assert.Contains(t, out, "create_pr")
}

func itoa(n int) string { return strconv.Itoa(n) }
