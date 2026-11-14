package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linkFixture(t *testing.T) (*Store, *Project, *Thread) {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	p := &Project{Name: "P"}
	require.NoError(t, s.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode}
	require.NoError(t, s.CreateThread(th))
	return s, p, th
}

func TestUpsertThreadLinkKeepsOneItemAndTheStrongerRole(t *testing.T) {
	s, _, th := linkFixture(t)
	first, err := s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "acme/w", Number: 3, Role: LinkOpened, State: LinkStateOpen, Title: "one"})
	require.NoError(t, err)
	require.False(t, first.CreatedAt.IsZero())

	// A later comment on the same PR updates it but does not demote "opened".
	_, err = s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "acme/w", Number: 3, Role: LinkCommented, State: LinkStateClosed, Title: "two"})
	require.NoError(t, err)

	links, err := s.ThreadLinks(th.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, LinkOpened, links[0].Role)
	assert.Equal(t, "two", links[0].Title)
	assert.Equal(t, LinkStateClosed, links[0].State)
	assert.True(t, links[0].CreatedAt.Equal(first.CreatedAt))

	// The same number as an issue, or in another repository, is another item.
	_, err = s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkIssue, Repo: "acme/w", Number: 3, Role: LinkOpened})
	require.NoError(t, err)
	_, err = s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "acme/other", Number: 3, Role: LinkOpened})
	require.NoError(t, err)
	links, err = s.ThreadLinks(th.ID)
	require.NoError(t, err)
	assert.Len(t, links, 3)
}

func TestLinksComeBackWithTheThreadAndOnlyThatThreads(t *testing.T) {
	s, p, th := linkFixture(t)
	other := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode}
	require.NoError(t, s.CreateThread(other))
	_, err := s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 7, State: LinkStateOpen, CI: "failing"})
	require.NoError(t, err)

	got, err := s.GetThread(th.ID)
	require.NoError(t, err)
	require.Len(t, got.Links, 1)
	assert.Equal(t, "failing", got.Links[0].CI)
	listed, err := s.ListThreads(p.ID)
	require.NoError(t, err)
	for _, l := range listed {
		if l.ID == other.ID {
			assert.Empty(t, l.Links)
		} else {
			assert.Len(t, l.Links, 1)
		}
	}
	f, err := s.FindThreadLink(th.ID, "a/b", 7)
	require.NoError(t, err)
	assert.Equal(t, 7, f.Number)
	_, err = s.FindThreadLink(th.ID, "a/other", 7)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.FindThreadLink(other.ID, "a/b", 7)
	assert.ErrorIs(t, err, ErrNotFound)
}

// SaveThread writes a whole row, from whatever copy the caller has. The links
// are not part of that: a copy read before a refresh must not undo it.
func TestSavingAnOldCopyOfAThreadDoesNotUndoItsLinks(t *testing.T) {
	s, _, th := linkFixture(t)
	stale, err := s.GetThread(th.ID)
	require.NoError(t, err)
	require.Empty(t, stale.Links)

	_, err = s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 1, State: LinkStateOpen, CI: "passing"})
	require.NoError(t, err)
	stale.Title = "renamed meanwhile"
	require.NoError(t, s.SaveThread(stale))

	got, err := s.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed meanwhile", got.Title, "the rest of the row is saved")
	require.Len(t, got.Links, 1, "the links the stale copy did not know are still there")
	assert.Equal(t, "passing", got.Links[0].CI)

	// And the other way round: a refresh does not touch the row's other columns.
	got.Title = "kept"
	require.NoError(t, s.SaveThread(got))
	require.NoError(t, s.SaveThreadLinkReading(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 1, State: LinkStateMerged, RefreshedAt: time.Now()}))
	again, err := s.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, "kept", again.Title)
	assert.Equal(t, LinkStateMerged, again.Links[0].State)
}

func TestLinkedThreadsSkipsThreadsWithoutLinksAndArchivedProjects(t *testing.T) {
	s, p, th := linkFixture(t)
	bare := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode}
	require.NoError(t, s.CreateThread(bare))
	_, err := s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 1, State: LinkStateOpen})
	require.NoError(t, err)

	got, err := s.LinkedThreads()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, th.ID, got[0].ID)
	require.Len(t, got[0].Links, 1)

	now := time.Now()
	p.ArchivedAt = &now
	require.NoError(t, s.SaveProject(p))
	got, err = s.LinkedThreads()
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSaveThreadLinkReadingChangesOnlyTheReading(t *testing.T) {
	s, _, th := linkFixture(t)
	_, err := s.UpsertThreadLink(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 1, Role: LinkOpened, State: LinkStateOpen, CI: "failing", Title: "T"})
	require.NoError(t, err)
	require.NoError(t, s.SaveThreadLinkReading(th.ID, ThreadLink{Kind: LinkPR, Repo: "a/b", Number: 1, Role: "hacked", CI: "passing", Failures: 2, LastError: "boom"}))
	got, err := s.FindThreadLink(th.ID, "a/b", 1)
	require.NoError(t, err)
	assert.Equal(t, "passing", got.CI)
	assert.Equal(t, 2, got.Failures)
	assert.Equal(t, "boom", got.LastError)
	assert.Equal(t, LinkOpened, got.Role)
	assert.Equal(t, 2, got.Failures)
}

func TestUpdatingLinksOfAMissingThreadIsNotFound(t *testing.T) {
	s, _, _ := linkFixture(t)
	_, err := s.UpsertThreadLink("no-such-thread", ThreadLink{Kind: LinkIssue, Repo: "a/b", Number: 1})
	assert.ErrorIs(t, err, ErrNotFound)
}
